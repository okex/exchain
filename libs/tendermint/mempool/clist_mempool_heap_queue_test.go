package mempool

import (
	"encoding/binary"
	"fmt"
	"github.com/okex/exchain/libs/tendermint/libs/clist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math/big"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/okex/exchain/libs/tendermint/abci/example/counter"
	"github.com/okex/exchain/libs/tendermint/abci/example/kvstore"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmrand "github.com/okex/exchain/libs/tendermint/libs/rand"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/types"
)

func newMempoolWithAppForHeapQueue(cc proxy.ClientCreator) (*CListMempool, cleanupFunc) {
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	return newMempoolWithAppAndConfig(cc, config)
}

func requireHeapQueue(t *testing.T, queue ITransactionQueue) {
	_, ok := queue.(*HeapQueue)
	require.True(t, ok)
}

func TestReapMaxBytesMaxGas_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	// Ensure gas calculation behaves as expected
	checkTxs(t, mempool, 1, UnknownPeerID)
	tx0 := mempool.txs.BroadcastFront().Value.(*mempoolTx)
	// assert that kv store has gas wanted = 1.
	require.Equal(t, app.CheckTx(abci.RequestCheckTx{Tx: tx0.tx}).GasWanted, int64(1), "KVStore had a gas value neq to 1")
	require.Equal(t, tx0.gasWanted, int64(1), "transactions gas was set incorrectly")
	// ensure each tx is 20 bytes long
	require.Equal(t, len(tx0.tx), 20, "Tx is longer than 20 bytes")
	mempool.Flush()

	// each table driven test creates numTxsToCreate txs with checkTx, and at the end clears all remaining txs.
	// each tx has 20 bytes + amino overhead = 21 bytes, 1 gas
	tests := []struct {
		numTxsToCreate int
		maxBytes       int64
		maxGas         int64
		expectedNumTxs int
	}{
		{20, -1, -1, 20},
		{20, -1, 0, 0},
		{20, -1, 10, 10},
		{20, -1, 30, 20},
		{20, 0, -1, 0},
		{20, 0, 10, 0},
		{20, 10, 10, 0},
		{20, 22, 10, 1},
		{20, 220, -1, 10},
		{20, 220, 5, 5},
		{20, 220, 10, 10},
		{20, 220, 15, 10},
		{20, 20000, -1, 20},
		{20, 20000, 5, 5},
		{20, 20000, 30, 20},
		{2000, -1, -1, 300},
	}
	for tcIndex, tt := range tests {
		checkTxs(t, mempool, tt.numTxsToCreate, UnknownPeerID)
		got := mempool.ReapMaxBytesMaxGas(tt.maxBytes, tt.maxGas)
		assert.Equal(t, tt.expectedNumTxs, len(got), "Got %d txs, expected %d, tc #%d",
			len(got), tt.expectedNumTxs, tcIndex)
		mempool.Flush()
	}
}

func TestMempoolFilters_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	emptyTxArr := []types.Tx{[]byte{}}

	nopPreFilter := func(tx types.Tx) error { return nil }
	nopPostFilter := func(tx types.Tx, res *abci.ResponseCheckTx) error { return nil }

	// each table driven test creates numTxsToCreate txs with checkTx, and at the end clears all remaining txs.
	// each tx has 20 bytes + amino overhead = 21 bytes, 1 gas
	tests := []struct {
		numTxsToCreate int
		preFilter      PreCheckFunc
		postFilter     PostCheckFunc
		expectedNumTxs int
	}{
		{10, nopPreFilter, nopPostFilter, 10},
		{10, PreCheckAminoMaxBytes(10), nopPostFilter, 0},
		{10, PreCheckAminoMaxBytes(20), nopPostFilter, 0},
		{10, PreCheckAminoMaxBytes(22), nopPostFilter, 10},
		{10, nopPreFilter, PostCheckMaxGas(-1), 10},
		{10, nopPreFilter, PostCheckMaxGas(0), 0},
		{10, nopPreFilter, PostCheckMaxGas(1), 10},
		{10, nopPreFilter, PostCheckMaxGas(3000), 10},
		{10, PreCheckAminoMaxBytes(10), PostCheckMaxGas(20), 0},
		{10, PreCheckAminoMaxBytes(30), PostCheckMaxGas(20), 10},
		{10, PreCheckAminoMaxBytes(22), PostCheckMaxGas(1), 10},
		{10, PreCheckAminoMaxBytes(22), PostCheckMaxGas(0), 0},
	}
	for tcIndex, tt := range tests {
		mempool.Update(1, emptyTxArr, abciResponses(len(emptyTxArr), abci.CodeTypeOK), tt.preFilter, tt.postFilter)
		checkTxs(t, mempool, tt.numTxsToCreate, UnknownPeerID)
		require.Equal(t, tt.expectedNumTxs, mempool.Size(), "mempool had the incorrect size, on test case %d", tcIndex)
		mempool.Flush()
	}
}

