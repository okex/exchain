package iavl

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/okex/exchain/libs/cosmos-sdk/store/cachekv"
	"github.com/okex/exchain/libs/cosmos-sdk/store/flatkv"
	"github.com/okex/exchain/libs/cosmos-sdk/store/tracekv"
	"github.com/okex/exchain/libs/cosmos-sdk/store/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/iavl"
	iavlconfig "github.com/okex/exchain/libs/iavl/config"
	"github.com/okex/exchain/libs/system/trace/persist"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto/merkle"
	tmkv "github.com/okex/exchain/libs/tendermint/libs/kv"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	dbm "github.com/okex/exchain/libs/tm-db"
)

var (
	FlagIavlCacheSize = "iavl-cache-size"

	IavlCacheSize = 1000000
)

var (
	_ types.KVStore       = (*Store)(nil)
	_ types.CommitStore   = (*Store)(nil)
	_ types.CommitKVStore = (*Store)(nil)
	_ types.Queryable     = (*Store)(nil)
)

// Store Implements types.KVStore and CommitKVStore.
type Store struct {
	tree        Tree
	flatKVStore *flatkv.Store
	//for upgrade
	upgradeVersion int64
	//for time statistics
	beginTime time.Time
}

func (st *Store) CurrentVersion() int64 {
	tr := st.tree.(*iavl.MutableTree)
	return tr.Version()
}
func (st *Store) StopStoreWithVersion(version int64) {
	tr := st.tree.(*iavl.MutableTree)
	tr.StopTree()
}
func (st *Store) StopStore() {
	tr := st.tree.(*iavl.MutableTree)
	tr.StopTree()
}

func (st *Store) GetHeights() map[int64][]byte {
	return st.tree.GetPersistedRoots()
}

// LoadStore returns an IAVL Store as a CommitKVStore. Internally, it will load the
// store's version (id) from the provided DB. An error is returned if the version
// fails to load.
func LoadStore(db dbm.DB, flatKVDB dbm.DB, id types.CommitID, lazyLoading bool, startVersion int64) (types.CommitKVStore, error) {
	return LoadStoreWithInitialVersion(db, flatKVDB, id, lazyLoading, uint64(startVersion), uint64(0))
}

// LoadStoreWithInitialVersion returns an IAVL Store as a CommitKVStore setting its initialVersion
// to the one given. Internally, it will load the store's version (id) from the
// provided DB. An error is returned if the version fails to load.
func LoadStoreWithInitialVersion(db dbm.DB, flatKVDB dbm.DB, id types.CommitID, lazyLoading bool, initialVersion uint64, upgradeVersion uint64) (types.CommitKVStore, error) {
	tree, err := iavl.NewMutableTreeWithOpts(db, iavlconfig.DynamicConfig.GetIavlCacheSize(), &iavl.Options{InitialVersion: initialVersion, UpgradeVersion: upgradeVersion})
	if err != nil {
		return nil, err
	}
	if lazyLoading {
		_, err = tree.LazyLoadVersion(id.Version)
	} else {
		_, err = tree.LoadVersion(id.Version)
	}

	if err != nil {
		return nil, err
	}

	st := &Store{
		tree:           tree,
		flatKVStore:    flatkv.NewStore(flatKVDB),
		upgradeVersion: -1,
	}

	if err = st.ValidateFlatVersion(); err != nil {
		return nil, err
	}

	return st, nil
}
func HasVersion(db dbm.DB, version int64) (bool, error) {
	tree, err := iavl.NewMutableTreeWithOpts(db, iavlconfig.DynamicConfig.GetIavlCacheSize(), &iavl.Options{InitialVersion: 0})
	if err != nil {
		return false, err
	}
	return tree.VersionExistsInDb(version), nil
}
func GetCommitVersions(db dbm.DB) ([]int64, error) {
	tree, err := iavl.NewMutableTreeWithOpts(db, iavlconfig.DynamicConfig.GetIavlCacheSize(), &iavl.Options{InitialVersion: 0})
	if err != nil {
		return nil, err
	}
	return tree.GetVersions()
}

// UnsafeNewStore returns a reference to a new IAVL Store with a given mutable
// IAVL tree reference. It should only be used for testing purposes.
//
// CONTRACT: The IAVL tree should be fully loaded.
// CONTRACT: PruningOptions passed in as argument must be the same as pruning options
// passed into iavl.MutableTree
func UnsafeNewStore(tree *iavl.MutableTree) *Store {
	return &Store{
		tree:           tree,
		upgradeVersion: -1,
	}
}

