package cachekv

import (
	"bytes"
	"io"
	"sort"
	"sync"
	"unsafe"

	"github.com/okex/exchain/libs/iavl"
	"github.com/okex/exchain/libs/system/trace"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/tendermint/go-amino"

	"github.com/okex/exchain/libs/cosmos-sdk/store/tracekv"
	"github.com/okex/exchain/libs/cosmos-sdk/store/types"
	kv "github.com/okex/exchain/libs/cosmos-sdk/types/kv"
)

// If value is nil but deleted is false, it means the parent doesn't have the
// key.  (No need to delete upon Write())
type cValue struct {
	value   []byte
	deleted bool
}

type PreChangesHandler func(keys []string, setOrDel []byte)

// Store wraps an in-memory cache around an underlying types.KVStore.
type Store struct {
	mtx           sync.Mutex
	dirty         map[string]cValue
	readList      map[string][]byte
	unsortedCache map[string]struct{}
	sortedCache   *kv.List // always ascending sorted
	parent        types.KVStore

	preChangesHandler    PreChangesHandler
	disableCacheReadList bool // not cache readList for group-paralleled-tx
	trace.StatisticsCell
}

var _ types.CacheKVStore = (*Store)(nil)

func NewStore(parent types.KVStore) *Store {
	return &Store{
		dirty:         make(map[string]cValue),
		readList:      make(map[string][]byte),
		unsortedCache: make(map[string]struct{}),
		sortedCache:   kv.NewList(),
		parent:        parent,
	}
}

func NewStoreWithPreChangeHandler(parent types.KVStore, preChangesHandler PreChangesHandler) *Store {
	s := NewStore(parent)
	s.preChangesHandler = preChangesHandler
	return s
}

// Implements Store.
func (store *Store) GetStoreType() types.StoreType {
	return store.parent.GetStoreType()
}

// Implements types.KVStore.
func (store *Store) Get(key []byte) (value []byte) {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	types.AssertValidKey(key)

	cacheValue, ok := store.dirty[string(key)]
	if !ok {
		if c, ok := store.readList[string(key)]; ok {
			value = c
		} else {
			value = store.parent.Get(key)
			if !store.disableCacheReadList {
				store.setCacheValue(key, value, false, false)
			}
		}
	} else {
		value = cacheValue.value
	}

	return value
}

func (store *Store) GetRWSet(mp types.MsRWSet) {
	panic("implement me")
}

func (store *Store) IteratorCache(isdirty bool, cb func(key string, value []byte, isDirty bool, isDelete bool, sKey types.StoreKey) bool, sKey types.StoreKey) bool {
	if cb == nil {
		return true
	}
	store.mtx.Lock()
	defer store.mtx.Unlock()

	if isdirty {
		for key, v := range store.dirty {
			if !cb(key, v.value, true, v.deleted, sKey) {
				return false
			}
		}
	} else {
		for key, v := range store.readList {
			if !cb(key, v, false, false, sKey) {
				return false
			}
		}
	}

	return true
}

// Implements types.KVStore.
func (store *Store) Set(key []byte, value []byte) {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	types.AssertValidKey(key)
	types.AssertValidValue(value)

	store.setCacheValue(key, value, false, true)
}

// Implements types.KVStore.
func (store *Store) Has(key []byte) bool {
	value := store.Get(key)
	return value != nil
}

// Implements types.KVStore.
func (store *Store) Delete(key []byte) {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	types.AssertValidKey(key)

	store.setCacheValue(key, nil, true, true)
}

// Implements Cachetypes.KVStore.
func (store *Store) WriteWithSnapshotWSet() types.SnapshotWSet {
	// if parent is cachekv.Store, we can write kv more efficiently
	if pStore, ok := store.parent.(*Store); ok {
		return store.writeToCacheKvWithSnapShotWSet(pStore)
	}

	store.mtx.Lock()
	defer store.mtx.Unlock()

	// We need a copy of all of the keys.
	// Not the best, but probably not a bottleneck depending.
	keys := make([]string, len(store.dirty))
	index := 0
	for key, _ := range store.dirty {
		keys[index] = key
		index++

	}

	sort.Strings(keys)
	store.preWrite(keys)

	store.StartTiming()

	swset := types.NewSnapShotWSet()
	// TODO: Consider allowing usage of Batch, which would allow the write to
	// at least happen atomically.
	for _, key := range keys {
		//set prevalue
		swset.Write[key] = types.RevertWriteChange{PrevValue: store.parent.Get([]byte(key))}
		cacheValue := store.dirty[key]
		switch {
		case cacheValue.deleted:
			store.parent.Delete([]byte(key))
		case cacheValue.value == nil:
			// Skip, it already doesn't exist in parent.
		default:
			store.parent.Set([]byte(key), cacheValue.value)
		}
	}

	// Clear the cache
	store.clearCache()
	store.EndTiming(trace.FlushCache)
	return swset
}