func TestMempoolUpdate_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)

	// 1. Adds valid txs to the cache
	{
		mempool.Update(1, []types.Tx{[]byte{0x01}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
		err := mempool.CheckTx([]byte{0x01}, nil, TxInfo{})
		if assert.Error(t, err) {
			assert.Equal(t, ErrTxInCache, err)
		}
	}

	// 2. Removes valid txs from the mempool
	{
		err := mempool.CheckTx([]byte{0x02}, nil, TxInfo{})
		require.NoError(t, err)
		mempool.Update(1, []types.Tx{[]byte{0x02}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
		assert.Zero(t, mempool.Size())
	}

	// 3. Removes invalid transactions from the cache and the mempool (if present)
	{
		err := mempool.CheckTx([]byte{0x03}, nil, TxInfo{})
		require.NoError(t, err)
		mempool.Update(1, []types.Tx{[]byte{0x03}}, abciResponses(1, 1), nil, nil)
		assert.Zero(t, mempool.Size())

		err = mempool.CheckTx([]byte{0x03}, nil, TxInfo{})
		assert.NoError(t, err)
	}
}

func TestTxsAvailable_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	mempool.EnableTxsAvailable()

	timeoutMS := 500

	// with no txs, it shouldnt fire
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch of txs, it should only fire once
	txs := checkTxs(t, mempool, 100, UnknownPeerID)
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// call update with half the txs.
	// it should fire once now for the new height
	// since there are still txs left
	committedTxs, txs := txs[:50], txs[50:]
	if err := mempool.Update(1, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK), nil, nil); err != nil {
		t.Error(err)
	}
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch more txs. we already fired for this height so it shouldnt fire again
	moreTxs := checkTxs(t, mempool, 50, UnknownPeerID)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// now call update with all the txs. it should not fire as there are no txs left
	committedTxs = append(txs, moreTxs...) //nolint: gocritic
	if err := mempool.Update(2, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK), nil, nil); err != nil {
		t.Error(err)
	}
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch more txs, it should only fire once
	checkTxs(t, mempool, 100, UnknownPeerID)
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)
}

func TestSerialReap_HeapQueue(t *testing.T) {
	app := counter.NewApplication(true)
	app.SetOption(abci.RequestSetOption{Key: "serial", Value: "on"})
	cc := proxy.NewLocalClientCreator(app)

	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	mempool.config.MaxTxNumPerBlock = 10000

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err := appConnCon.Start()
	require.Nil(t, err)

	cacheMap := make(map[string]struct{})
	deliverTxsRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {

			// This will succeed
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			err := mempool.CheckTx(txBytes, nil, TxInfo{})
			_, cached := cacheMap[string(txBytes)]
			if cached {
				require.NotNil(t, err, "expected error for cached tx")
			} else {
				require.Nil(t, err, "expected no err for uncached tx")
			}
			cacheMap[string(txBytes)] = struct{}{}

			// Duplicates are cached and should return error
			err = mempool.CheckTx(txBytes, nil, TxInfo{})
			require.NotNil(t, err, "Expected error after CheckTx on duplicated tx")
		}
	}

	reapCheck := func(exp int) {
		txs := mempool.ReapMaxBytesMaxGas(-1, -1)
		require.Equal(t, len(txs), exp, fmt.Sprintf("Expected to reap %v txs but got %v", exp, len(txs)))
	}

	updateRange := func(start, end int) {
		txs := make([]types.Tx, 0)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			txs = append(txs, txBytes)
		}
		if err := mempool.Update(0, txs, abciResponses(len(txs), abci.CodeTypeOK), nil, nil); err != nil {
			t.Error(err)
		}
	}

	commitRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
			if err != nil {
				t.Errorf("client error committing tx: %v", err)
			}
			if res.IsErr() {
				t.Errorf("error committing tx. Code:%v result:%X log:%v",
					res.Code, res.Data, res.Log)
			}
		}
		res, err := appConnCon.CommitSync(abci.RequestCommit{})
		if err != nil {
			t.Errorf("client error committing: %v", err)
		}
		if len(res.Data) != 8 {
			t.Errorf("error committing. Hash:%X", res.Data)
		}
	}

	//----------------------------------------

	// Deliver some txs.
	deliverTxsRange(0, 100)

	// Reap the txs.
	reapCheck(100)

	// Reap again.  We should get the same amount
	reapCheck(100)

	// Deliver 0 to 999, we should reap 900 new txs
	// because 100 were already counted.
	deliverTxsRange(0, 1000)

	// Reap the txs.
	reapCheck(BlockMaxTxNum)

	// Reap again.  We should get the same amount
	reapCheck(BlockMaxTxNum)

	// Commit from the conensus AppConn
	commitRange(0, BlockMaxTxNum)
	updateRange(0, BlockMaxTxNum)

	// We should have 500 left.
	reapCheck(BlockMaxTxNum)

	// Deliver 100 invalid txs and 100 valid txs
	deliverTxsRange(900, 1100)

	// We should have 300 now.
	reapCheck(BlockMaxTxNum)
}

