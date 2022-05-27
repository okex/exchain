package mempool

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/tendermint/go-amino"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/okex/exchain/libs/system/trace"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	auto "github.com/okex/exchain/libs/tendermint/libs/autofile"
	"github.com/okex/exchain/libs/tendermint/libs/clist"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmmath "github.com/okex/exchain/libs/tendermint/libs/math"
	tmos "github.com/okex/exchain/libs/tendermint/libs/os"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/types"
	"github.com/pkg/errors"
)

type TxInfoParser interface {
	GetRawTxInfo(tx types.Tx) ExTxInfo
	GetTxHistoryGasUsed(tx types.Tx) int64
}

//--------------------------------------------------------------------------------

// CListMempool is an ordered in-memory pool for transactions before they are
// proposed in a consensus round. Transaction validity is checked using the
// CheckTx abci message before the transaction is added to the pool. The
// mempool uses a concurrent list structure for storing transactions that can
// be efficiently accessed by multiple concurrent readers.
type CListMempool struct {
	// Atomic integers
	height   int64 // the last block Update()'d to
	txsBytes int64 // total size of mempool, in bytes

	// notify listeners (ie. consensus) when txs are available
	notifiedTxsAvailable bool
	txsAvailable         chan struct{} // fires once for each height, when the mempool is not empty

	config *cfg.MempoolConfig

	// Exclusive mutex for Update method to prevent concurrent execution of
	// CheckTx or ReapMaxBytesMaxGas(ReapMaxTxs) methods.
	updateMtx sync.RWMutex
	preCheck  PreCheckFunc
	postCheck PostCheckFunc

	wal          *auto.AutoFile // a log of mempool txs
	txs          *clist.CList   // concurrent linked-list of good txs
	bcTxsList    *clist.CList   // only for tx sort model
	proxyAppConn proxy.AppConnMempool

	// Track whether we're rechecking txs.
	// These are not protected by a mutex and are expected to be mutated in
	// serial (ie. by abci responses which are called in serial).
	recheckCursor *clist.CElement // next expected response
	recheckEnd    *clist.CElement // re-checking stops here

	// Map for quick access to txs to record sender in CheckTx.
	// txsMap: txKey -> CElement
	txsMap   sync.Map
	bcTxsMap sync.Map // only for tx sort model

	// Keep a cache of already-seen txs.
	// This reduces the pressure on the proxyApp.
	// Save wtx as value if occurs or save nil as value
	cache txCache

	eventBus types.TxEventPublisher

	logger log.Logger

	metrics *Metrics

	addressRecord *AddressRecord

	pendingPool       *PendingPool
	accountRetriever  AccountRetriever
	pendingPoolNotify chan map[string]uint64

	txInfoparser TxInfoParser
	checkCnt     int64
}

var _ Mempool = &CListMempool{}

// CListMempoolOption sets an optional parameter on the mempool.
type CListMempoolOption func(*CListMempool)

// NewCListMempool returns a new mempool with the given configuration and connection to an application.
func NewCListMempool(
	config *cfg.MempoolConfig,
	proxyAppConn proxy.AppConnMempool,
	height int64,
	options ...CListMempoolOption,
) *CListMempool {
	mempool := &CListMempool{
		config:        config,
		proxyAppConn:  proxyAppConn,
		txs:           clist.New(),
		bcTxsList:     clist.New(),
		height:        height,
		recheckCursor: nil,
		recheckEnd:    nil,
		eventBus:      types.NopEventBus{},
		logger:        log.NewNopLogger(),
		metrics:       NopMetrics(),
	}
	if config.CacheSize > 0 {
		mempool.cache = newMapTxCache(config.CacheSize)
	} else {
		mempool.cache = nopTxCache{}
	}
	proxyAppConn.SetResponseCallback(mempool.globalCb)
	for _, option := range options {
		option(mempool)
	}
	mempool.addressRecord = newAddressRecord(mempool)

	if config.EnablePendingPool {
		mempool.pendingPool = newPendingPool(config.PendingPoolSize, config.PendingPoolPeriod,
			config.PendingPoolReserveBlocks, config.PendingPoolMaxTxPerAddress)
		mempool.pendingPoolNotify = make(chan map[string]uint64, 1)
		go mempool.pendingPoolJob()
	}

	return mempool
}

// NOTE: not thread safe - should only be called once, on startup
func (mem *CListMempool) EnableTxsAvailable() {
	mem.txsAvailable = make(chan struct{}, 1)
}

// SetLogger sets the Logger.
func (mem *CListMempool) SetEventBus(eventBus types.TxEventPublisher) {
	mem.eventBus = eventBus
}

// SetLogger sets the Logger.
func (mem *CListMempool) SetLogger(l log.Logger) {
	mem.logger = l
}

// WithPreCheck sets a filter for the mempool to reject a tx if f(tx) returns
// false. This is ran before CheckTx.
func WithPreCheck(f PreCheckFunc) CListMempoolOption {
	return func(mem *CListMempool) { mem.preCheck = f }
}