// GetImmutable returns a reference to a new store backed by an immutable IAVL
// tree at a specific version (height) without any pruning options. This should
// be used for querying and iteration only. If the version does not exist or has
// been pruned, an empty immutable IAVL tree will be used.
// Any mutable operations executed will result in a panic.
func (st *Store) GetImmutable(version int64) (*Store, error) {
	var iTree *iavl.ImmutableTree
	var err error
	if !abci.GetDisableABCIQueryMutex() {
		if !st.VersionExists(version) {
			return nil, iavl.ErrVersionDoesNotExist
		}

		iTree, err = st.tree.GetImmutable(version)
		if err != nil {
			return nil, err
		}
	} else {
		iTree, err = st.tree.GetImmutable(version)
		if err != nil {
			return nil, iavl.ErrVersionDoesNotExist
		}
	}
	return &Store{
		tree: &immutableTree{iTree},
	}, nil
}

// GetEmptyImmutable returns an empty immutable IAVL tree
func (st *Store) GetEmptyImmutable() *Store {
	return &Store{tree: &immutableTree{&iavl.ImmutableTree{}}}
}

func (st *Store) CommitterCommit(inputDelta *iavl.TreeDelta) (types.CommitID, *iavl.TreeDelta) { // CommitterCommit
	flag := false
	if inputDelta != nil {
		flag = true
		st.tree.SetDelta(inputDelta)
	}
	ver := st.GetUpgradeVersion()
	if ver != -1 {
		st.tree.SetUpgradeVersion(ver)
		st.SetUpgradeVersion(-1)
	}
	hash, version, outputDelta, err := st.tree.SaveVersion(flag)
	if err != nil {
		panic(err)
	}

	// commit to flat kv db
	st.commitFlatKV(version)

	return types.CommitID{
		Version: version,
		Hash:    hash,
	}, &outputDelta
}

// Implements Committer.
func (st *Store) LastCommitID() types.CommitID {
	return types.CommitID{
		Version: st.tree.Version(),
		Hash:    st.tree.Hash(),
	}
}

func (st *Store) LastCommitVersion() int64 {
	return st.tree.Version()
}

// SetPruning panics as pruning options should be provided at initialization
// since IAVl accepts pruning options directly.
func (st *Store) SetPruning(_ types.PruningOptions) {
	panic("cannot set pruning options on an initialized IAVL store")
}

// VersionExists returns whether or not a given version is stored.
func (st *Store) VersionExists(version int64) bool {
	return st.tree.VersionExists(version)
}

// Implements Store.
func (st *Store) GetStoreType() types.StoreType {
	return types.StoreTypeIAVL
}

// Implements Store.
func (st *Store) CacheWrap() types.CacheWrap {
	stores := cachekv.NewStoreWithPreChangeHandler(st, st.tree.PreChanges)
	stores.StatisticsCell = st

	return stores
}

// CacheWrapWithTrace implements the Store interface.
func (st *Store) CacheWrapWithTrace(w io.Writer, tc types.TraceContext) types.CacheWrap {
	return cachekv.NewStore(tracekv.NewStore(st, w, tc))
}

// Implements types.KVStore.
func (st *Store) Set(key, value []byte) {
	types.AssertValidValue(value)
	st.tree.Set(key, value)
	st.setFlatKV(key, value)
}

// Implements types.KVStore.
func (st *Store) Get(key []byte) []byte {
	value := st.getFlatKV(key)
	if value != nil {
		return value
	}
	value = st.tree.Get(key)
	if value != nil {
		st.setFlatKV(key, value)
	}

	return value
}

// Implements types.KVStore.
func (st *Store) Has(key []byte) (exists bool) {
	if st.hasFlatKV(key) {
		return true
	}
	return st.tree.Has(key)
}

// Implements types.KVStore.
func (st *Store) Delete(key []byte) {
	st.tree.Remove(key)
	st.deleteFlatKV(key)
}

// DeleteVersions deletes a series of versions from the MutableTree. An error
// is returned if any single version is invalid or the delete fails. All writes
// happen in a single batch with a single commit.
func (st *Store) DeleteVersions(versions ...int64) error {
	return st.tree.DeleteVersions(versions...)
}

// Implements types.KVStore.
func (st *Store) Iterator(start, end []byte) types.Iterator {
	var iTree *iavl.ImmutableTree

	switch tree := st.tree.(type) {
	case *immutableTree:
		iTree = tree.ImmutableTree
	case *iavl.MutableTree:
		iTree = tree.ImmutableTree
	}

	return newIAVLIterator(iTree, start, end, true)
}

// Implements types.KVStore.
func (st *Store) ReverseIterator(start, end []byte) types.Iterator {
	var iTree *iavl.ImmutableTree

	switch tree := st.tree.(type) {
	case *immutableTree:
		iTree = tree.ImmutableTree
	case *iavl.MutableTree:
		iTree = tree.ImmutableTree
	}

	return newIAVLIterator(iTree, start, end, false)
}

