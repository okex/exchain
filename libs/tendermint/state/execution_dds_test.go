package state

import (
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/okex/exchain/libs/iavl"
	redis_cgi "github.com/okex/exchain/libs/tendermint/delta/redis-cgi"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/go-amino"
)

func getRedisClient(t *testing.T) *redis_cgi.RedisClient {
	s := miniredis.RunT(t)
	logger := log.TestingLogger()
	ss := redis_cgi.NewRedisClient(s.Addr(), "", time.Minute, 0, logger)
	return ss
}

func failRedisClient() *redis_cgi.RedisClient {
	logger := log.TestingLogger()
	ss := redis_cgi.NewRedisClient("127.0.0.1:6378", "", time.Minute, 0, logger)
	return ss
}

func setupTest(t *testing.T) *DeltaContext {
	dc := newDeltaContext(log.TestingLogger())
	rclient := getRedisClient(t)
	dc.deltaWriter = rclient
	dc.deltaReader = rclient
	return dc
}

func bytesEqual(b1, b2 []byte) bool {
	return hex.EncodeToString(b1) == hex.EncodeToString(b2)
}
func deltaEqual(d1, d2 *types.Deltas) bool {
	if d1 == nil && d2 == nil {
		return true
	}
	if d1 == nil || d2 == nil {
		return false
	}
	return d1.Height == d2.Height &&
		d1.From == d2.From &&
		d1.CompressType == d2.CompressType &&
		d1.CompressFlag == d2.CompressFlag &&
		bytesEqual(d1.ABCIRsp(), d2.ABCIRsp()) &&
		bytesEqual(d1.DeltasBytes(), d2.DeltasBytes()) &&
		bytesEqual(d1.WatchBytes(), d2.WatchBytes())
}

func TestDeltaContext_prepareStateDelta(t *testing.T) {
	dc := setupTest(t)
	dc.downloadDelta = true

	deltaInfos := make([]*DeltaInfo, 3)
	for i := 0; i <= 2; i++ {
		h := int64(i + 1)
		deltaInfos[i] = &DeltaInfo{
			from:          "0x01",
			deltaLen:      1000,
			deltaHeight:   h,
			abciResponses: &ABCIResponses{},
			treeDeltaMap:  iavl.TreeDeltaMap{},
		}
		dc.dataMap.insert(h, deltaInfos[i], h)
	}

	tests := []struct {
		name     string
		height   int64
		wantInfo *DeltaInfo
	}{
		{"normal case", 1, deltaInfos[0]},
		{"empty delta", 4, nil},
		{"already remove", 1, nil},
		{"higher height", 3, deltaInfos[2]},
		{"lower remove", 2, deltaInfos[1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotInfo := dc.prepareStateDelta(tt.height); !reflect.DeepEqual(gotInfo, tt.wantInfo) {
				t.Errorf("prepareStateDelta() = %v, want %v", gotInfo, tt.wantInfo)
			}
		})
	}
}

func TestDeltaContext_download(t *testing.T) {
	dc := setupTest(t)
	deltas := &types.Deltas{Height: 10, Payload: types.DeltaPayload{ABCIRsp: []byte("ABCIRsp"), DeltasBytes: []byte("DeltasBytes"), WatchBytes: []byte("WatchBytes")}}
	dc.uploadRoutine(deltas, 0)

	tests := []struct {
		name   string
		height int64
		wants  *types.Deltas
	}{
		{"normal case", 10, deltas},
		{"higher height", 11, nil},
		{"lower height", 9, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err, mrh := dc.deltaReader.GetDeltas(tt.height)
			got, got1, got2 := dc.download(tt.height)
			if !reflect.DeepEqual(got, err) {
				t.Errorf("download() got = %v, want %v", got, err)
			}
			if !deltaEqual(got1, tt.wants) {
				t.Errorf("download() got = %v, want %v", got, deltas)
			}
			if got2 != mrh {
				t.Errorf("download() got2 = %v, want %v", got2, mrh)
			}
		})
	}
}