// WithPostCheck sets a filter for the mempool to reject a tx if f(tx) returns
// false. This is ran after CheckTx.
func WithPostCheck(f PostCheckFunc) CListMempoolOption {
	return func(mem *CListMempool) { mem.postCheck = f }
}

// WithMetrics sets the metrics.
func WithMetrics(metrics *Metrics) CListMempoolOption {
	return func(mem *CListMempool) { mem.metrics = metrics }
}

func (mem *CListMempool) InitWAL() error {
	var (
		walDir  = mem.config.WalDir()
		walFile = walDir + "/wal"
	)

	const perm = 0700
	if err := tmos.EnsureDir(walDir, perm); err != nil {
		return err
	}

	af, err := auto.OpenAutoFile(walFile)
	if err != nil {
		return fmt.Errorf("can't open autofile %s: %w", walFile, err)
	}

	mem.wal = af
	return nil
}

func (mem *CListMempool) CloseWAL() {
	if err := mem.wal.Close(); err != nil {
		mem.logger.Error("Error closing WAL", "err", err)
	}
	mem.wal = nil
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) Lock() {
	mem.updateMtx.Lock()
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) Unlock() {
	mem.updateMtx.Unlock()
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) Size() int {
	return mem.txs.Len()
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) TxsBytes() int64 {
	return atomic.LoadInt64(&mem.txsBytes)
}

// Lock() must be help by the caller during execution.
func (mem *CListMempool) FlushAppConn() error {
	return mem.proxyAppConn.FlushSync()
}

// XXX: Unsafe! Calling Flush may leave mempool in inconsistent state.
func (mem *CListMempool) Flush() {
	mem.updateMtx.Lock()
	defer mem.updateMtx.Unlock()

	for e := mem.txs.Front(); e != nil; e = e.Next() {
		mem.removeTx(e)
	}

	_ = atomic.SwapInt64(&mem.txsBytes, 0)
	mem.cache.Reset()
}

// TxsFront returns the first transaction in the ordered list for peer
// goroutines to call .NextWait() on.
// FIXME: leaking implementation details!
//
// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) TxsFront() *clist.CElement {
	return mem.txs.Front()
}

func (mem *CListMempool) BroadcastTxsFront() *clist.CElement {
	if mem.config.SortTxByGp {
		return mem.bcTxsList.Front()
	}
	return mem.txs.Front()
}

// TxsWaitChan returns a channel to wait on transactions. It will be closed
// once the mempool is not empty (ie. the internal `mem.txs` has at least one
// element)
//
// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) TxsWaitChan() <-chan struct{} {
	return mem.txs.WaitChan()
}

// It blocks if we're waiting on Update() or Reap().
// cb: A callback from the CheckTx command.
//     It gets called from another goroutine.
// CONTRACT: Either cb will get called, or err returned.
//
// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) CheckTx(tx types.Tx, cb func(*abci.Response), txInfo TxInfo) error {

	txSize := len(tx)
	if err := mem.isFull(txSize); err != nil {
		return err
	}
	// The size of the corresponding amino-encoded TxMessage
	// can't be larger than the maxMsgSize, otherwise we can't
	// relay it to peers.
	if txSize > mem.config.MaxTxBytes {
		return ErrTxTooLarge{mem.config.MaxTxBytes, txSize}
	}
	// CACHE
	if !mem.cache.Push(tx) {
		return ErrTxInCache
	}

	var err error
	var gasUsed int64
	if cfg.DynamicConfig.GetMaxGasUsedPerBlock() > -1 {
		gasUsed = mem.txInfoparser.GetTxHistoryGasUsed(tx)
		if gasUsed < 0 {
			simuRes, err := mem.simulateTx(tx)
			if err != nil {
				return err
			}
			gasUsed = int64(simuRes.GasUsed)
		}
	}

	mem.updateMtx.RLock()
	// use defer to unlock mutex because application (*local client*) might panic
	defer mem.updateMtx.RUnlock()

	if mem.preCheck != nil {
		if err = mem.preCheck(tx); err != nil {
			return ErrPreCheck{err}
		}
	}

	// CACHE
	// Record a new sender for a tx we've already seen.
	// Note it's possible a tx is still in the cache but no longer in the mempool
	// (eg. after committing a block, txs are removed from mempool but not cache),
	// so we only record the sender for txs still in the mempool.
	if e, ok := mem.txsMap.Load(txKey(tx)); ok {
		memTx := e.(*clist.CElement).Value.(*mempoolTx)
		memTx.senders.LoadOrStore(txInfo.SenderID, true)
		// TODO: consider punishing peer for dups,
		// its non-trivial since invalid txs can become valid,
		// but they can spam the same tx with little cost to them atm.
	}
	// END CACHE

	// WAL
	if mem.wal != nil {
		// TODO: Notify administrators when WAL fails
		_, err = mem.wal.Write([]byte(tx))
		if err != nil {
			mem.logger.Error("Error writing to WAL", "err", err)
		}
		_, err = mem.wal.Write([]byte("\n"))
		if err != nil {
			mem.logger.Error("Error writing to WAL", "err", err)
		}
	}
	// END WAL

	// NOTE: proxyAppConn may error if tx buffer is full
	if err = mem.proxyAppConn.Error(); err != nil {
		return err
	}

	reqRes := mem.proxyAppConn.CheckTxAsync(abci.RequestCheckTx{Tx: tx, Type: txInfo.checkType, From: txInfo.wtx.GetFrom()})
	if cfg.DynamicConfig.GetMaxGasUsedPerBlock() > -1 {
		if r, ok := reqRes.Response.Value.(*abci.Response_CheckTx); ok {
			mem.logger.Info(fmt.Sprintf("mempool.SimulateTx: txhash<%s>, gasLimit<%d>, gasUsed<%d>",
				hex.EncodeToString(tx.Hash(mem.height)), r.CheckTx.GasWanted, gasUsed))
			r.CheckTx.GasWanted = gasUsed
		}
	}
	reqRes.SetCallback(mem.reqResCb(tx, txInfo, cb))
	atomic.AddInt64(&mem.checkCnt, 1)
	return nil
}

