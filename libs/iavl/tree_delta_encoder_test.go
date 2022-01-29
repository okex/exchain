package iavl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/okex/exchain/libs/tendermint/types"
)

var testTreeDeltaMap = []TreeDeltaMap{
	//empty
	{
		"test0": {},
	},
	//empty NodesDelta
	{
		"test1": {
			NodesDelta:         []*NodeJsonImp{},
			OrphansDelta:       []*NodeJson{{Version: 3}, {Version: 4}},
			CommitOrphansDelta: []*CommitOrphansImp{{"nd1", 1}, {"nd2", 2}},
		},
	},
	//empty OrphansDelta
	{
		"test2": {
			NodesDelta: []*NodeJsonImp{
				{"nd1", &NodeJson{Version: 1}},
				{"nd2", &NodeJson{Version: 2}},
			},
			OrphansDelta:       []*NodeJson{},
			CommitOrphansDelta: []*CommitOrphansImp{{"nd1", 1}, {"nd2", 2}},
		},
	},
	//empty CommitOrphansDelta
	{
		"test3": {
			NodesDelta: []*NodeJsonImp{
				{"nd1", &NodeJson{Version: 1}},
				{"nd2", &NodeJson{Version: 2}},
			},
			OrphansDelta:       []*NodeJson{{Version: 3}, {Version: 4}},
			CommitOrphansDelta: []*CommitOrphansImp{},
		},
	},
	// full data
	{
		"test4": {
			NodesDelta: []*NodeJsonImp{
				{"nd1", &NodeJson{Version: 1}},
				{"nd2", &NodeJson{Version: 2}},
			},
			OrphansDelta:       []*NodeJson{{Version: 3}, {Version: 4}},
			CommitOrphansDelta: []*CommitOrphansImp{{"nd1", 1}, {"nd2", 2}},
		},
	},
	// multiple data
	{
		"test5.0": {
			NodesDelta: []*NodeJsonImp{
				{"nd1", &NodeJson{Version: 1}},
				{"nd2", &NodeJson{Version: 2}},
			},
			OrphansDelta:       []*NodeJson{{Version: 3}, {Version: 4}},
			CommitOrphansDelta: []*CommitOrphansImp{{"nd1", 1}, {"nd2", 2}},
		},
		"test5.1": {
			NodesDelta: []*NodeJsonImp{
				{"nd3", &NodeJson{Version: 3}},
			},
			OrphansDelta:       []*NodeJson{{Version: 5}},
			CommitOrphansDelta: []*CommitOrphansImp{{"nd1", 3}},
		},
	},
}

func newTestTreeDeltaMap() TreeDeltaMap {
	return testTreeDeltaMap[5]
}

func TestTreeDeltaAmino(t *testing.T) { testTreeDeltaAmino(t) }
func testTreeDeltaAmino(t *testing.T) {
	for i, tdm := range testTreeDeltaMap {
		// each tree delta
		for _, td := range tdm {
			expect, err := cdc.MarshalBinaryBare(td)
			require.NoError(t, err, fmt.Sprintf("num %v", i))

			actual, err := td.MarshalToAmino(cdc)
			require.NoError(t, err, fmt.Sprintf("num %v", i))
			require.EqualValues(t, expect, actual, fmt.Sprintf("num %v", i))

			var expectValue TreeDelta
			err = cdc.UnmarshalBinaryBare(expect, &expectValue)
			require.NoError(t, err, fmt.Sprintf("num %v", i))

			var actualValue TreeDelta
			err = actualValue.UnmarshalFromAmino(cdc, expect)
			require.NoError(t, err, fmt.Sprintf("num %v", i))
			require.EqualValues(t, expectValue, actualValue, fmt.Sprintf("num %v", i))
		}
	}
}

// benchmark encode performance
func BenchmarkAminoEncodeDelta(b *testing.B) { benchmarkEncodeDelta(b, newEncoder("amino")) }
func BenchmarkJsonEncodeDelta(b *testing.B)  { benchmarkEncodeDelta(b, newEncoder("json")) }
func benchmarkEncodeDelta(b *testing.B, enc encoder) {
	data := newTestTreeDeltaMap()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.encodeFunc(data)
	}

}

// benchmark decode performance
func BenchmarkAminoDecodeDelta(b *testing.B) { benchmarkDecodeDelta(b, newEncoder("amino")) }
func BenchmarkJsonDecodeDelta(b *testing.B)  { benchmarkDecodeDelta(b, newEncoder("json")) }
func benchmarkDecodeDelta(b *testing.B, enc encoder) {
	deltaList1 := newTestTreeDeltaMap()
	data, _ := enc.encodeFunc(deltaList1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.decodeFunc(data)
	}
}

type encoder interface {
	name() string
	encodeFunc(TreeDeltaMap) ([]byte, error)
	decodeFunc([]byte) (TreeDeltaMap, error)
}

func newEncoder(encType string) encoder {
	switch encType {
	case "amino":
		return &aminoEncoder{}
	case "json":
		return &jsonEncoder{}
	default:
	}
	panic("unsupport encoder")
}

// amino encoder
type aminoEncoder struct{}

func (ae *aminoEncoder) name() string { return "amino" }
func (ae *aminoEncoder) encodeFunc(data TreeDeltaMap) ([]byte, error) {
	return data.MarshalToAmino(nil)
}
func (ae *aminoEncoder) decodeFunc(data []byte) (TreeDeltaMap, error) {
	deltaList := TreeDeltaMap{}
	err := deltaList.UnmarshalFromAmino(nil, data)
	return deltaList, err
}

// json encoder
type jsonEncoder struct{}

func (je *jsonEncoder) name() string { return "json" }
func (je *jsonEncoder) encodeFunc(data TreeDeltaMap) ([]byte, error) {
	return types.Json.Marshal(data)
}
func (je *jsonEncoder) decodeFunc(data []byte) (TreeDeltaMap, error) {
	deltaList := TreeDeltaMap{}
	err := types.Json.Unmarshal(data, &deltaList)
	return deltaList, err
}