func TestDeltaContext_upload(t *testing.T) {
	dc := setupTest(t)
	deltas := &types.Deltas{Payload: types.DeltaPayload{ABCIRsp: []byte("ABCIRsp"), DeltasBytes: []byte("DeltasBytes"), WatchBytes: []byte("WatchBytes")}}
	okRedis := getRedisClient(t)
	failRedis := failRedisClient()

	tests := []struct {
		name   string
		r      *redis_cgi.RedisClient
		deltas *types.Deltas
		want   bool
	}{
		{"normal case", okRedis, deltas, true},
		{"nil delta", okRedis, nil, false},
		{"empty delta", okRedis, &types.Deltas{}, false},
		{"fail redis", failRedis, deltas, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.deltaWriter = tt.r
			if got := dc.upload(tt.deltas, 0, 0); got != tt.want {
				t.Errorf("upload() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------------------

func produceBlock() ([]*types.Block, dbm.DB) {
	state, stateDB, _ := makeState(2, 2)
	prevHash := state.LastBlockID.Hash
	prevParts := types.PartSetHeader{}
	prevBlockID := types.BlockID{Hash: prevHash, PartsHeader: prevParts}
	var (
		now        = tmtime.Now()
		commitSig0 = types.NewCommitSigForBlock(
			[]byte("Signature1"),
			state.Validators.Validators[0].Address,
			now)
		commitSig1 = types.NewCommitSigForBlock(
			[]byte("Signature2"),
			state.Validators.Validators[1].Address,
			now)
		absentSig = types.NewCommitSigAbsent()
	)

	testCases := []struct {
		desc                     string
		lastCommitSigs           []types.CommitSig
		expectedAbsentValidators []int
	}{
		{"none absent", []types.CommitSig{commitSig0, commitSig1}, []int{}},
		{"one absent", []types.CommitSig{commitSig0, absentSig}, []int{1}},
		{"multiple absent", []types.CommitSig{absentSig, absentSig}, []int{0, 1}},
	}
	blocks := make([]*types.Block, len(testCases))
	for i, tc := range testCases {
		lastCommit := types.NewCommit(1, 0, prevBlockID, tc.lastCommitSigs)
		blocks[i], _ = state.MakeBlock(2, makeTxs(2), lastCommit, nil, state.Validators.GetProposer().Address)
	}

	return blocks, stateDB
}

func produceAbciRsp() *ABCIResponses {
	proxyApp := newTestApp()
	proxyApp.Start()
	defer proxyApp.Stop()

	blocks, stateDB := produceBlock()
	ctx := &executionTask{
		logger:   log.TestingLogger(),
		block:    blocks[0],
		db:       stateDB,
		proxyApp: proxyApp.Consensus(),
	}

	abciResponses, _ := execBlockOnProxyApp(ctx)
	return abciResponses
}

func TestProduceDelta(t *testing.T) {
	proxyApp := newTestApp()
	err := proxyApp.Start()
	require.Nil(t, err)
	defer proxyApp.Stop()

	blocks, stateDB := produceBlock()
	for _, block := range blocks {
		deltas, _, err := execCommitBlockDelta(proxyApp.Consensus(), block, log.TestingLogger(), stateDB)
		require.Nil(t, err)
		require.NotNil(t, deltas)
	}
}

func BenchmarkMarshalJson(b *testing.B) {
	abciResponses := produceAbciRsp()

	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		types.Json.Marshal(abciResponses)
	}
}

func BenchmarkMarshalAmino(b *testing.B) {
	abciResponses := produceAbciRsp()
	var cdc = amino.NewCodec()

	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		cdc.MarshalBinaryBare(abciResponses)
	}
}

func BenchmarkMarshalCustom(b *testing.B) {
	abciResponses := produceAbciRsp()

	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		abciResponses.MarshalToAmino(ModuleCodec)
	}
}

func BenchmarkUnmarshalFromJson(b *testing.B) {
	abciResponses := produceAbciRsp()
	data, _ := types.Json.Marshal(abciResponses)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		ar := &ABCIResponses{}
		types.Json.Unmarshal(data, ar)
	}
}
func BenchmarkUnmarshalFromAmino(b *testing.B) {
	abciResponses := produceAbciRsp()
	var cdc = amino.NewCodec()
	data, _ := cdc.MarshalBinaryBare(abciResponses)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		ar := &ABCIResponses{}
		cdc.UnmarshalBinaryBare(data, ar)
	}
}
func BenchmarkUnmarshalFromCustom(b *testing.B) {
	abciResponses := produceAbciRsp()
	data, _ := abciResponses.MarshalToAmino(cdc)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n <= b.N; n++ {
		ar := &ABCIResponses{}
		ar.UnmarshalFromAmino(nil, data)
	}
}