// Global callback that will be called after every ABCI response.
// Having a single global callback avoids needing to set a callback for each request.
// However, processing the checkTx response requires the peerID (so we can track which txs we heard from who),
// and peerID is not included in the ABCI request, so we have to set request-specific callbacks that
// include this information. If we're not in the midst of a recheck, this function will just return,
// so the request specific callback can do the work.
//
// When rechecking, we don't need the peerID, so the recheck callback happens
// here.
func (mem *CListMempool) globalCb(req *abci.Request, res *abci.Response) {
	if mem.recheckCursor == nil {
		return
	}

	mem.metrics.RecheckTimes.Add(1)
	mem.resCbRecheck(req, res)

	// update metrics
	mem.metrics.Size.Set(float64(mem.Size()))
}

// Request specific callback that should be set on individual reqRes objects
// to incorporate local information when processing the response.
// This allows us to track the peer that sent us this tx, so we can avoid sending it back to them.
// NOTE: alternatively, we could include this information in the ABCI request itself.
//
// External callers of CheckTx, like the RPC, can also pass an externalCb through here that is called
// when all other response processing is complete.
//
// Used in CheckTx to record PeerID who sent us the tx.
func (mem *CListMempool) reqResCb(
	tx []byte,
	txInfo TxInfo,
	externalCb func(*abci.Response),
) func(res *abci.Response) {
	return func(res *abci.Response) {
		if mem.recheckCursor != nil {
			// this should never happen
			panic("recheck cursor is not nil in reqResCb")
		}

		mem.resCbFirstTime(tx, txInfo, res)

		// update metrics
		mem.metrics.Size.Set(float64(mem.Size()))
		if mem.pendingPool != nil {
			mem.metrics.PendingPoolSize.Set(float64(mem.pendingPool.Size()))
		}

		// passed in by the caller of CheckTx, eg. the RPC
		if externalCb != nil {
			externalCb(res)
		}
	}
}

// Called from:
//  - resCbFirstTime (lock not held) if tx is valid
func (mem *CListMempool) addAndSortTx(memTx *mempoolTx) error {

	// Replace the same Nonce transaction from the same account
	elem := mem.addressRecord.checkRepeatedAndAddItem(memTx, int64(mem.config.TxPriceBump), mem.txs.InsertElement)
	if elem == nil {
		return errors.New(fmt.Sprintf("Failed to replace tx for acccount %s with nonce %d, "+
			"the provided gas price %d is not bigger enough", memTx.from, memTx.realTx.GetNonce(), memTx.realTx.GetGasPrice()))
	}

	txHash := txKey(memTx.tx)
	mem.txsMap.Store(txHash, elem)
	atomic.AddInt64(&mem.txsBytes, int64(len(memTx.tx)))

	ele := mem.bcTxsList.PushBack(memTx)
	mem.bcTxsMap.Store(txHash, ele)

	mem.metrics.TxSizeBytes.Observe(float64(len(memTx.tx)))
	mem.eventBus.PublishEventPendingTx(types.EventDataTx{TxResult: types.TxResult{
		Height: memTx.height,
		Tx:     memTx.tx,
	}})

	types.SignatureCache().Remove(memTx.realTx.TxHash())

	return nil
}

// only used in AddressRecord
func (mem *CListMempool) removeElement(elem *clist.CElement) {
	mem.removeTx(elem, true)
}

// only used in AddressRecord
func (mem *CListMempool) reorganizeElements(items []*clist.CElement) {
	if len(items) == 0 {
		return
	}
	// When inserting, strictly order by nonce, otherwise tx will not appear according to nonce,
	// resulting in execution failure
	sort.Slice(items, func(i, j int) bool { return items[i].Nonce < items[j].Nonce })

	for _, item := range items[1:] {
		mem.txs.DetachElement(item)
		item.NewDetachPrev()
		item.NewDetachNext()
	}

	for _, item := range items {
		mem.txs.InsertElement(item)
	}
}