// Handle gatest the latest height, if height is 0
func getHeight(tree Tree, req abci.RequestQuery) int64 {
	height := req.Height
	if height == 0 {
		latest := tree.Version()
		_, err := tree.GetImmutable(latest - 1)
		if err == nil {
			height = latest - 1
		} else {
			height = latest
		}
	}
	return height
}

// Query implements ABCI interface, allows queries
//
// by default we will return from (latest height -1),
// as we will have merkle proofs immediately (header height = data height + 1)
// If latest-1 is not present, use latest (which must be present)
// if you care to have the latest data to see a tx results, you must
// explicitly set the height you want to see
func (st *Store) Query(req abci.RequestQuery) (res abci.ResponseQuery) {
	if tmtypes.HigherThanVenus1(req.Height) {
		return st.queryWithCM40(req)
	}
	if len(req.Data) == 0 {
		return sdkerrors.QueryResult(sdkerrors.Wrap(sdkerrors.ErrTxDecode, "query cannot be zero length"))
	}

	// store the height we chose in the response, with 0 being changed to the
	// latest height
	res.Height = getHeight(st.tree, req)

	switch req.Path {
	case "/key": // get by key
		key := req.Data // data holds the key bytes
		res.Key = key

		tree, err := st.tree.GetImmutable(res.Height)
		if err != nil {
			return sdkerrors.QueryResult(sdkerrors.Wrapf(iavl.ErrVersionDoesNotExist, "request height %d", req.Height))
		}

		if req.Prove {
			value, proof, err := tree.GetWithProof(key)
			if err != nil {
				res.Log = err.Error()
				break
			}
			if proof == nil {
				// Proof == nil implies that the store is empty.
				if value != nil {
					panic("unexpected value for an empty proof")
				}
			}
			if value != nil {
				// value was found
				res.Value = value
				res.Proof = &merkle.Proof{Ops: []merkle.ProofOp{iavl.NewValueOp(key, proof).ProofOp()}}
			} else {
				// value wasn't found
				res.Value = nil
				res.Proof = &merkle.Proof{Ops: []merkle.ProofOp{iavl.NewAbsenceOp(key, proof).ProofOp()}}
			}
		} else {
			_, res.Value = tree.GetWithIndex(key)
		}

	case "/subspace":
		var KVs []types.KVPair

		subspace := req.Data
		res.Key = subspace

		iterator := types.KVStorePrefixIterator(st, subspace)
		for ; iterator.Valid(); iterator.Next() {
			KVs = append(KVs, types.KVPair{Key: iterator.Key(), Value: iterator.Value()})
		}

		iterator.Close()
		res.Value = cdc.MustMarshalBinaryLengthPrefixed(KVs)

	default:
		return sdkerrors.QueryResult(sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unexpected query path: %v", req.Path))
	}

	return res
}

func (st *Store) GetDBReadTime() int {
	return st.tree.GetDBReadTime()
}

func (st *Store) GetDBWriteCount() int {
	return st.tree.GetDBWriteCount()
}

func (st *Store) GetDBReadCount() int {
	return st.tree.GetDBReadCount()
}

func (st *Store) GetNodeReadCount() int {
	return st.tree.GetNodeReadCount()
}

func (st *Store) ResetCount() {
	st.tree.ResetCount()
	st.resetFlatKVCount()
}

func (st *Store) StartTiming() {
	st.beginTime = time.Now()
}

func (st *Store) EndTiming(tag string) {
	persist.GetStatistics().Accumulate(tag, st.beginTime)
}

//----------------------------------------

// Implements types.Iterator.
type iavlIterator struct {
	// Domain
	start, end []byte

	key   []byte // The current key (mutable)
	value []byte // The current value (mutable)

	// Underlying store
	tree *iavl.ImmutableTree

	// Channel to push iteration values.
	iterCh chan tmkv.Pair

	// Close this to release goroutine.
	quitCh chan struct{}

	// Close this to signal that state is initialized.
	initCh chan struct{}

	mtx sync.Mutex

	ascending bool // Iteration order

	invalid bool // True once, true forever (mutable)
}

var _ types.Iterator = (*iavlIterator)(nil)

// newIAVLIterator will create a new iavlIterator.
// CONTRACT: Caller must release the iavlIterator, as each one creates a new
// goroutine.
func newIAVLIterator(tree *iavl.ImmutableTree, start, end []byte, ascending bool) *iavlIterator {
	iter := &iavlIterator{
		tree:      tree,
		start:     types.Cp(start),
		end:       types.Cp(end),
		ascending: ascending,
		iterCh:    make(chan tmkv.Pair), // Set capacity > 0?
		quitCh:    make(chan struct{}),
		initCh:    make(chan struct{}),
	}
	go iter.iterateRoutine()
	go iter.initRoutine()
	return iter
}

// Run this to funnel items from the tree to iterCh.
func (iter *iavlIterator) iterateRoutine() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s, %v", time.Now(), r)
		}
	}()
	iter.tree.IterateRange(
		iter.start, iter.end, iter.ascending,
		func(key, value []byte) bool {
			select {
			case <-iter.quitCh:
				return true // done with iteration.
			case iter.iterCh <- tmkv.Pair{Key: key, Value: value}:
				return false // yay.
			}
		},
	)
	close(iter.iterCh) // done.
}