// Implements Cachetypes.KVStore.
func (store *Store) RevertDBWithSnapshotRWSet(set types.SnapshotWSet) {
	types.RevertSnapshotWSet(store, set)
}

// Implements Cachetypes.KVStore.
func (store *Store) Write() {
	// if parent is cachekv.Store, we can write kv more efficiently
	if pStore, ok := store.parent.(*Store); ok {
		store.writeToCacheKv(pStore)
		return
	}

	store.mtx.Lock()
	defer store.mtx.Unlock()

	// We need a copy of all of the keys.
	// Not the best, but probably not a bottleneck depending.
	keys := make([]string, len(store.dirty))
	index := 0
	for key, _ := range store.dirty {
		keys[index] = key
		index++

	}

	sort.Strings(keys)
	store.preWrite(keys)

	store.StartTiming()

	// TODO: Consider allowing usage of Batch, which would allow the write to
	// at least happen atomically.
	for _, key := range keys {
		cacheValue := store.dirty[key]
		switch {
		case cacheValue.deleted:
			store.parent.Delete([]byte(key))
		case cacheValue.value == nil:
			// Skip, it already doesn't exist in parent.
		default:
			store.parent.Set([]byte(key), cacheValue.value)
		}
	}

	// Clear the cache
	store.clearCache()
	store.EndTiming(trace.FlushCache)
}

func (store *Store) preWrite(keys []string) {
	if store.preChangesHandler == nil || len(keys) < 4 {
		return
	}

	setOrDel := make([]byte, 0, len(keys))

	for _, key := range keys {
		cacheValue := store.dirty[key]
		switch {
		case cacheValue.deleted:
			setOrDel = append(setOrDel, iavl.PreChangeOpDelete)
		case cacheValue.value == nil:
			// Skip, it already doesn't exist in parent.
			setOrDel = append(setOrDel, iavl.PreChangeNop)
		default:
			setOrDel = append(setOrDel, iavl.PreChangeOpSet)
		}
	}

	store.preChangesHandler(keys, setOrDel)
}

// writeToCacheKv will write cached kv to the parent Store, then clear the cache.
func (store *Store) writeToCacheKv(parent *Store) {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	// TODO: Consider allowing usage of Batch, which would allow the write to
	// at least happen atomically.
	for key, cacheValue := range store.dirty {
		switch {
		case cacheValue.deleted:
			parent.Delete(amino.StrToBytes(key))
		case cacheValue.value == nil:
			// Skip, it already doesn't exist in parent.
		default:
			parent.Set(amino.StrToBytes(key), cacheValue.value)
		}
	}

	// Clear the cache
	store.clearCache()
}

// writeToCacheKv will write cached kv to the parent Store, then clear the cache.
func (store *Store) writeToCacheKvWithSnapShotWSet(parent *Store) types.SnapshotWSet {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	// TODO: Consider allowing usage of Batch, which would allow the write to
	// at least happen atomically.
	swset := types.NewSnapShotWSet()
	for key, cacheValue := range store.dirty {
		swset.Write[key] = types.RevertWriteChange{PrevValue: store.parent.Get([]byte(key))}
		switch {
		case cacheValue.deleted:
			parent.Delete(amino.StrToBytes(key))
		case cacheValue.value == nil:
			// Skip, it already doesn't exist in parent.
		default:
			parent.Set(amino.StrToBytes(key), cacheValue.value)
		}
	}

	// Clear the cache
	store.clearCache()
	return swset
}

func (store *Store) clearCache() {
	// https://github.com/golang/go/issues/20138
	for key := range store.dirty {
		delete(store.dirty, key)
	}

	for Key := range store.readList {
		delete(store.readList, Key)
	}
	for key := range store.unsortedCache {
		delete(store.unsortedCache, key)
	}
	store.disableCacheReadList = false
	store.sortedCache.Init()
}

//----------------------------------------
// To cache-wrap this Store further.

// Implements CacheWrapper.
func (store *Store) CacheWrap() types.CacheWrap {
	return NewStore(store)
}

// CacheWrapWithTrace implements the CacheWrapper interface.
func (store *Store) CacheWrapWithTrace(w io.Writer, tc types.TraceContext) types.CacheWrap {
	return NewStore(tracekv.NewStore(store, w, tc))
}