// Called from:
//  - resCbFirstTime (lock not held) if tx is valid
func (mem *CListMempool) addTx(memTx *mempoolTx) error {
	if mem.config.SortTxByGp {
		return mem.addAndSortTx(memTx)
	}
	e := mem.txs.PushBack(memTx)
	e.Address = memTx.from

	mem.addressRecord.AddItem(e.Address, e)

	mem.txsMap.Store(txKey(memTx.tx), e)
	atomic.AddInt64(&mem.txsBytes, int64(len(memTx.tx)))
	mem.metrics.TxSizeBytes.Observe(float64(len(memTx.tx)))
	mem.eventBus.PublishEventPendingTx(types.EventDataTx{TxResult: types.TxResult{
		Height: memTx.height,
		Tx:     memTx.tx,
	}})

	types.SignatureCache().Remove(memTx.realTx.TxHash())

	return nil
}

// Called from:
//  - Update (lock held) if tx was committed
// 	- resCbRecheck (lock not held) if tx was invalidated
func (mem *CListMempool) removeTx(elem *clist.CElement, ignoreAddressRecord ...bool) {
	tx := elem.Value.(*mempoolTx).tx
	txHash := txKey(tx)
	if mem.config.SortTxByGp {
		if e, ok := mem.bcTxsMap.LoadAndDelete(txHash); ok {
			tmpEle := e.(*clist.CElement)
			mem.bcTxsList.Remove(tmpEle)
			tmpEle.DetachPrev()
		}
	}

	mem.txs.Remove(elem)
	elem.DetachPrev()

	if len(ignoreAddressRecord) == 0 {
		mem.addressRecord.DeleteItem(elem)
	}

	mem.txsMap.Delete(txHash)
	atomic.AddInt64(&mem.txsBytes, int64(-len(tx)))
}

func (mem *CListMempool) isFull(txSize int) error {
	var (
		memSize  = mem.Size()
		txsBytes = mem.TxsBytes()
	)
	if memSize >= cfg.DynamicConfig.GetMempoolSize() || int64(txSize)+txsBytes > mem.config.MaxTxsBytes {
		return ErrMempoolIsFull{
			memSize, cfg.DynamicConfig.GetMempoolSize(),
			txsBytes, mem.config.MaxTxsBytes,
		}
	}

	return nil
}

func (mem *CListMempool) addPendingTx(memTx *mempoolTx) error {
	// nonce is continuous
	expectedNonce := memTx.senderNonce
	pendingNonce, ok := mem.GetPendingNonce(memTx.from)
	if ok {
		expectedNonce = pendingNonce + 1
	}
	txNonce := memTx.realTx.GetNonce()
	// cosmos tx does not support pending pool, so here must check whether txNonce is 0
	if txNonce == 0 || txNonce == expectedNonce {
		err := mem.addTx(memTx)
		if err == nil {
			go mem.consumePendingTx(memTx.from, memTx.realTx.GetNonce()+1)
		}
		return err
	}

	// add tx to PendingPool
	if err := mem.pendingPool.validate(memTx.from, memTx.tx, memTx.height); err != nil {
		return err
	}
	pendingTx := memTx
	mem.pendingPool.addTx(pendingTx)
	mem.logger.Debug("pending pool addTx", "tx", pendingTx)

	return nil
}

func (mem *CListMempool) consumePendingTx(address string, nonce uint64) {
	for {
		pendingTx := mem.pendingPool.getTx(address, nonce)
		if pendingTx == nil {
			return
		}
		if err := mem.isFull(len(pendingTx.tx)); err != nil {
			time.Sleep(time.Duration(mem.pendingPool.period) * time.Second)
			continue
		}

		mempoolTx := pendingTx
		mempoolTx.height = mem.height
		if err := mem.addTx(mempoolTx); err != nil {
			mem.logger.Error(fmt.Sprintf("Pending Pool add tx failed:%s", err.Error()))
			mem.pendingPool.removeTx(address, nonce)
			return
		}

		mem.logger.Info("Added good transaction",
			"tx", txID(mempoolTx.tx, mempoolTx.height),
			"height", mempoolTx.height,
			"total", mem.Size(),
		)
		mem.notifyTxsAvailable()
		mem.pendingPool.removeTx(address, nonce)
		nonce++
	}
}