func TestMempoolMaxMsgSize_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempl, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempl.txs)
	maxTxSize := mempl.config.MaxTxBytes
	maxMsgSize := calcMaxMsgSize(maxTxSize)

	testCases := []struct {
		len int
		err bool
	}{
		// check small txs. no error
		{10, false},
		{1000, false},
		{1000000, false},

		// check around maxTxSize
		// changes from no error to error
		{maxTxSize - 2, false},
		{maxTxSize - 1, false},
		{maxTxSize, false},
		{maxTxSize + 1, true},
		{maxTxSize + 2, true},

		// check around maxMsgSize. all error
		{maxMsgSize - 1, true},
		{maxMsgSize, true},
		{maxMsgSize + 1, true},
	}

	for i, testCase := range testCases {
		caseString := fmt.Sprintf("case %d, len %d", i, testCase.len)

		tx := tmrand.Bytes(testCase.len)
		err := mempl.CheckTx(tx, nil, TxInfo{})
		msg := &TxMessage{tx, ""}
		encoded := cdc.MustMarshalBinaryBare(msg)
		require.Equal(t, len(encoded), txMessageSize(tx), caseString)
		if !testCase.err {
			require.True(t, len(encoded) <= maxMsgSize, caseString)
			require.NoError(t, err, caseString)
		} else {
			require.True(t, len(encoded) > maxMsgSize, caseString)
			require.Equal(t, err, ErrTxTooLarge{maxTxSize, testCase.len}, caseString)
		}
	}

}

func TestMempoolTxsBytes_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.MaxTxsBytes = 10
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	// 1. zero by default
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 2. len(tx) after CheckTx
	err := mempool.CheckTx([]byte{0x01}, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, mempool.TxsBytes())

	// 3. zero again after tx is removed by Update
	mempool.Update(1, []types.Tx{[]byte{0x01}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 4. zero after Flush
	err = mempool.CheckTx([]byte{0x02, 0x03}, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, mempool.TxsBytes())

	mempool.Flush()
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 5. ErrMempoolIsFull is returned when/if MaxTxsBytes limit is reached.
	err = mempool.CheckTx([]byte{0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}, nil, TxInfo{})
	require.NoError(t, err)
	err = mempool.CheckTx([]byte{0x05}, nil, TxInfo{})
	if assert.Error(t, err) {
		assert.IsType(t, ErrMempoolIsFull{}, err)
	}

	// 6. zero after tx is rechecked and removed due to not being valid anymore
	app2 := counter.NewApplication(true)
	cc = proxy.NewLocalClientCreator(app2)
	mempool, cleanup = newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	txBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(txBytes, uint64(0))

	err = mempool.CheckTx(txBytes, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 8, mempool.TxsBytes())

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err = appConnCon.Start()
	require.Nil(t, err)
	defer appConnCon.Stop()
	res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Code)
	res2, err := appConnCon.CommitSync(abci.RequestCommit{})
	require.NoError(t, err)
	require.NotEmpty(t, res2.Data)
	// Pretend like we committed nothing so txBytes gets rechecked and removed.
	// our config recheck flag default is false so cannot rechecked to remove unavailable txs
	// add config to check whether to assert mempool txsbytes
	height := int64(1)
	mempool.Update(height, []types.Tx{}, abciResponses(0, abci.CodeTypeOK), nil, nil)
	if cfg.DynamicConfig.GetMempoolRecheck() || height%cfg.DynamicConfig.GetMempoolForceRecheckGap() == 0 {
		assert.EqualValues(t, 0, mempool.TxsBytes())
	} else {
		assert.EqualValues(t, len(txBytes), mempool.TxsBytes())
	}
}