//----------------------------------------
// Iteration

// Implements types.KVStore.
func (store *Store) Iterator(start, end []byte) types.Iterator {
	return store.iterator(start, end, true)
}

// Implements types.KVStore.
func (store *Store) ReverseIterator(start, end []byte) types.Iterator {
	return store.iterator(start, end, false)
}

func (store *Store) iterator(start, end []byte, ascending bool) types.Iterator {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	var parent, cache types.Iterator

	if ascending {
		parent = store.parent.Iterator(start, end)
	} else {
		parent = store.parent.ReverseIterator(start, end)
	}

	store.dirtyItems(start, end)
	cache = newMemIterator(start, end, store.sortedCache, ascending)

	return newCacheMergeIterator(parent, cache, ascending)
}

// strToByte is meant to make a zero allocation conversion
// from string -> []byte to speed up operations, it is not meant
// to be used generally, but for a specific pattern to check for available
// keys within a domain.
func strToByte(s string) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.StringData(s))), len(s))
}

// byteSliceToStr is meant to make a zero allocation conversion
// from []byte -> string to speed up operations, it is not meant
// to be used generally, but for a specific pattern to delete keys
// from a map.
func byteSliceToStr(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Constructs a slice of dirty items, to use w/ memIterator.
func (store *Store) dirtyItems(start, end []byte) {
	unsorted := make([]*kv.Pair, 0)

	n := len(store.unsortedCache)
	for key := range store.unsortedCache {
		if dbm.IsKeyInDomain(strToByte(key), start, end) {
			cacheValue := store.dirty[key]
			unsorted = append(unsorted, &kv.Pair{Key: []byte(key), Value: cacheValue.value})
		}
	}

	if len(unsorted) == n { // This pattern allows the Go compiler to emit the map clearing idiom for the entire map.
		for key := range store.unsortedCache {
			delete(store.unsortedCache, key)
		}
	} else { // Otherwise, normally delete the unsorted keys from the map.
		for _, kv := range unsorted {
			delete(store.unsortedCache, byteSliceToStr(kv.Key))
		}
	}

	sort.Slice(unsorted, func(i, j int) bool {
		return bytes.Compare(unsorted[i].Key, unsorted[j].Key) < 0
	})

	for e := store.sortedCache.Front(); e != nil && len(unsorted) != 0; {
		uitem := unsorted[0]
		sitem := e.Value
		comp := bytes.Compare(uitem.Key, sitem.Key)
		switch comp {
		case -1:
			unsorted = unsorted[1:]
			store.sortedCache.InsertBefore(uitem, e)
		case 1:
			e = e.Next()
		case 0:
			unsorted = unsorted[1:]
			e.Value = uitem
			e = e.Next()
		}
	}

	for _, kvp := range unsorted {
		store.sortedCache.PushBack(kvp)
	}

}

//----------------------------------------
// etc

// Only entrypoint to mutate store.cache.
func (store *Store) setCacheValue(key, value []byte, deleted bool, dirty bool) {
	keyStr := string(key)
	if !dirty {
		store.readList[keyStr] = value
		return
	}

	store.dirty[keyStr] = cValue{
		value:   value,
		deleted: deleted,
	}
	if dirty {
		store.unsortedCache[keyStr] = struct{}{}
	}
}

// Reset will clear all internal data without writing to the parent and set the new parent.
func (store *Store) Reset(parent types.KVStore) {
	store.mtx.Lock()

	store.preChangesHandler = nil
	store.parent = parent
	store.StatisticsCell = nil
	store.clearCache()

	store.mtx.Unlock()
}

// Clear will clear all internal data without writing to the parent.
func (store *Store) Clear() {
	store.mtx.Lock()
	store.clearCache()
	store.mtx.Unlock()
}

func (store *Store) DisableCacheReadList() {
	store.mtx.Lock()
	store.disableCacheReadList = true
	store.mtx.Unlock()
}

func (store *Store) StartTiming() {
	if store.StatisticsCell != nil {
		store.StatisticsCell.StartTiming()
	}
}

func (store *Store) EndTiming(tag string) {
	if store.StatisticsCell != nil {
		store.StatisticsCell.EndTiming(tag)
	}
}

func (store *Store) CopyRWSet(rw types.CacheKVRWSet) {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	for k, v := range store.readList {
		rw.Read[k] = v
	}

	for k, v := range store.dirty {
		rw.Write[k] = types.DirtyValue{
			Deleted: v.deleted,
			Value:   v.value,
		}
	}
}