// callback, which is called after the app checked the tx for the first time.
//
// The case where the app checks the tx for the second and subsequent times is
// handled by the resCbRecheck callback.
func (mem *CListMempool) resCbFirstTime(
	tx []byte,
	txInfo TxInfo,
	res *abci.Response,
) {
	switch r := res.Value.(type) {
	case *abci.Response_CheckTx:
		var postCheckErr error
		if mem.postCheck != nil {
			postCheckErr = mem.postCheck(tx, r.CheckTx)
		}
		if (r.CheckTx.Code == abci.CodeTypeOK) && postCheckErr == nil {
			// Check mempool isn't full again to reduce the chance of exceeding the
			// limits.
			if err := mem.isFull(len(tx)); err != nil {
				// remove from cache (mempool might have a space later)
				mem.cache.Remove(tx)
				mem.logger.Error(err.Error())
				r.CheckTx.Code = 1
				r.CheckTx.Log = err.Error()
				return
			}

			//var exTxInfo ExTxInfo
			//if err := json.Unmarshal(r.CheckTx.Data, &exTxInfo); err != nil {
			//	mem.cache.Remove(tx)
			//	mem.logger.Error(fmt.Sprintf("Unmarshal ExTxInfo error:%s", err.Error()))
			//	return
			//}
			if r.CheckTx.Tx.GetGasPrice().Sign() <= 0 {
				mem.cache.Remove(tx)
				errMsg := "Failed to get extra info for this tx!"
				mem.logger.Error(errMsg)
				r.CheckTx.Code = 1
				r.CheckTx.Log = errMsg
				return
			}

			memTx := &mempoolTx{
				height:      mem.height,
				gasWanted:   r.CheckTx.GasWanted,
				tx:          tx,
				realTx:      r.CheckTx.Tx,
				nodeKey:     txInfo.wtx.GetNodeKey(),
				signature:   txInfo.wtx.GetSignature(),
				from:        r.CheckTx.Tx.GetFrom(),
				senderNonce: r.CheckTx.SenderNonce,
			}

			memTx.senders.Store(txInfo.SenderID, true)

			var err error
			if mem.pendingPool != nil {
				err = mem.addPendingTx(memTx)
			} else {
				err = mem.addTx(memTx)
			}

			if err == nil {
				mem.logger.Info("Added good transaction",
					"tx", txID(tx, mem.height),
					"res", r,
					"height", memTx.height,
					"total", mem.Size(),
				)
				mem.notifyTxsAvailable()
			} else {
				// ignore bad transaction
				mem.logger.Info("Fail to add transaction into mempool, rejected it",
					"tx", txID(tx, mem.height), "peerID", txInfo.SenderP2PID, "res", r, "err", postCheckErr)
				mem.metrics.FailedTxs.Add(1)
				// remove from cache (it might be good later)
				mem.cache.Remove(tx)

				r.CheckTx.Code = 1
				r.CheckTx.Log = err.Error()
			}
		} else {
			// ignore bad transaction
			mem.logger.Info("Rejected bad transaction",
				"tx", txID(tx, mem.height), "peerID", txInfo.SenderP2PID, "res", r, "err", postCheckErr)
			mem.metrics.FailedTxs.Add(1)
			// remove from cache (it might be good later)
			mem.cache.Remove(tx)
		}
	default:
		// ignore other messages
	}
}

// callback, which is called after the app rechecked the tx.
//
// The case where the app checks the tx for the first time is handled by the
// resCbFirstTime callback.
func (mem *CListMempool) resCbRecheck(req *abci.Request, res *abci.Response) {
	switch r := res.Value.(type) {
	case *abci.Response_CheckTx:
		tx := req.GetCheckTx().Tx
		memTx := mem.recheckCursor.Value.(*mempoolTx)
		if !bytes.Equal(tx, memTx.tx) {
			panic(fmt.Sprintf(
				"Unexpected tx response from proxy during recheck\nExpected %X, got %X",
				memTx.tx,
				tx))
		}
		var postCheckErr error
		if mem.postCheck != nil {
			postCheckErr = mem.postCheck(tx, r.CheckTx)
		}
		if (r.CheckTx.Code == abci.CodeTypeOK) && postCheckErr == nil {
			// Good, nothing to do.
		} else {
			// Tx became invalidated due to newly committed block.
			mem.logger.Info("Tx is no longer valid", "tx", txID(tx, memTx.height), "res", r, "err", postCheckErr)
			// NOTE: we remove tx from the cache because it might be good later
			mem.cache.Remove(tx)
			mem.removeTx(mem.recheckCursor)
		}
		if mem.recheckCursor == mem.recheckEnd {
			mem.recheckCursor = nil
		} else {
			mem.recheckCursor = mem.recheckCursor.Next()
		}
		if mem.recheckCursor == nil {
			// Done!
			mem.logger.Info("Done rechecking txs")

			// incase the recheck removed all txs
			mem.notifyTxsAvailable()
		}
	default:
		// ignore other messages
	}
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) TxsAvailable() <-chan struct{} {
	return mem.txsAvailable
}

func (mem *CListMempool) notifyTxsAvailable() {
	if mem.Size() == 0 {
		return
	}
	if mem.txsAvailable != nil && !mem.notifiedTxsAvailable {
		// channel cap is 1, so this will send once
		mem.notifiedTxsAvailable = true
		select {
		case mem.txsAvailable <- struct{}{}:
		default:
		}
	}
}