// Run this to fetch the first item.
func (iter *iavlIterator) initRoutine() {
	iter.receiveNext()
	close(iter.initCh)
}

// Implements types.Iterator.
func (iter *iavlIterator) Domain() (start, end []byte) {
	return iter.start, iter.end
}

// Implements types.Iterator.
func (iter *iavlIterator) Valid() bool {
	iter.waitInit()
	iter.mtx.Lock()

	validity := !iter.invalid
	iter.mtx.Unlock()
	return validity
}

// Implements types.Iterator.
func (iter *iavlIterator) Next() {
	iter.waitInit()
	iter.mtx.Lock()
	iter.assertIsValid(true)

	iter.receiveNext()
	iter.mtx.Unlock()
}

// Implements types.Iterator.
func (iter *iavlIterator) Key() []byte {
	iter.waitInit()
	iter.mtx.Lock()
	iter.assertIsValid(true)

	key := iter.key
	iter.mtx.Unlock()
	return key
}

// Implements types.Iterator.
func (iter *iavlIterator) Value() []byte {
	iter.waitInit()
	iter.mtx.Lock()
	iter.assertIsValid(true)

	val := iter.value
	iter.mtx.Unlock()
	return val
}

// Close closes the IAVL iterator by closing the quit channel and waiting for
// the iterCh to finish/close.
func (iter *iavlIterator) Close() {
	close(iter.quitCh)
	// wait iterCh to close
	for range iter.iterCh {
	}
}

// Error performs a no-op.
func (iter *iavlIterator) Error() error {
	return nil
}

//----------------------------------------

func (iter *iavlIterator) setNext(key, value []byte) {
	iter.assertIsValid(false)

	iter.key = key
	iter.value = value
}

func (iter *iavlIterator) setInvalid() {
	iter.assertIsValid(false)

	iter.invalid = true
}

func (iter *iavlIterator) waitInit() {
	<-iter.initCh
}

func (iter *iavlIterator) receiveNext() {
	kvPair, ok := <-iter.iterCh
	if ok {
		iter.setNext(kvPair.Key, kvPair.Value)
	} else {
		iter.setInvalid()
	}
}

// assertIsValid panics if the iterator is invalid. If unlockMutex is true,
// it also unlocks the mutex before panicing, to prevent deadlocks in code that
// recovers from panics
func (iter *iavlIterator) assertIsValid(unlockMutex bool) {
	if iter.invalid {
		if unlockMutex {
			iter.mtx.Unlock()
		}
		panic("invalid iterator")
	}
}

// SetInitialVersion sets the initial version of the IAVL tree. It is used when
// starting a new chain at an arbitrary height.
func (st *Store) SetInitialVersion(version int64) {
	st.tree.SetInitialVersion(uint64(version))
}

// Exports the IAVL store at the given version, returning an iavl.Exporter for the tree.
func (st *Store) Export(version int64) (*iavl.Exporter, error) {
	istore, err := st.GetImmutable(version)
	if err != nil {
		return nil, fmt.Errorf("iavl export failed for version %v: %w", version, err)
	}
	tree, ok := istore.tree.(*immutableTree)
	if !ok || tree == nil {
		return nil, fmt.Errorf("iavl export failed: unable to fetch tree for version %v", version)
	}
	return tree.Export(), nil
}

// Import imports an IAVL tree at the given version, returning an iavl.Importer for importing.
func (st *Store) Import(version int64) (*iavl.Importer, error) {
	tree, ok := st.tree.(*iavl.MutableTree)
	if !ok {
		return nil, errors.New("iavl import failed: unable to find mutable tree")
	}
	return tree.Import(version)
}

func (st *Store) SetUpgradeVersion(version int64) {
	st.upgradeVersion = version
}

func (st *Store) GetUpgradeVersion() int64 {
	return st.upgradeVersion
}
