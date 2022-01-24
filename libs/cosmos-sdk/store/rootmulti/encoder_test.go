package rootmulti

import (
	"testing"

	"github.com/okex/exchain/libs/cosmos-sdk/store/types"
	iavltree "github.com/okex/exchain/libs/iavl"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
)

func newTestTreeDelta() map[string]iavltree.TreeDelta {
	// new multiplex store
	var db dbm.DB = dbm.NewMemDB()
	ms := newMultiStoreWithMounts(db, types.PruneNothing)
	ms.LoadLatestVersion()

	// set value into store map
	k, v := []byte("wind"), []byte("blows")
	k1, v1 := []byte("key1"), []byte("val1")
	k2, v2 := []byte("key2"), []byte("val2")
	store1 := ms.getStoreByName("store1").(types.KVStore)
	store1.Set(k, v)
	store1.Set(k1, v1)
	store2 := ms.getStoreByName("store2").(types.KVStore)
	store2.Set(k2, v2)

	// each store to be committed and return its delta
	returnedDeltas := make(map[string]iavltree.TreeDelta)
	for key, store := range ms.stores {
		_, reDelta, _ := store.Commit(nil, nil)
		if store.GetStoreType() == types.StoreTypeTransient {
			continue
		}
		returnedDeltas[key.Name()] = reDelta
	}
	return returnedDeltas
}

// test encode function
func TestAminoEncodeDelta(t *testing.T) { testEncodeTreeDelta(t, newEncoder("amino")) }
func TestJsonEncodeDelta(t *testing.T)  { testEncodeTreeDelta(t, newEncoder("json")) }
func testEncodeTreeDelta(t *testing.T, enc encoder) {
	deltaList := newTestTreeDelta()

	_, err := enc.encodeFunc(deltaList)
	require.NoError(t, err, enc.name())
}

// benchmark encode performance
func BenchmarkAminoEncodeDelta(b *testing.B) { benchmarkEncodeDelta(b, newEncoder("amino")) }
func BenchmarkJsonEncodeDelta(b *testing.B)  { benchmarkEncodeDelta(b, newEncoder("json")) }
func benchmarkEncodeDelta(b *testing.B, enc encoder) {
	data := newTestTreeDelta()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.encodeFunc(data)
	}

}

// test decode function
func TestAminoDecodeDelta(t *testing.T) { testDecodeTreeDelta(t, newEncoder("amino")) }
func TestJsonDecodeDelta(t *testing.T)  { testDecodeTreeDelta(t, newEncoder("json")) }
func testDecodeTreeDelta(t *testing.T, enc encoder) {
	deltaList1 := newTestTreeDelta()
	data, err := enc.encodeFunc(deltaList1)

	_, err = enc.decodeFunc(data)
	require.NoError(t, err, enc.name())
}

// benchmark decode performance
func BenchmarkAminoDecodeDelta(b *testing.B) { benchmarkDecodeDelta(b, newEncoder("amino")) }
func BenchmarkJsonDecodeDelta(b *testing.B)  { benchmarkDecodeDelta(b, newEncoder("json")) }
func benchmarkDecodeDelta(b *testing.B, enc encoder) {
	deltaList1 := newTestTreeDelta()
	data, _ := enc.encodeFunc(deltaList1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.decodeFunc(data)
	}
}

type encoder interface {
	name() string
	encodeFunc(map[string]iavltree.TreeDelta) ([]byte, error)
	decodeFunc([]byte) (map[string]*iavltree.TreeDelta, error)
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
func (ae *aminoEncoder) encodeFunc(data map[string]iavltree.TreeDelta) ([]byte, error) {
	return MarshalAppliedDeltaToAmino(data)
}
func (ae *aminoEncoder) decodeFunc(data []byte) (map[string]*iavltree.TreeDelta, error) {
	return UnmarshalAppliedDeltaFromAmino(data)
}

// json encoder
type jsonEncoder struct{}

func (je *jsonEncoder) name() string { return "json" }
func (je *jsonEncoder) encodeFunc(data map[string]iavltree.TreeDelta) ([]byte, error) {
	return itjs.Marshal(data)
}
func (je *jsonEncoder) decodeFunc(data []byte) (map[string]*iavltree.TreeDelta, error) {
	deltalist := map[string]*iavltree.TreeDelta{}
	err := itjs.Unmarshal(data, &deltalist)
	return deltalist, err
}