func (mem *CListMempool) ReapEssentialTx(tx types.Tx) abci.TxEssentials {
	if ele, ok := mem.txsMap.Load(txKey(tx)); ok {
		return ele.(*clist.CElement).Value.(*mempoolTx).realTx
	}
	return nil
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) ReapMaxBytesMaxGas(maxBytes, maxGas int64) []types.Tx {
	mem.updateMtx.RLock()
	defer mem.updateMtx.RUnlock()

	var (
		totalBytes int64
		totalGas   int64
		totalTxNum int64
	)
	// TODO: we will get a performance boost if we have a good estimate of avg
	// size per tx, and set the initial capacity based off of that.
	// txs := make([]types.Tx, 0, tmmath.MinInt(mem.txs.Len(), max/mem.avgTxSize))
	txs := make([]types.Tx, 0, tmmath.MinInt(mem.txs.Len(), int(cfg.DynamicConfig.GetMaxTxNumPerBlock())))
	defer func() {
		mem.logger.Info("ReapMaxBytesMaxGas", "ProposingHeight", mem.height+1,
			"MempoolTxs", mem.txs.Len(), "ReapTxs", len(txs))
	}()
	for e := mem.txs.Front(); e != nil; e = e.Next() {
		memTx := e.Value.(*mempoolTx)
		// Check total size requirement
		aminoOverhead := types.ComputeAminoOverhead(memTx.tx, 1)
		if maxBytes > -1 && totalBytes+int64(len(memTx.tx))+aminoOverhead > maxBytes {
			return txs
		}
		totalBytes += int64(len(memTx.tx)) + aminoOverhead
		// Check total gas requirement.
		// If maxGas is negative, skip this check.
		// Since newTotalGas < masGas, which
		// must be non-negative, it follows that this won't overflow.
		newTotalGas := totalGas + memTx.gasWanted
		if maxGas > -1 && newTotalGas > maxGas {
			return txs
		}
		if totalTxNum >= cfg.DynamicConfig.GetMaxTxNumPerBlock() {
			return txs
		}

		totalTxNum++
		totalGas = newTotalGas
		txs = append(txs, memTx.tx)
	}

	return txs
}

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) ReapMaxTxs(max int) types.Txs {
	mem.updateMtx.RLock()
	defer mem.updateMtx.RUnlock()

	if max < 0 {
		max = mem.txs.Len()
	}

	txs := make([]types.Tx, 0, tmmath.MinInt(mem.txs.Len(), max))
	for e := mem.txs.Front(); e != nil && len(txs) <= max; e = e.Next() {
		memTx := e.Value.(*mempoolTx)
		txs = append(txs, memTx.tx)
	}
	return txs
}

func (mem *CListMempool) GetTxByHash(hash [sha256.Size]byte) (types.Tx, error) {
	if e, ok := mem.txsMap.Load(hash); ok {
		memTx := e.(*clist.CElement).Value.(*mempoolTx)
		return memTx.tx, nil
	}
	return nil, ErrNoSuchTx
}

func (mem *CListMempool) ReapUserTxsCnt(address string) int {
	mem.updateMtx.RLock()
	defer mem.updateMtx.RUnlock()

	return mem.GetUserPendingTxsCnt(address)
}

func (mem *CListMempool) ReapUserTxs(address string, max int) types.Txs {
	return mem.addressRecord.GetAddressTxs(address, mem.txs.Len(), max)
}

func (mem *CListMempool) GetUserPendingTxsCnt(address string) int {
	return mem.addressRecord.GetAddressTxsCnt(address)
}

func (mem *CListMempool) GetAddressList() []string {
	return mem.addressRecord.GetAddressList()
}

func (mem *CListMempool) GetPendingNonce(address string) (uint64, bool) {
	return mem.addressRecord.GetAddressNonce(address)
}