func TestAddAndSortTx_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	//tx := &mempoolTx{height: 1, gasWanted: 1, tx:[]byte{0x01}}
	testCases := []struct {
		Tx *mempoolTx
	}{
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("1"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(3780)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("2"), from: "6", realTx: abci.MockTx{GasPrice: big.NewInt(5853)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("3"), from: "7", realTx: abci.MockTx{GasPrice: big.NewInt(8315)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("4"), from: "10", realTx: abci.MockTx{GasPrice: big.NewInt(9526)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("5"), from: "15", realTx: abci.MockTx{GasPrice: big.NewInt(9140)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("6"), from: "9", realTx: abci.MockTx{GasPrice: big.NewInt(9227)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("7"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(761)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("8"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(9740)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("9"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(6574)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10"), from: "8", realTx: abci.MockTx{GasPrice: big.NewInt(9656)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("11"), from: "12", realTx: abci.MockTx{GasPrice: big.NewInt(6554)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("12"), from: "16", realTx: abci.MockTx{GasPrice: big.NewInt(5609)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("13"), from: "6", realTx: abci.MockTx{GasPrice: big.NewInt(2791), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("14"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(2698), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("15"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(6925), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("16"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(3171)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("17"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(2965), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("18"), from: "19", realTx: abci.MockTx{GasPrice: big.NewInt(2484)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("19"), from: "13", realTx: abci.MockTx{GasPrice: big.NewInt(9722)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("20"), from: "7", realTx: abci.MockTx{GasPrice: big.NewInt(4236), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("21"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(1780)}}},
	}

	for _, exInfo := range testCases {
		mempool.addTx(exInfo.Tx)
	}
	require.Equal(t, 18, mempool.txs.Len(), fmt.Sprintf("Expected to txs length %v but got %v", 18, mempool.txs.Len()))

	// The txs in mempool should sorted, the output should be (head -> tail):
	//
	//Address:  18  , GasPrice:  9740  , Nonce:  0
	//Address:  13  , GasPrice:  9722  , Nonce:  0
	//Address:  8  , GasPrice:  9656  , Nonce:  0
	//Address:  10  , GasPrice:  9526  , Nonce:  0
	//Address:  9  , GasPrice:  9227  , Nonce:  0
	//Address:  15  , GasPrice:  9140  , Nonce:  0
	//Address:  7  , GasPrice:  8315  , Nonce:  0
	//Address:  1  , GasPrice:  6574  , Nonce:  0
	//Address:  1  , GasPrice:  6925  , Nonce:  1
	//Address:  12  , GasPrice:  6554  , Nonce:  0
	//Address:  6  , GasPrice:  5853  , Nonce:  0
	//Address:  16  , GasPrice:  5609  , Nonce:  0
	//Address:  7  , GasPrice:  4236  , Nonce:  1
	//Address:  3  , GasPrice:  3171  , Nonce:  0
	//Address:  1  , GasPrice:  2965  , Nonce:  2
	//Address:  6  , GasPrice:  2791  , Nonce:  1
	//Address:  18  , GasPrice:  2698  , Nonce:  1
	//Address:  19  , GasPrice:  2484  , Nonce:  0

	require.Equal(t, 3, mempool.GetUserPendingTxsCnt("1"))
	require.Equal(t, 1, mempool.GetUserPendingTxsCnt("15"))
	require.Equal(t, 2, mempool.GetUserPendingTxsCnt("18"))

	hq := mempool.txs.(*HeapQueue)
	heads := hq.Init()
	require.True(t, len(heads) > 0)
	require.Equal(t, "18", heads[0].Address)
	require.Equal(t, big.NewInt(9740), heads[0].GasPrice)
	require.Equal(t, uint64(0), heads[0].Nonce)

	tail := hq.InitReverse()
	backTx := hq.PeekReverse(tail)
	require.Equal(t, "19", backTx.Address)
	require.Equal(t, big.NewInt(2484), backTx.GasPrice)
	require.Equal(t, uint64(0), backTx.Nonce)

	require.Equal(t, true, checkTx(heads[0]))

	addressList := mempool.GetAddressList()
	for _, addr := range addressList {
		list, ok := hq.txs[addr]
		require.True(t, ok)
		require.Equal(t, true, checkAccNonce(addr, list.Front()))
	}

	txs := mempool.ReapMaxBytesMaxGas(-1, -1)
	require.Equal(t, 18, len(txs), fmt.Sprintf("Expected to reap %v txs but got %v", 18, len(txs)))

	mempool.Flush()
	require.Equal(t, 0, mempool.txs.Len())
	require.Equal(t, 0, mempool.txs.BroadcastLen())
	require.Equal(t, 0, len(mempool.GetAddressList()))

}

func TestReplaceTx_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	//tx := &mempoolTx{height: 1, gasWanted: 1, tx:[]byte{0x01}}
	testCases := []struct {
		Tx *mempoolTx
	}{
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10000"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(9740)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10001"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(5853), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10002"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(8315), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10003"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(9526), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10004"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(9140), Nonce: 4}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10002"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(9227), Nonce: 2}}},
	}

	for _, exInfo := range testCases {
		mempool.addTx(exInfo.Tx)
	}
	require.Equal(t, 5, mempool.txs.Len(), fmt.Sprintf("Expected to txs length %v but got %v", 5, mempool.txs.Len()))

	var nonces []uint64
	var gasPrices []uint64
	hq := mempool.txs.(*HeapQueue)
	heads := hq.Init()
	tx := hq.Peek(heads)
	for tx != nil {
		nonces = append(nonces, tx.realTx.GetNonce())
		gasPrices = append(gasPrices, tx.realTx.GetGasPrice().Uint64())
		hq.Shift(&heads)
		tx = hq.Peek(heads)
	}

	require.Equal(t, []uint64{0, 1, 2, 3, 4}, nonces)
	require.Equal(t, []uint64{9740, 5853, 9227, 9526, 9140}, gasPrices)
}

func TestAddAndSortTxByRandom_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	AddrNonce := make(map[string]int)
	for i := 0; i < 1000; i++ {
		mempool.addTx(generateNode(AddrNonce, i))
	}

	hq := mempool.txs.(*HeapQueue)
	heads := hq.Init()
	require.True(t, len(heads) > 0)
	front := heads[0]
	require.Equal(t, true, checkTx(front))
	addressList := mempool.GetAddressList()
	for _, addr := range addressList {
		list, ok := hq.txs[addr]
		require.True(t, ok)
		require.Equal(t, true, checkAccNonce(addr, list.Front()))
	}
}

func TestReapUserTxs_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	//tx := &mempoolTx{height: 1, gasWanted: 1, tx:[]byte{0x01}}
	testCases := []struct {
		Tx *mempoolTx
	}{
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("1"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(9740)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("2"), from: "6", realTx: abci.MockTx{GasPrice: big.NewInt(5853)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("3"), from: "7", realTx: abci.MockTx{GasPrice: big.NewInt(8315)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("4"), from: "10", realTx: abci.MockTx{GasPrice: big.NewInt(9526)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("5"), from: "15", realTx: abci.MockTx{GasPrice: big.NewInt(9140)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("6"), from: "9", realTx: abci.MockTx{GasPrice: big.NewInt(9227)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("7"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(761)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("8"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(3780)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("9"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(6574)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10"), from: "8", realTx: abci.MockTx{GasPrice: big.NewInt(9656)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("11"), from: "12", realTx: abci.MockTx{GasPrice: big.NewInt(6554)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("12"), from: "16", realTx: abci.MockTx{GasPrice: big.NewInt(5609)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("13"), from: "6", realTx: abci.MockTx{GasPrice: big.NewInt(2791), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("14"), from: "18", realTx: abci.MockTx{GasPrice: big.NewInt(2698), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("15"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(6925), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("16"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(3171)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("17"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(2965), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("18"), from: "19", realTx: abci.MockTx{GasPrice: big.NewInt(2484)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("19"), from: "13", realTx: abci.MockTx{GasPrice: big.NewInt(9722)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("20"), from: "7", realTx: abci.MockTx{GasPrice: big.NewInt(4236), Nonce: 1}}},
	}

	for _, exInfo := range testCases {
		mempool.addTx(exInfo.Tx)
	}
	require.Equal(t, 18, mempool.txs.Len(), fmt.Sprintf("Expected to txs length %v but got %v", 18,
		mempool.txs.Len()))

	require.Equal(t, 3, mempool.ReapUserTxsCnt("1"), fmt.Sprintf("Expected to txs length of %s "+
		"is %v but got %v", "1", 3, mempool.ReapUserTxsCnt("1")))

	require.Equal(t, 0, mempool.ReapUserTxsCnt("111"), fmt.Sprintf("Expected to txs length of %s "+
		"is %v but got %v", "111", 0, mempool.ReapUserTxsCnt("111")))

	require.Equal(t, 3, len(mempool.ReapUserTxs("1", -1)), fmt.Sprintf("Expected to txs length "+
		"of %s is %v but got %v", "1", 3, len(mempool.ReapUserTxs("1", -1))))

	require.Equal(t, 3, len(mempool.ReapUserTxs("1", 100)), fmt.Sprintf("Expected to txs length "+
		"of %s is %v but got %v", "1", 3, len(mempool.ReapUserTxs("1", 100))))

	require.Equal(t, 0, len(mempool.ReapUserTxs("111", -1)), fmt.Sprintf("Expected to txs length "+
		"of %s is %v but got %v", "111", 0, len(mempool.ReapUserTxs("111", -1))))

	require.Equal(t, 0, len(mempool.ReapUserTxs("111", 100)), fmt.Sprintf("Expected to txs length "+
		"of %s is %v but got %v", "111", 0, len(mempool.ReapUserTxs("111", 100))))
}

func TestMultiPriceBump_HeapQueue(t *testing.T) {
	tests := []struct {
		rawPrice    *big.Int
		priceBump   uint64
		targetPrice *big.Int
	}{
		{big.NewInt(1), 0, big.NewInt(1)},
		{big.NewInt(10), 1, big.NewInt(10)},
		{big.NewInt(100), 0, big.NewInt(100)},
		{big.NewInt(100), 5, big.NewInt(105)},
		{big.NewInt(100), 50, big.NewInt(150)},
		{big.NewInt(100), 150, big.NewInt(250)},
	}
	for _, tt := range tests {
		require.True(t, tt.targetPrice.Cmp(MultiPriceBump(tt.rawPrice, int64(tt.priceBump))) == 0)
	}
}

func TestAddAndSortTxConcurrency_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	//tx := &mempoolTx{height: 1, gasWanted: 1, tx:[]byte{0x01}}
	type Case struct {
		Tx *mempoolTx
	}

	testCases := []Case{
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("1"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(3780)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("2"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(3780), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("3"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(5315), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("4"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(4526), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("5"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(2140), Nonce: 4}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("6"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(4227), Nonce: 5}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("7"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(2161)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("8"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(5740), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("9"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(6574), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("10"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(9630), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("11"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(6554), Nonce: 4}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("12"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(5609), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("13"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(2791)}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("14"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(2698), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("15"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(6925), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("16"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(4171), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("17"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(2965), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("18"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(2484), Nonce: 2}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("19"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(9722), Nonce: 1}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("20"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(4236), Nonce: 3}}},
		{&mempoolTx{height: 1, gasWanted: 1, tx: []byte("21"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(8780), Nonce: 4}}},
	}

	var wait sync.WaitGroup
	for _, exInfo := range testCases {
		wait.Add(1)
		go func(p Case) {
			mempool.addTx(p.Tx)
			wait.Done()
		}(exInfo)
	}

	wait.Wait()
}

func TestReplaceTxWithMultiAddrs_HeapQueue(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.SortTxByGpWithHeap = true
	config.Mempool.SortTxByGp = false
	mempool, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	tx1 := &mempoolTx{height: 1, gasWanted: 1, tx: []byte("10002"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(9740), Nonce: 1}}
	mempool.addTx(tx1)
	tx2 := &mempoolTx{height: 1, gasWanted: 1, tx: []byte("90000"), from: "2", realTx: abci.MockTx{GasPrice: big.NewInt(10717), Nonce: 1}}
	mempool.addTx(tx2)
	tx3 := &mempoolTx{height: 1, gasWanted: 1, tx: []byte("90000"), from: "3", realTx: abci.MockTx{GasPrice: big.NewInt(10715), Nonce: 1}}
	mempool.addTx(tx3)
	tx4 := &mempoolTx{height: 1, gasWanted: 1, tx: []byte("10001"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(10716), Nonce: 2}}
	mempool.addTx(tx4)
	tx5 := &mempoolTx{height: 1, gasWanted: 1, tx: []byte("10001"), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(10712), Nonce: 1}}
	mempool.addTx(tx5)

	var nonces []uint64

	hq := mempool.txs.(*HeapQueue)
	heads := hq.Init()
	tx := hq.Peek(heads)
	for tx != nil {
		if tx.from == "1" {
			nonces = append(nonces, tx.realTx.GetNonce())
		}
		hq.Shift(&heads)
		tx = hq.Peek(heads)
	}
	require.Equal(t, []uint64{1, 2}, nonces)
}

func BenchmarkMempoolLogUpdate_HeapQueue(b *testing.B) {
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "benchmark")
	var options []log.Option
	options = append(options, log.AllowErrorWith("module", "benchmark"))
	logger = log.NewFilter(logger, options...)

	mem := &CListMempool{height: 123456, logger: logger}
	addr := "address"
	nonce := uint64(123456)

	b.Run("pool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mem.logUpdate(addr, nonce)
		}
	})

	b.Run("logger", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mem.logger.Debug("mempool update", "address", addr, "nonce", nonce)
		}
	})
}

func BenchmarkMempoolLogAddTx_HeapQueue(b *testing.B) {
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "benchmark")
	var options []log.Option
	options = append(options, log.AllowErrorWith("module", "benchmark"))
	logger = log.NewFilter(logger, options...)

	mem := &CListMempool{height: 123456, logger: logger, txs: NewBaseTxQueue()}
	tx := []byte("tx")

	memTx := &mempoolTx{
		height: mem.Height(),
		tx:     tx,
	}

	r := &abci.Response_CheckTx{}

	b.Run("pool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mem.logAddTx(memTx, r)
		}
	})

	b.Run("logger", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mem.logger.Info("Added good transaction",
				"tx", txIDStringer{tx, mem.height},
				"res", r,
				"height", memTx.height,
				"total", mem.Size(),
			)
		}
	})
}

func TestCListMempool_GetEnableDeleteMinGPTx_HeapQueue(t *testing.T) {

	testCases := []struct {
		name     string
		prepare  func(mempool *CListMempool, tt *testing.T)
		execFunc func(mempool *CListMempool, tt *testing.T)
	}{
		{
			name: "normal mempool is full add tx failed, disableDeleteMinGPTx",
			prepare: func(mempool *CListMempool, tt *testing.T) {
				mempool.Flush()
				err := mempool.CheckTx([]byte{0x01}, nil, TxInfo{})
				require.NoError(tt, err)
			},
			execFunc: func(mempool *CListMempool, tt *testing.T) {
				err := mempool.CheckTx([]byte{0x02}, nil, TxInfo{})
				require.Error(tt, err)
				_, ok := err.(ErrMempoolIsFull)
				require.True(t, ok)
			},
		},
		{
			name: "normal mempool is full add tx failed, enableDeleteMinGPTx",
			prepare: func(mempool *CListMempool, tt *testing.T) {
				mempool.Flush()
				err := mempool.CheckTx([]byte{0x02}, nil, TxInfo{})
				require.NoError(tt, err)
				moc := cfg.MockDynamicConfig{}
				moc.SetEnableDeleteMinGPTx(true)
				cfg.SetDynamicConfig(moc)
			},
			execFunc: func(mempool *CListMempool, tt *testing.T) {
				err := mempool.CheckTx([]byte{0x03}, nil, TxInfo{})
				require.NoError(tt, err)
				require.Equal(tt, 2, mempool.Size())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			app := kvstore.NewApplication()
			cc := proxy.NewLocalClientCreator(app)
			mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
			mempool.config.MaxTxsBytes = 1 //  in unit test we only use tx bytes to  control mempool weather full
			defer cleanup()

			requireHeapQueue(t, mempool.txs)

			tc.prepare(mempool, tt)
			tc.execFunc(mempool, tt)
		})
	}

}

func TestConsumePendingtxConcurrency_HeapQueue(t *testing.T) {

	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mem, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()
	requireHeapQueue(t, mem.txs)
	mem.pendingPool = newPendingPool(500000, 3, 10, 500000)

	for i := 0; i < 10000; i++ {
		mem.pendingPool.addTx(&mempoolTx{height: 1, gasWanted: 1, tx: []byte(strconv.Itoa(i)), from: "1", realTx: abci.MockTx{GasPrice: big.NewInt(3780), Nonce: uint64(i)}})
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	startWg := &sync.WaitGroup{}
	startWg.Add(1)
	go func() {
		startWg.Wait()
		mem.consumePendingTx("1", 0)
		wg.Done()
	}()
	startWg.Done()
	mem.consumePendingTx("1", 5000)
	wg.Wait()
	require.Equal(t, 0, mem.pendingPool.Size())
}

func TestSerialRechecktTx_HeapQueue(t *testing.T) {
	app := counter.NewApplication(true)
	app.SetOption(abci.RequestSetOption{Key: "serial", Value: "on"})
	cc := proxy.NewLocalClientCreator(app)

	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	mempool.config.MaxTxNumPerBlock = 10000

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err := appConnCon.Start()
	require.Nil(t, err)

	cacheMap := make(map[string]struct{})
	deliverTxsRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {

			// This will succeed
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			err := mempool.CheckTx(txBytes, nil, TxInfo{})
			_, cached := cacheMap[string(txBytes)]
			if cached {
				require.NotNil(t, err, "expected error for cached tx")
			} else {
				require.Nil(t, err, "expected no err for uncached tx")
			}
			cacheMap[string(txBytes)] = struct{}{}

			// Duplicates are cached and should return error
			err = mempool.CheckTx(txBytes, nil, TxInfo{})
			require.NotNil(t, err, "Expected error after CheckTx on duplicated tx")
		}
	}

	reapCheck := func(exp int) {
		txs := mempool.ReapMaxTxs(exp)
		require.Equal(t, len(txs), exp, fmt.Sprintf("Expected to reap %v txs but got %v", exp, len(txs)))
	}

	updateRange := func(start, end int) {
		txs := make([]types.Tx, 0)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			txs = append(txs, txBytes)
		}
		if err := mempool.Update(0, txs, abciResponses(len(txs), abci.CodeTypeOK), nil, nil); err != nil {
			t.Error(err)
		}
	}

	commitRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
			if err != nil {
				t.Errorf("client error committing tx: %v", err)
			}
			if res.IsErr() {
				t.Errorf("error committing tx. Code:%v result:%X log:%v",
					res.Code, res.Data, res.Log)
			}
		}
		res, err := appConnCon.CommitSync(abci.RequestCommit{})
		if err != nil {
			t.Errorf("client error committing: %v", err)
		}
		if len(res.Data) != 8 {
			t.Errorf("error committing. Hash:%X", res.Data)
		}
	}

	makeInvalidTx := func(start, end int) {
		hq := mempool.txs.(*HeapQueue)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			vaule, ok := hq.txsMap.Load(txKey(txBytes))
			require.True(t, ok)
			vaule.(*clist.CElement).Value.(*mempoolTx).tx = make([]byte, 10)
		}
	}

	//----------------------------------------

	// Deliver 0 to 999, we should reap 900 new txs
	// because 100 were already counted.
	deliverTxsRange(0, BlockMaxTxNum+10)

	// Commit from the conensus AppConn
	commitRange(0, BlockMaxTxNum)
	updateRange(0, BlockMaxTxNum)

	reapCheck(10)
	num := 0
	mempool.recheckHeap.Range(func(key, value interface{}) bool {
		num++
		return true
	})
	require.Equal(t, num, 0)
	require.Equal(t, mempool.recheckHeapSize, int64(0))

	makeInvalidTx(BlockMaxTxNum+2, BlockMaxTxNum+10)
	updateRange(BlockMaxTxNum, BlockMaxTxNum+2)

	num = 0
	mempool.recheckHeap.Range(func(key, value interface{}) bool {
		num++
		return true
	})
	require.Equal(t, num, 0)
	require.Equal(t, mempool.recheckHeapSize, int64(0))
}

func TestSerialFlush_HeapQueue(t *testing.T) {
	app := counter.NewApplication(true)
	app.SetOption(abci.RequestSetOption{Key: "serial", Value: "on"})
	cc := proxy.NewLocalClientCreator(app)

	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	mempool.config.MaxTxNumPerBlock = 10000

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err := appConnCon.Start()
	require.Nil(t, err)

	cacheMap := make(map[string]struct{})
	deliverTxsRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {

			// This will succeed
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			err := mempool.CheckTx(txBytes, nil, TxInfo{})
			_, cached := cacheMap[string(txBytes)]
			if cached {
				require.NotNil(t, err, "expected error for cached tx")
			} else {
				require.Nil(t, err, "expected no err for uncached tx")
			}
			cacheMap[string(txBytes)] = struct{}{}

			// Duplicates are cached and should return error
			err = mempool.CheckTx(txBytes, nil, TxInfo{})
			require.NotNil(t, err, "Expected error after CheckTx on duplicated tx")
		}
	}

	//----------------------------------------

	// Deliver 0 to 999, we should reap 900 new txs
	// because 100 were already counted.
	deliverTxsRange(0, 1000)

	mempool.Flush()
	require.Equal(t, mempool.Size(), 0)
}

func TestDeleteMinGPTx_HeapQueue(t *testing.T) {
	app := counter.NewApplication(true)
	app.SetOption(abci.RequestSetOption{Key: "serial", Value: "on"})
	cc := proxy.NewLocalClientCreator(app)

	mempool, cleanup := newMempoolWithAppForHeapQueue(cc)
	defer cleanup()

	requireHeapQueue(t, mempool.txs)
	mempool.config.MaxTxNumPerBlock = 10000

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err := appConnCon.Start()
	require.Nil(t, err)

	cacheMap := make(map[string]struct{})
	deliverTxsRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {

			// This will succeed
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			err := mempool.CheckTx(txBytes, nil, TxInfo{})
			_, cached := cacheMap[string(txBytes)]
			if cached {
				require.NotNil(t, err, "expected error for cached tx")
			} else {
				require.Nil(t, err, "expected no err for uncached tx")
			}
			cacheMap[string(txBytes)] = struct{}{}

			// Duplicates are cached and should return error
			err = mempool.CheckTx(txBytes, nil, TxInfo{})
			require.NotNil(t, err, "Expected error after CheckTx on duplicated tx")
		}
	}

	reapCheck := func(exp int) {
		txs := mempool.ReapMaxTxs(exp)
		require.Equal(t, len(txs), exp, fmt.Sprintf("Expected to reap %v txs but got %v", exp, len(txs)))
	}

	updateRange := func(start, end int) {
		txs := make([]types.Tx, 0)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			txs = append(txs, txBytes)
		}
		if err := mempool.Update(0, txs, abciResponses(len(txs), abci.CodeTypeOK), nil, nil); err != nil {
			t.Error(err)
		}
	}

	commitRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
			if err != nil {
				t.Errorf("client error committing tx: %v", err)
			}
			if res.IsErr() {
				t.Errorf("error committing tx. Code:%v result:%X log:%v",
					res.Code, res.Data, res.Log)
			}
		}
		res, err := appConnCon.CommitSync(abci.RequestCommit{})
		if err != nil {
			t.Errorf("client error committing: %v", err)
		}
		if len(res.Data) != 8 {
			t.Errorf("error committing. Hash:%X", res.Data)
		}
	}

	makeInvalidTx := func(start, end int) {
		hq := mempool.txs.(*HeapQueue)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			vaule, ok := hq.txsMap.Load(txKey(txBytes))
			require.True(t, ok)
			vaule.(*clist.CElement).Value.(*mempoolTx).tx = make([]byte, 10)
		}
	}

	//----------------------------------------

	// Deliver 0 to 999, we should reap 900 new txs
	// because 100 were already counted.
	deliverTxsRange(0, BlockMaxTxNum+10)

	// Commit from the conensus AppConn
	commitRange(0, BlockMaxTxNum)
	updateRange(0, BlockMaxTxNum)

	reapCheck(10)
	num := 0
	mempool.recheckHeap.Range(func(key, value interface{}) bool {
		num++
		return true
	})
	require.Equal(t, num, 0)
	require.Equal(t, mempool.recheckHeapSize, int64(0))

	makeInvalidTx(BlockMaxTxNum+2, BlockMaxTxNum+10)
	updateRange(BlockMaxTxNum, BlockMaxTxNum+2)

	num = 0
	mempool.recheckHeap.Range(func(key, value interface{}) bool {
		num++
		return true
	})
	require.Equal(t, num, 0)
	require.Equal(t, mempool.recheckHeapSize, int64(0))
}