// Lock() must be help by the caller during execution.
func (mem *CListMempool) Update(
	height int64,
	txs types.Txs,
	deliverTxResponses []*abci.ResponseDeliverTx,
	preCheck PreCheckFunc,
	postCheck PostCheckFunc,
) error {
	// Set height
	mem.height = height
	mem.notifiedTxsAvailable = false

	if preCheck != nil {
		mem.preCheck = preCheck
	}
	if postCheck != nil {
		mem.postCheck = postCheck
	}

	var gasUsed uint64
	toCleanAccMap := make(map[string]uint64)
	addressNonce := make(map[string]uint64)
	for i, tx := range txs {
		txCode := deliverTxResponses[i].Code
		// CodeTypeOK means tx was successfully executed.
		// CodeTypeNonceInc means tx fails but the nonce of the account increases,
		// e.g., the transaction gas has been consumed.
		if txCode == abci.CodeTypeOK || txCode > abci.CodeTypeNonceInc {
			// add gas used with valid committed tx
			gasUsed += uint64(deliverTxResponses[i].GasUsed)
			// Add valid committed tx to the cache (if missing).
			_ = mem.cache.Push(tx)
		} else {
			// Allow invalid transactions to be resubmitted.
			mem.cache.Remove(tx)
		}

		// Remove committed tx from the mempool.
		//
		// Note an evil proposer can drop valid txs!
		// Mempool before:
		//   100 -> 101 -> 102
		// Block, proposed by an evil proposer:
		//   101 -> 102
		// Mempool after:
		//   100
		// https://github.com/tendermint/tendermint/issues/3322.
		addr := ""
		nonce := uint64(0)
		if e, ok := mem.txsMap.Load(txKey(tx)); ok {
			ele := e.(*clist.CElement)
			addr = ele.Address
			nonce = ele.Nonce
			mem.removeTx(ele)
			mem.logger.Debug("Mempool update", "address", ele.Address, "nonce", ele.Nonce)
		} else {
			if mem.txInfoparser != nil {
				txInfo := mem.txInfoparser.GetRawTxInfo(tx)
				addr = txInfo.Sender
				nonce = txInfo.Nonce
			}

			// remove tx signature cache
			types.SignatureCache().Remove(tx.Hash(height))
		}

		if txCode == abci.CodeTypeOK || txCode > abci.CodeTypeNonceInc {
			toCleanAccMap[addr] = nonce
		}
		addressNonce[addr] = nonce

		if mem.pendingPool != nil {
			mem.pendingPool.removeTxByHash(txID(tx, height))
		}
	}
	mem.metrics.GasUsed.Set(float64(gasUsed))
	trace.GetElapsedInfo().AddInfo(trace.GasUsed, fmt.Sprintf("%d", gasUsed))

	for accAddr, accMaxNonce := range toCleanAccMap {
		items := mem.addressRecord.CleanItems(accAddr, accMaxNonce)
		for _, ele := range items {
			mem.removeTx(ele, true)
		}
	}

	// Either recheck non-committed txs to see if they became invalid
	// or just notify there're some txs left.
	if mem.Size() > 0 {
		if cfg.DynamicConfig.GetMempoolRecheck() || height%cfg.DynamicConfig.GetMempoolForceRecheckGap() == 0 {
			mem.logger.Info("Recheck txs", "numtxs", mem.Size(), "height", height)
			mem.recheckTxs()
			mem.logger.Info("After Recheck txs", "numtxs", mem.Size(), "height", height)
			// At this point, mem.txs are being rechecked.
			// mem.recheckCursor re-scans mem.txs and possibly removes some txs.
			// Before mem.Reap(), we should wait for mem.recheckCursor to be nil.
		} else {
			mem.notifyTxsAvailable()
		}
	} else if height%cfg.DynamicConfig.GetMempoolForceRecheckGap() == 0 {
		// saftly clean dirty data that stucks in the cache
		mem.cache.Reset()
	}

	// Update metrics
	mem.metrics.Size.Set(float64(mem.Size()))
	if mem.pendingPool != nil {
		mem.pendingPoolNotify <- addressNonce
		mem.metrics.PendingPoolSize.Set(float64(mem.pendingPool.Size()))
	}

	trace.GetElapsedInfo().AddInfo(trace.MempoolCheckTxCnt, fmt.Sprintf("%d", atomic.LoadInt64(&mem.checkCnt)))
	trace.GetElapsedInfo().AddInfo(trace.MempoolTxsCnt, fmt.Sprintf("%d", mem.txs.Len()))
	atomic.StoreInt64(&mem.checkCnt, 0)

	// WARNING: The txs inserted between [ReapMaxBytesMaxGas, Update) is insert-sorted in the mempool.txs,
	// but they are not included in the latest block, after remove the latest block txs, these txs may
	// in unsorted state. We need to resort them again for the the purpose of absolute order, or just let it go for they are
	// already sorted int the last round (will only affect the account that send these txs).

	return nil
}

func (mem *CListMempool) recheckTxs() {
	if mem.Size() == 0 {
		panic("recheckTxs is called, but the mempool is empty")
	}

	mem.recheckCursor = mem.txs.Front()
	mem.recheckEnd = mem.txs.Back()

	// Push txs to proxyAppConn
	// NOTE: globalCb may be called concurrently.
	for e := mem.txs.Front(); e != nil; e = e.Next() {
		memTx := e.Value.(*mempoolTx)
		mem.proxyAppConn.CheckTxAsync(abci.RequestCheckTx{
			Tx:   memTx.tx,
			Type: abci.CheckTxType_Recheck,
		})
	}

	mem.proxyAppConn.FlushAsync()
}

func (mem *CListMempool) GetConfig() *cfg.MempoolConfig {
	return mem.config
}

func MultiPriceBump(rawPrice *big.Int, priceBump int64) *big.Int {
	tmpPrice := new(big.Int).Div(rawPrice, big.NewInt(100))
	inc := new(big.Int).Mul(tmpPrice, big.NewInt(priceBump))

	return new(big.Int).Add(inc, rawPrice)
}

//--------------------------------------------------------------------------------

// mempoolTx is a transaction that successfully ran
type mempoolTx struct {
	height      int64    // height that this tx had been validated in
	gasWanted   int64    // amount of gas this tx states it will require
	tx          types.Tx //
	realTx      abci.TxEssentials
	nodeKey     []byte
	signature   []byte
	from        string
	senderNonce uint64

	// ids of peers who've sent us this tx (as a map for quick lookups).
	// senders: PeerID -> bool
	senders sync.Map
}

// Height returns the height for this transaction
func (memTx *mempoolTx) Height() int64 {
	return atomic.LoadInt64(&memTx.height)
}

//--------------------------------------------------------------------------------

type txCache interface {
	Reset()
	Push(tx types.Tx) bool
	Remove(tx types.Tx)
}

// mapTxCache maintains a LRU cache of transactions. This only stores the hash
// of the tx, due to memory concerns.
type mapTxCache struct {
	mtx      sync.Mutex
	size     int
	cacheMap map[[sha256.Size]byte]*list.Element
	list     *list.List
}

var _ txCache = (*mapTxCache)(nil)

// newMapTxCache returns a new mapTxCache.
func newMapTxCache(cacheSize int) *mapTxCache {
	return &mapTxCache{
		size:     cacheSize,
		cacheMap: make(map[[sha256.Size]byte]*list.Element, cacheSize),
		list:     list.New(),
	}
}

// Reset resets the cache to an empty state.
func (cache *mapTxCache) Reset() {
	cache.mtx.Lock()
	cache.cacheMap = make(map[[sha256.Size]byte]*list.Element, cache.size)
	cache.list.Init()
	cache.mtx.Unlock()
}

// Push adds the given tx to the cache and returns true. It returns
// false if tx is already in the cache.
func (cache *mapTxCache) Push(tx types.Tx) bool {
	// Use the tx hash in the cache
	txHash := txKey(tx)

	cache.mtx.Lock()
	defer cache.mtx.Unlock()

	if moved, exists := cache.cacheMap[txHash]; exists {
		cache.list.MoveToBack(moved)
		return false
	}

	if cache.list.Len() >= cache.size {
		popped := cache.list.Front()
		poppedTxHash := popped.Value.([sha256.Size]byte)
		delete(cache.cacheMap, poppedTxHash)
		if popped != nil {
			cache.list.Remove(popped)
		}
	}
	e := cache.list.PushBack(txHash)
	cache.cacheMap[txHash] = e
	return true
}

// Remove removes the given tx from the cache.
func (cache *mapTxCache) Remove(tx types.Tx) {
	txHash := txKey(tx)

	cache.mtx.Lock()
	popped := cache.cacheMap[txHash]
	delete(cache.cacheMap, txHash)
	if popped != nil {
		cache.list.Remove(popped)
	}

	cache.mtx.Unlock()
}

type nopTxCache struct{}

var _ txCache = (*nopTxCache)(nil)

func (nopTxCache) Reset()             {}
func (nopTxCache) Push(types.Tx) bool { return true }
func (nopTxCache) Remove(types.Tx)    {}

//--------------------------------------------------------------------------------
// txKey is the fixed length array sha256 hash used as the key in maps.
func txKey(tx types.Tx) (retHash [sha256.Size]byte) {
	copy(retHash[:], tx.Hash(types.GetVenusHeight())[:sha256.Size])
	return
}

// txID is the hex encoded hash of the bytes as a types.Tx.
func txID(tx []byte, height int64) string {
	return amino.HexEncodeToStringUpper(types.Tx(tx).Hash(height))
}

//--------------------------------------------------------------------------------
type ExTxInfo struct {
	Sender      string   `json:"sender"`
	SenderNonce uint64   `json:"sender_nonce"`
	GasPrice    *big.Int `json:"gas_price"`
	Nonce       uint64   `json:"nonce"`
}

func (mem *CListMempool) SetAccountRetriever(retriever AccountRetriever) {
	mem.accountRetriever = retriever
}

func (mem *CListMempool) SetTxInfoParser(parser TxInfoParser) {
	mem.txInfoparser = parser
}

func (mem *CListMempool) pendingPoolJob() {
	for addressNonce := range mem.pendingPoolNotify {
		timeStart := time.Now()
		mem.logger.Debug("pending pool job begin", "poolSize", mem.pendingPool.Size())
		addrNonceMap := mem.pendingPool.handlePendingTx(addressNonce)
		for addr, nonce := range addrNonceMap {
			mem.consumePendingTx(addr, nonce)
		}
		mem.pendingPool.handlePeriodCounter()
		timeElapse := time.Since(timeStart).Microseconds()
		mem.logger.Debug("pending pool job end", "interval(ms)", timeElapse,
			"poolSize", mem.pendingPool.Size(),
			"addressNonceMap", addrNonceMap)
	}
}

func (mem *CListMempool) simulateTx(tx types.Tx) (*SimulationResponse, error) {
	var simuRes SimulationResponse
	res, err := mem.proxyAppConn.QuerySync(abci.RequestQuery{
		Path: "app/simulate/mempool",
		Data: tx,
	})
	if err != nil {
		return nil, err
	}
	err = cdc.UnmarshalBinaryBare(res.Value, &simuRes)
	return &simuRes, err
}
