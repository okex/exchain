package mempool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/tendermint/go-amino"

	"github.com/okex/exchain/libs/system/trace"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/clist"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmmath "github.com/okex/exchain/libs/tendermint/libs/math"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/types"
)

type TxInfoParser interface {
	GetRawTxInfo(tx types.Tx) ExTxInfo
	GetTxHistoryGasUsed(tx types.Tx, gasLimit int64) (int64, bool)
	GetRealTxFromRawTx(rawTx types.Tx) abci.TxEssentials
}

var (
	// GlobalRecommendedGP is initialized to 0.1GWei
	GlobalRecommendedGP = big.NewInt(100000000)
	IsCongested         = false
)

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

	//bcTxsList    *clist.CList   // only for tx sort model
	proxyAppConn proxy.AppConnMempool

	// Track whether we're rechecking txs.
	// These are not protected by a mutex and are expected to be mutated in
	// serial (ie. by abci responses which are called in serial).
	recheckCursor *clist.CElement // next expected response
	recheckEnd    *clist.CElement // re-checking stops here

	// Keep a cache of already-seen txs.
	// This reduces the pressure on the proxyApp.
	// Save wtx as value if occurs or save nil as value
	cache txCache

	eventBus types.TxEventPublisher

	logger    log.Logger
	pguLogger log.Logger

	metrics *Metrics

	pendingPool                *PendingPool
	accountRetriever           AccountRetriever
	pendingPoolNotify          chan map[string]uint64
	consumePendingTxQueue      chan *AddressNonce
	consumePendingTxQueueLimit int

	txInfoparser TxInfoParser

	checkCnt    int64
	checkRPCCnt int64
	checkP2PCnt int64

	checkTotalTime    int64
	checkRpcTotalTime int64
	checkP2PTotalTime int64

	txs ITransactionQueue

	simQueue        chan *mempoolTx
	rmPendingTxChan chan types.EventDataRmPendingTx

	gpo *Oracle

	peersTxCountMtx sync.RWMutex
	peersTxCount    map[string]uint64

	info pguInfo
}

type pguInfo struct {
	txCount int64
	gasUsed int64
}

func (p *pguInfo) reset() {
	p.txCount = 0
	p.gasUsed = 0
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
	var txQueue ITransactionQueue
	if config.SortTxByGp {
		txQueue = NewOptimizedTxQueue(int64(config.TxPriceBump))
	} else {
		txQueue = NewBaseTxQueue()
	}

	gpoConfig := NewGPOConfig(cfg.DynamicConfig.GetDynamicGpWeight(), cfg.DynamicConfig.GetDynamicGpCheckBlocks())
	gpo := NewOracle(gpoConfig)

	mempool := &CListMempool{
		config:        config,
		proxyAppConn:  proxyAppConn,
		height:        height,
		recheckCursor: nil,
		recheckEnd:    nil,
		eventBus:      types.NopEventBus{},
		logger:        log.NewNopLogger(),
		pguLogger:     log.NewNopLogger(),
		metrics:       NopMetrics(),
		txs:           txQueue,
		simQueue:      make(chan *mempoolTx, 200000),
		gpo:           gpo,
		peersTxCount:  make(map[string]uint64, 0),
	}

	if config.PendingRemoveEvent {
		mempool.rmPendingTxChan = make(chan types.EventDataRmPendingTx, 1000)
		go mempool.fireRmPendingTxEvents()
	}

	for i := 0; i < cfg.DynamicConfig.GetPGUConcurrency(); i++ {
		go mempool.simulationRoutine()
	}

	if cfg.DynamicConfig.GetMempoolCacheSize() > 0 {
		mempool.cache = newMapTxCache(cfg.DynamicConfig.GetMempoolCacheSize())
	} else {
		mempool.cache = nopTxCache{}
	}
	proxyAppConn.SetResponseCallback(mempool.globalCb)
	for _, option := range options {
		option(mempool)
	}

	if config.EnablePendingPool {
		mempool.pendingPool = newPendingPool(config.PendingPoolSize, config.PendingPoolPeriod,
			config.PendingPoolReserveBlocks, config.PendingPoolMaxTxPerAddress)
		mempool.pendingPoolNotify = make(chan map[string]uint64, 1)
		go mempool.pendingPoolJob()

		// consumePendingTxQueueLimit use  PendingPoolSize, because consumePendingTx is consume pendingTx.
		mempool.consumePendingTxQueueLimit = mempool.config.PendingPoolSize
		mempool.consumePendingTxQueue = make(chan *AddressNonce, mempool.consumePendingTxQueueLimit)
		go mempool.consumePendingTxQueueJob()
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
	mem.pguLogger = l.With("module", "pgu")
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

// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) Height() int64 {
	return atomic.LoadInt64(&mem.height)
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
	return mem.txs.BroadcastFront()
}

// TxsWaitChan returns a channel to wait on transactions. It will be closed
// once the mempool is not empty (ie. the internal `mem.txs` has at least one
// element)
//
// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) TxsWaitChan() <-chan struct{} {
	return mem.txs.TxsWaitChan()
}

func (mem *CListMempool) validatePeerCount(txInfo TxInfo) error {
	if cfg.DynamicConfig.GetMaxTxLimitPerPeer() == 0 {
		return nil
	}
	mem.peersTxCountMtx.Lock()
	defer mem.peersTxCountMtx.Unlock()
	if len(txInfo.SenderP2PID) != 0 {
		peerTxCount, ok := mem.peersTxCount[string(txInfo.SenderP2PID)]
		if !ok {
			peerTxCount = 0
		}
		if peerTxCount >= cfg.DynamicConfig.GetMaxTxLimitPerPeer() {
			mem.logger.Debug(fmt.Sprintf("%s has been over %d transaction, please wait a few second", txInfo.SenderP2PID, cfg.DynamicConfig.GetMaxTxLimitPerPeer()))
			return fmt.Errorf("%s has been over %d transaction, please wait a few second", txInfo.SenderP2PID, cfg.DynamicConfig.GetMaxTxLimitPerPeer())
		}
		peerTxCount++
		mem.peersTxCount[string(txInfo.SenderP2PID)] = peerTxCount
	}
	return nil
}

func (mem *CListMempool) resetPeerCount() {
	if cfg.DynamicConfig.GetMaxTxLimitPerPeer() == 0 {
		return
	}
	mem.peersTxCountMtx.Lock()
	defer mem.peersTxCountMtx.Unlock()
	for key := range mem.peersTxCount {
		delete(mem.peersTxCount, key)
	}
}

// It blocks if we're waiting on Update() or Reap().
// cb: A callback from the CheckTx command.
//
//	It gets called from another goroutine.
//
// CONTRACT: Either cb will get called, or err returned.
//
// Safe for concurrent use by multiple goroutines.
func (mem *CListMempool) CheckTx(tx types.Tx, cb func(*abci.Response), txInfo TxInfo) error {
	if err := mem.validatePeerCount(txInfo); err != nil {
		return err
	}
	timeStart := int64(0)
	if cfg.DynamicConfig.GetMempoolCheckTxCost() {
		timeStart = time.Now().UnixMicro()
	}

	txSize := len(tx)
	// the old logic for can not allow to delete low gasprice tx,then we must check mempool txs weather is full.
	if !mem.GetEnableDeleteMinGPTx() {
		if err := mem.isFull(txSize); err != nil {
			return err
		}
	}
	// TODO
	// the new logic that even if mempool is full, we check tx gasprice weather > the minimum gas price tx in mempool. If true , we delete it.
	// But For mempool is under the abci, it can not get tx gasprice, so the line we can not precheck gasprice. Maybe we can break abci level for

	// The size of the corresponding amino-encoded TxMessage
	// can't be larger than the maxMsgSize, otherwise we can't
	// relay it to peers.
	if txSize > mem.config.MaxTxBytes {
		return ErrTxTooLarge{mem.config.MaxTxBytes, txSize}
	}

	var nonce uint64
	wCMTx := mem.CheckAndGetWrapCMTx(tx, txInfo)
	if wCMTx != nil {
		txInfo.wrapCMTx = wCMTx
		tx = wCMTx.GetTx()
		nonce = wCMTx.GetNonce()
		mem.logger.Debug("checkTx is wrapCMTx", "nonce", nonce)
	}

	txkey := txKey(tx)

	// CACHE
	if !mem.cache.PushKey(txkey) {
		// Record a new sender for a tx we've already seen.
		// Note it's possible a tx is still in the cache but no longer in the mempool
		// (eg. after committing a block, txs are removed from mempool but not cache),
		// so we only record the sender for txs still in the mempool.
		if ele, ok := mem.txs.Load(txkey); ok {
			memTx := ele.Value.(*mempoolTx)
			memTx.senderMtx.Lock()
			memTx.senders[txInfo.SenderID] = struct{}{}
			memTx.senderMtx.Unlock()
			// TODO: consider punishing peer for dups,
			// its non-trivial since invalid txs can become valid,
			// but they can spam the same tx with little cost to them atm.
		}
		return ErrTxInCache
	}
	// END CACHE

	mem.updateMtx.RLock()
	// use defer to unlock mutex because application (*local client*) might panic
	defer mem.updateMtx.RUnlock()

	var err error

	if mem.preCheck != nil {
		if err = mem.preCheck(tx); err != nil {
			return ErrPreCheck{err}
		}
	}

	// NOTE: proxyAppConn may error if tx buffer is full
	if err = mem.proxyAppConn.Error(); err != nil {
		return err
	}

	if txInfo.from != "" {
		types.SignatureCache().Add(txkey[:], txInfo.from)
	}

	reqRes := mem.proxyAppConn.CheckTxAsync(abci.RequestCheckTx{Tx: tx, Type: txInfo.checkType, From: txInfo.wtx.GetFrom(), Nonce: nonce})
	if r, ok := reqRes.Response.Value.(*abci.Response_CheckTx); ok {
		gasLimit := r.CheckTx.GasWanted
		if cfg.DynamicConfig.GetMaxGasUsedPerBlock() > -1 {
			txHash := tx.Hash(mem.Height())
			txInfo.gasUsed, txInfo.isGasPrecise = mem.txInfoparser.GetTxHistoryGasUsed(tx, gasLimit) // r.CheckTx.GasWanted is gasLimit
			mem.logger.Info(fmt.Sprintf("mempool.SimulateTx: txhash<%s>, gasLimit<%d>, gasUsed<%d>",
				hex.EncodeToString(txHash), r.CheckTx.GasWanted, txInfo.gasUsed))
		}
		if txInfo.gasUsed <= 0 || txInfo.gasUsed > gasLimit {
			txInfo.gasUsed = gasLimit
		}
	}

	reqRes.SetCallback(mem.reqResCb(tx, txInfo, cb))
	atomic.AddInt64(&mem.checkCnt, 1)

	if cfg.DynamicConfig.GetMempoolCheckTxCost() {
		pastTime := time.Now().UnixMicro() - timeStart
		if txInfo.SenderID != 0 {
			atomic.AddInt64(&mem.checkP2PCnt, 1)
			atomic.AddInt64(&mem.checkP2PTotalTime, pastTime)
		} else {
			atomic.AddInt64(&mem.checkRPCCnt, 1)
			atomic.AddInt64(&mem.checkRpcTotalTime, pastTime)
		}
		atomic.AddInt64(&mem.checkTotalTime, pastTime)
	}

	return nil
}

func (mem *CListMempool) CheckAndGetWrapCMTx(tx types.Tx, txInfo TxInfo) *types.WrapCMTx {
	if txInfo.wrapCMTx != nil { // from p2p
		return txInfo.wrapCMTx
	}
	// from rpc should check if the tx is WrapCMTx
	wtx := &types.WrapCMTx{}
	err := cdc.UnmarshalJSON(tx, &wtx)
	if err != nil {
		return nil
	}
	return wtx
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
//   - resCbFirstTime (lock not held) if tx is valid
func (mem *CListMempool) addTx(memTx *mempoolTx) error {
	if err := mem.txs.Insert(memTx); err != nil {
		return err
	}
	if cfg.DynamicConfig.GetMaxGasUsedPerBlock() > -1 && cfg.DynamicConfig.GetEnablePGU() && atomic.LoadUint32(&memTx.isSim) == 0 {
		select {
		case mem.simQueue <- memTx:
		default:
			mem.logger.Error("tx simulation queue is full")
		}
	}

	atomic.AddInt64(&mem.txsBytes, int64(len(memTx.tx)))
	mem.metrics.TxSizeBytes.Observe(float64(len(memTx.tx)))
	mem.eventBus.PublishEventPendingTx(types.EventDataTx{
		TxResult: types.TxResult{
			Height: memTx.height,
			Tx:     memTx.tx,
		},
		Nonce: memTx.senderNonce,
	})

	return nil
}

// Called from:
//   - Update (lock held) if tx was committed
//   - resCbRecheck (lock not held) if tx was invalidated
func (mem *CListMempool) removeTx(elem *clist.CElement) {
	mem.txs.Remove(elem)
	tx := elem.Value.(*mempoolTx).tx
	atomic.AddInt64(&mem.txsBytes, int64(-len(tx)))
}

func (mem *CListMempool) removeTxByKey(key [32]byte) (elem *clist.CElement) {
	elem = mem.txs.RemoveByKey(key)
	if elem != nil {
		tx := elem.Value.(*mempoolTx).tx
		atomic.AddInt64(&mem.txsBytes, int64(-len(tx)))
	}
	return
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
	mem.logger.Debug("mempool", "addPendingTx", hex.EncodeToString(memTx.realTx.TxHash()), "nonce", memTx.realTx.GetNonce(), "gp", memTx.realTx.GetGasPrice(), "pending Nouce", pendingNonce, "excepectNouce", expectedNonce)
	if txNonce == 0 || txNonce < expectedNonce {
		return mem.addTx(memTx)
	}
	// add pending tx
	if txNonce == expectedNonce {
		err := mem.addTx(memTx)
		if err == nil {
			addrNonce := addressNoncePool.Get().(*AddressNonce)
			addrNonce.addr = memTx.from
			addrNonce.nonce = txNonce + 1
			select {
			case mem.consumePendingTxQueue <- addrNonce:
			default:
				//This line maybe be lead to user pendingTx will not be packed into block
				//when extreme condition (mem.consumePendingTxQueue is block which is maintain caused by mempool is full).
				//But we must be do thus,for protect chain's block can be product.
				addressNoncePool.Put(addrNonce)
				mem.logger.Error("mempool", "addPendingTx", "when consumePendingTxQueue and mempool is full, disable consume pending tx")
			}
			//go mem.consumePendingTx(memTx.from, txNonce+1)
		}
		return err
	}

	// add tx to PendingPool
	if err := mem.pendingPool.validate(memTx.from, memTx.tx, memTx.height); err != nil {
		return err
	}
	pendingTx := memTx
	mem.pendingPool.addTx(pendingTx)
	mem.logger.Debug("mempool", "add-pending-Tx", hex.EncodeToString(memTx.realTx.TxHash()), "nonce", memTx.realTx.GetNonce(), "gp", memTx.realTx.GetGasPrice())

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
			minGPTx := mem.txs.Back().Value.(*mempoolTx)
			// If disable deleteMinGPTx, it'old logic, must be remove cache key
			// If enable deleteMinGPTx,it's new logic, check tx.gasprice < minimum tx gas price then remove cache key

			thresholdGasPrice := MultiPriceBump(minGPTx.realTx.GetGasPrice(), int64(mem.config.TxPriceBump))
			if !mem.GetEnableDeleteMinGPTx() || (mem.GetEnableDeleteMinGPTx() && thresholdGasPrice.Cmp(pendingTx.realTx.GetGasPrice()) >= 0) {
				time.Sleep(time.Duration(mem.pendingPool.period) * time.Second)
				continue
			}
		}
		mem.logger.Debug("mempool", "consumePendingTx", hex.EncodeToString(pendingTx.realTx.TxHash()), "nonce", pendingTx.realTx.GetNonce(), "gp", pendingTx.realTx.GetGasPrice())

		mempoolTx := pendingTx
		mempoolTx.height = mem.Height()
		if err := mem.addTx(mempoolTx); err != nil {
			mem.logger.Error(fmt.Sprintf("Pending Pool add tx failed:%s", err.Error()))
			mem.pendingPool.removeTx(address, nonce)
			return
		}

		mem.logger.Info("Added good transaction",
			"tx", txIDStringer{mempoolTx.tx, mempoolTx.height},
			"height", mempoolTx.height,
			"total", mem.Size(),
		)
		mem.notifyTxsAvailable()
		mem.pendingPool.removeTx(address, nonce)
		nonce++
	}
}

type logAddTxData struct {
	Params [8]interface{}
	TxID   txIDStringer
	Height int64
	Total  int
}

var logAddTxDataPool = sync.Pool{
	New: func() interface{} {
		return &logAddTxData{}
	},
}

func (mem *CListMempool) logAddTx(memTx *mempoolTx, r *abci.Response_CheckTx) {
	logAddTxData := logAddTxDataPool.Get().(*logAddTxData)
	logAddTxData.TxID = txIDStringer{memTx.tx, memTx.height}
	logAddTxData.Height = memTx.height
	logAddTxData.Total = mem.Size()

	params := &logAddTxData.Params
	params[0] = "tx"
	params[1] = &logAddTxData.TxID
	params[2] = "res"
	params[3] = r
	params[4] = "height"
	params[5] = &logAddTxData.Height
	params[6] = "total"
	params[7] = &logAddTxData.Total
	mem.logger.Info("Added good transaction", params[:8]...)
	logAddTxDataPool.Put(logAddTxData)
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
		var txHash []byte
		if r.CheckTx != nil && r.CheckTx.Tx != nil {
			txHash = r.CheckTx.Tx.TxHash()
		}
		txkey := txOrTxHashToKey(tx, txHash, mem.height)

		if (r.CheckTx.Code == abci.CodeTypeOK) && postCheckErr == nil {
			// Check mempool isn't full again to reduce the chance of exceeding the
			// limits.
			if err := mem.isFull(len(tx)); err != nil {
				minGPTx := mem.txs.Back().Value.(*mempoolTx)
				// If disable deleteMinGPTx, it'old logic, must be remove cache key
				// If enable deleteMinGPTx,it's new logic, check tx.gasprice < minimum tx gas price then remove cache key
				thresholdGasPrice := MultiPriceBump(minGPTx.realTx.GetGasPrice(), int64(mem.config.TxPriceBump))
				if !mem.GetEnableDeleteMinGPTx() || (mem.GetEnableDeleteMinGPTx() && thresholdGasPrice.Cmp(r.CheckTx.Tx.GetGasPrice()) >= 0) {
					// remove from cache (mempool might have a space later)
					mem.cache.RemoveKey(txkey)
					errStr := err.Error()
					mem.logger.Info(errStr)
					r.CheckTx.Code = 1
					r.CheckTx.Log = errStr
					return
				}
			}

			//var exTxInfo ExTxInfo
			//if err := json.Unmarshal(r.CheckTx.Data, &exTxInfo); err != nil {
			//	mem.cache.Remove(tx)
			//	mem.logger.Error(fmt.Sprintf("Unmarshal ExTxInfo error:%s", err.Error()))
			//	return
			//}
			if r.CheckTx.Tx.GetGasPrice().Sign() <= 0 {
				mem.cache.RemoveKey(txkey)
				errMsg := "Failed to get extra info for this tx!"
				mem.logger.Error(errMsg)
				r.CheckTx.Code = 1
				r.CheckTx.Log = errMsg
				return
			}

			memTx := &mempoolTx{
				height:      mem.Height(),
				gasLimit:    r.CheckTx.GasWanted,
				gasWanted:   txInfo.gasUsed,
				tx:          tx,
				realTx:      r.CheckTx.Tx,
				nodeKey:     txInfo.wtx.GetNodeKey(),
				signature:   txInfo.wtx.GetSignature(),
				from:        r.CheckTx.Tx.GetEthAddr(),
				senderNonce: r.CheckTx.SenderNonce,
			}
			if txInfo.isGasPrecise {
				// gas for hgu is precise, just mark it simulated, so it will not be simulated again
				memTx.isSim = 1
				memTx.hguPrecise = true
			}

			if txInfo.wrapCMTx != nil {
				memTx.isWrapCMTx = true
				memTx.wrapCMNonce = txInfo.wrapCMTx.GetNonce()
			}

			memTx.senders = make(map[uint16]struct{})
			memTx.senders[txInfo.SenderID] = struct{}{}

			var err error
			if mem.pendingPool != nil {
				err = mem.addPendingTx(memTx)
			} else {
				err = mem.addTx(memTx)
			}

			if err == nil {
				mem.logAddTx(memTx, r)
				mem.notifyTxsAvailable()
			} else {
				// ignore bad transaction
				mem.logger.Info("Fail to add transaction into mempool, rejected it",
					"tx", txIDStringer{tx, mem.height}, "peerID", txInfo.SenderP2PID, "res", r, "err", err)
				mem.metrics.FailedTxs.Add(1)
				// remove from cache (it might be good later)
				mem.cache.RemoveKey(txkey)

				r.CheckTx.Code = 1
				r.CheckTx.Log = err.Error()
			}
		} else {
			// ignore bad transaction
			mem.logger.Info("Rejected bad transaction",
				"tx", txIDStringer{tx, mem.height}, "peerID", txInfo.SenderP2PID, "res", r, "err", postCheckErr)
			mem.metrics.FailedTxs.Add(1)
			// remove from cache (it might be good later)
			mem.cache.RemoveKey(txkey)
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
			mem.logger.Info("Tx is no longer valid", "tx", txIDStringer{tx, memTx.height}, "res", r, "err", postCheckErr)
			// NOTE: we remove tx from the cache because it might be good later
			mem.cache.Remove(tx)
			mem.removeTx(mem.recheckCursor)

			if mem.config.PendingRemoveEvent {
				mem.rmPendingTxChan <- types.EventDataRmPendingTx{
					memTx.realTx.TxHash(),
					memTx.realTx.GetEthAddr(),
					memTx.realTx.GetNonce(),
					types.Recheck,
				}
			}
		}
		if mem.recheckCursor == mem.recheckEnd {
			mem.recheckCursor = nil
			mem.recheckEnd = nil
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

func (mem *CListMempool) GetTxSimulateGas(txHash string) int64 {
	return getPGUGas([]byte(txHash))
}

func (mem *CListMempool) ReapEssentialTx(tx types.Tx) abci.TxEssentials {
	if ele, ok := mem.txs.Load(txKey(tx)); ok {
		return ele.Value.(*mempoolTx).realTx
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
	txFilter := make(map[[32]byte]struct{})
	var simCount, simGas int64
	defer func() {
		mem.logger.Info("ReapMaxBytesMaxGas", "ProposingHeight", mem.Height()+1,
			"MempoolTxs", mem.txs.Len(), "ReapTxs", len(txs))
		mem.info.txCount = simCount
		mem.info.gasUsed = simGas
	}()
	for e := mem.txs.Front(); e != nil; e = e.Next() {
		memTx := e.Value.(*mempoolTx)
		key := txOrTxHashToKey(memTx.tx, memTx.realTx.TxHash(), mem.Height())
		if _, ok := txFilter[key]; ok {
			// Just log error and ignore the dup tx. and it will be packed into the next block and deleted from mempool
			mem.logger.Error("found duptx in same block", "tx hash", hex.EncodeToString(key[:]))
			continue
		}
		txFilter[key] = struct{}{}
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
		atomic.AddUint32(&memTx.outdated, 1)
		gasWanted := atomic.LoadInt64(&memTx.gasWanted)
		newTotalGas := totalGas + gasWanted
		if maxGas > -1 && gasWanted >= maxGas {
			mem.logger.Error("tx gas overflow", "txHash", hex.EncodeToString(key[:]), "gasWanted", gasWanted, "isSim", memTx.isSim)
		}
		if maxGas > -1 && newTotalGas > maxGas && len(txs) > 0 {
			return txs
		}
		if totalTxNum >= cfg.DynamicConfig.GetMaxTxNumPerBlock() {
			return txs
		}

		totalTxNum++
		totalGas = newTotalGas
		txs = append(txs, memTx.tx)
		simGas += gasWanted
		if atomic.LoadUint32(&memTx.isSim) > 0 {
			simCount++
		}
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
	if ele, ok := mem.txs.Load(hash); ok {
		return ele.Value.(*mempoolTx).tx, nil
	}
	return nil, ErrNoSuchTx
}

func (mem *CListMempool) ReapUserTxsCnt(address string) int {
	mem.updateMtx.RLock()
	defer mem.updateMtx.RUnlock()

	return mem.GetUserPendingTxsCnt(address)
}

func (mem *CListMempool) ReapUserTxs(address string, max int) types.Txs {
	max = tmmath.MinInt(mem.txs.Len(), max)
	return mem.txs.GetAddressTxs(address, max)
}

func (mem *CListMempool) GetUserPendingTxsCnt(address string) int {
	return mem.txs.GetAddressTxsCnt(address)
}

func (mem *CListMempool) GetAddressList() []string {
	return mem.txs.GetAddressList()
}

func (mem *CListMempool) GetPendingNonce(address string) (uint64, bool) {
	return mem.txs.GetAddressNonce(address)
}

type logData struct {
	Params  [4]interface{}
	Address string
	Nonce   uint64
}

var logDataPool = sync.Pool{
	New: func() interface{} {
		return &logData{}
	},
}

func (mem *CListMempool) logUpdate(address string, nonce uint64) {
	logData := logDataPool.Get().(*logData)
	logData.Address = address
	logData.Nonce = nonce
	params := &logData.Params
	params[0] = "address"
	params[1] = &logData.Address
	params[2] = "nonce"
	params[3] = &logData.Nonce
	mem.logger.Debug("mempool update", params[:4]...)
	logDataPool.Put(logData)
}

// Lock() must be help by the caller during execution.
func (mem *CListMempool) Update(
	height int64,
	txs types.Txs,
	deliverTxResponses []*abci.ResponseDeliverTx,
	preCheck PreCheckFunc,
	postCheck PostCheckFunc,
) error {
	mem.resetPeerCount()
	// no need to update when mempool is unavailable
	if mem.config.Sealed {
		return mem.updateSealed(height, txs, deliverTxResponses)
	}

	// Set height
	atomic.StoreInt64(&mem.height, height)
	mem.notifiedTxsAvailable = false

	if preCheck != nil {
		mem.preCheck = preCheck
	}
	if postCheck != nil {
		mem.postCheck = postCheck
	}

	var gasUsed uint64
	var toCleanAccMap, addressNonce map[string]uint64
	toCleanAccMap = make(map[string]uint64)
	if mem.pendingPool != nil {
		addressNonce = make(map[string]uint64)
	}

	for i, tx := range txs {
		txCode := deliverTxResponses[i].Code
		addr := ""
		nonce := uint64(0)
		txhash := tx.Hash(height)
		gasUsedPerTx := deliverTxResponses[i].GasUsed
		gasPricePerTx := big.NewInt(0)
		if ele := mem.cleanTx(height, tx, txCode); ele != nil {
			atomic.AddUint32(&(ele.Value.(*mempoolTx).outdated), 1)
			addr = ele.Address
			nonce = ele.Nonce
			gasPricePerTx = ele.GasPrice
			mem.logUpdate(ele.Address, ele.Nonce)
		} else {
			if mem.txInfoparser != nil {
				txInfo := mem.txInfoparser.GetRawTxInfo(tx)
				gasPricePerTx = txInfo.GasPrice
				addr = txInfo.Sender
				nonce = txInfo.Nonce
			}

			// remove tx signature cache
			types.SignatureCache().Remove(txhash)
		}

		if txCode == abci.CodeTypeOK || txCode > abci.CodeTypeNonceInc {
			toCleanAccMap[addr] = nonce
			gasUsed += uint64(gasUsedPerTx)
		}

		if cfg.DynamicConfig.GetDynamicGpMode() != types.MinimalGpMode {
			// Collect gas price and gas used of txs in current block for gas price recommendation
			mem.gpo.CurrentBlockGPs.Update(gasPricePerTx, uint64(gasUsedPerTx))
		}

		if mem.pendingPool != nil {
			addressNonce[addr] = nonce
		}

		if mem.pendingPool != nil {
			mem.pendingPool.removeTxByHash(amino.HexEncodeToStringUpper(txhash))
		}
		if mem.config.PendingRemoveEvent {
			mem.rmPendingTxChan <- types.EventDataRmPendingTx{txhash, addr, nonce, types.Confirmed}
		}
	}

	if cfg.DynamicConfig.GetDynamicGpMode() != types.MinimalGpMode {
		currentBlockGPsCopy := mem.gpo.CurrentBlockGPs.Copy()
		_ = mem.gpo.BlockGPQueue.Push(currentBlockGPsCopy)
		GlobalRecommendedGP, IsCongested = mem.gpo.RecommendGP()
		mem.gpo.CurrentBlockGPs.Clear()
	}

	mem.metrics.GasUsed.Set(float64(gasUsed))
	trace.GetElapsedInfo().AddInfo(trace.GasUsed, strconv.FormatUint(gasUsed, 10))

	for accAddr, accMaxNonce := range toCleanAccMap {
		mem.txs.CleanItems(accAddr, accMaxNonce)
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
		select {
		case mem.pendingPoolNotify <- addressNonce:
			mem.metrics.PendingPoolSize.Set(float64(mem.pendingPool.Size()))
		default:
			//This line maybe be lead to user pendingTx will not be packed into block
			//when extreme condition (mem.pendingPoolNotify is block which is maintain caused by mempool is full).
			//But we must be do thus,for protect chain's block can be product.
			mem.logger.Error("mempool", "Update", "when mempool  is  full and consume pendingPool, disable consume pending tx")
		}
	}

	if cfg.DynamicConfig.GetMempoolCheckTxCost() {
		mem.checkTxCost()
	} else {
		trace.GetElapsedInfo().AddInfo(trace.MempoolCheckTxCnt, strconv.FormatInt(atomic.LoadInt64(&mem.checkCnt), 10))
		trace.GetElapsedInfo().AddInfo(trace.MempoolTxsCnt, strconv.Itoa(mem.txs.Len()))
		atomic.StoreInt64(&mem.checkCnt, 0)
	}

	if cfg.DynamicConfig.GetEnableDeleteMinGPTx() {
		mem.deleteMinGPTxOnlyFull()
	}
	// WARNING: The txs inserted between [ReapMaxBytesMaxGas, Update) is insert-sorted in the mempool.txs,
	// but they are not included in the latest block, after remove the latest block txs, these txs may
	// in unsorted state. We need to resort them again for the the purpose of absolute order, or just let it go for they are
	// already sorted int the last round (will only affect the account that send these txs).

	return nil
}

func (mem *CListMempool) fireRmPendingTxEvents() {
	for rmTx := range mem.rmPendingTxChan {
		mem.eventBus.PublishEventRmPendingTx(rmTx)
	}
}

func (mem *CListMempool) checkTxCost() {
	trace.GetElapsedInfo().AddInfo(trace.MempoolCheckTxCnt,
		strconv.FormatInt(atomic.LoadInt64(&mem.checkCnt), 10)+","+
			strconv.FormatInt(atomic.LoadInt64(&mem.checkRPCCnt), 10)+","+
			strconv.FormatInt(atomic.LoadInt64(&mem.checkP2PCnt), 10))
	atomic.StoreInt64(&mem.checkCnt, 0)
	atomic.StoreInt64(&mem.checkRPCCnt, 0)
	atomic.StoreInt64(&mem.checkP2PCnt, 0)

	trace.GetElapsedInfo().AddInfo(trace.MempoolCheckTxTime,
		strconv.FormatInt(atomic.LoadInt64(&mem.checkTotalTime)/1000, 10)+"ms,"+
			strconv.FormatInt(atomic.LoadInt64(&mem.checkRpcTotalTime)/1000, 10)+"ms,"+
			strconv.FormatInt(atomic.LoadInt64(&mem.checkP2PTotalTime)/1000, 10)+"ms")
	atomic.StoreInt64(&mem.checkTotalTime, 0)
	atomic.StoreInt64(&mem.checkRpcTotalTime, 0)
	atomic.StoreInt64(&mem.checkP2PTotalTime, 0)
}

func (mem *CListMempool) cleanTx(height int64, tx types.Tx, txCode uint32) *clist.CElement {
	var txHash []byte
	if mem.txInfoparser != nil {
		if realTx := mem.txInfoparser.GetRealTxFromRawTx(tx); realTx != nil {
			txHash = realTx.TxHash()
		}
	}
	txKey := txOrTxHashToKey(tx, txHash, height)
	// CodeTypeOK means tx was successfully executed.
	// CodeTypeNonceInc means tx fails but the nonce of the account increases,
	// e.g., the transaction gas has been consumed.
	if txCode == abci.CodeTypeOK || txCode > abci.CodeTypeNonceInc {
		// Add valid committed tx to the cache (if missing).
		_ = mem.cache.PushKey(txKey)
	} else {
		// Allow invalid transactions to be resubmitted.
		mem.cache.RemoveKey(txKey)
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
	return mem.removeTxByKey(txKey)
}

func (mem *CListMempool) updateSealed(height int64, txs types.Txs, deliverTxResponses []*abci.ResponseDeliverTx) error {
	// Set height
	atomic.StoreInt64(&mem.height, height)
	mem.notifiedTxsAvailable = false
	// no need to update mempool
	if mem.Size() <= 0 {
		return nil
	}
	toCleanAccMap := make(map[string]uint64)
	// update mempool
	for i, tx := range txs {
		txCode := deliverTxResponses[i].Code
		// remove tx from mempool
		if ele := mem.cleanTx(height, tx, txCode); ele != nil {
			if txCode == abci.CodeTypeOK || txCode > abci.CodeTypeNonceInc {
				toCleanAccMap[ele.Address] = ele.Nonce
			}
			mem.logUpdate(ele.Address, ele.Nonce)
		}
	}
	for accAddr, accMaxNonce := range toCleanAccMap {
		mem.txs.CleanItems(accAddr, accMaxNonce)
	}
	// mempool logs
	trace.GetElapsedInfo().AddInfo(trace.MempoolCheckTxCnt, strconv.FormatInt(atomic.LoadInt64(&mem.checkCnt), 10))
	trace.GetElapsedInfo().AddInfo(trace.MempoolTxsCnt, strconv.Itoa(mem.txs.Len()))
	atomic.StoreInt64(&mem.checkCnt, 0)
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

func (mem *CListMempool) LogPgu(isBlockProducer bool) {
	// no need to print pguInfo
	if mem.config.Sealed {
		return
	}

	if isBlockProducer {
		trace.GetElapsedInfo().AddInfo(trace.SimTx, fmt.Sprintf("%d", mem.info.txCount))
		trace.GetElapsedInfo().AddInfo(trace.SimGasUsed, fmt.Sprintf("%d", mem.info.gasUsed))
	} else {
		trace.GetElapsedInfo().AddInfo(trace.SimTx, fmt.Sprintf("%d", 0))
		trace.GetElapsedInfo().AddInfo(trace.SimGasUsed, fmt.Sprintf("%d", 0))
	}

	mem.info.reset()
}

func MultiPriceBump(rawPrice *big.Int, priceBump int64) *big.Int {
	tmpPrice := new(big.Int).Div(rawPrice, big.NewInt(100))
	inc := new(big.Int).Mul(tmpPrice, big.NewInt(priceBump))

	return new(big.Int).Add(inc, rawPrice)
}

//--------------------------------------------------------------------------------

// mempoolTx is a transaction that successfully ran
type mempoolTx struct {
	height      int64 // height that this tx had been validated in
	gasWanted   int64 // amount of gas this tx states it will require
	gasLimit    int64
	tx          types.Tx //
	realTx      abci.TxEssentials
	nodeKey     []byte
	signature   []byte
	from        string
	senderNonce uint64

	outdated uint32
	isSim    uint32

	// `hguPrecise` is true means hgu for this tx is precise and simulation is not necessary
	hguPrecise bool

	isWrapCMTx  bool
	wrapCMNonce uint64

	// ids of peers who've sent us this tx (as a map for quick lookups).
	// senders: PeerID -> bool
	senders   map[uint16]struct{}
	senderMtx sync.RWMutex
}

// Height returns the height for this transaction
func (memTx *mempoolTx) Height() int64 {
	return atomic.LoadInt64(&memTx.height)
}

func (memTx *mempoolTx) ToWrappedMempoolTx() types.WrappedMempoolTx {
	return types.WrappedMempoolTx{
		Height:      memTx.height,
		GasWanted:   memTx.gasWanted,
		GasLimit:    memTx.gasLimit,
		Tx:          memTx.tx,
		NodeKey:     memTx.nodeKey,
		Signature:   memTx.signature,
		From:        memTx.from,
		SenderNonce: memTx.senderNonce,
		Outdated:    memTx.outdated,
		IsSim:       memTx.isSim,
		IsWrapCMTx:  memTx.isWrapCMTx,
		WrapCMNonce: memTx.wrapCMNonce,
	}
}

//--------------------------------------------------------------------------------

type txCache interface {
	Reset()
	Push(tx types.Tx) bool
	PushKey(key [sha256.Size]byte) bool
	Remove(tx types.Tx)
	RemoveKey(key [sha256.Size]byte)
}

// mapTxCache maintains a LRU cache of transactions. This only stores the hash
// of the tx, due to memory concerns.
type mapTxCache struct {
	mtx      sync.Mutex
	size     int
	cacheMap *fastcache.Cache
}

var _ txCache = (*mapTxCache)(nil)

// newMapTxCache returns a new mapTxCache.
func newMapTxCache(cacheSize int) *mapTxCache {
	return &mapTxCache{
		size:     cacheSize,
		cacheMap: fastcache.New(cacheSize * 32),
	}
}

// Reset resets the cache to an empty state.
func (cache *mapTxCache) Reset() {
	cache.mtx.Lock()
	cache.cacheMap = fastcache.New(cache.size * 32)
	cache.mtx.Unlock()
}

// Push adds the given tx to the cache and returns true. It returns
// false if tx is already in the cache.
func (cache *mapTxCache) Push(tx types.Tx) bool {
	// Use the tx hash in the cache
	txHash := txKey(tx)

	return cache.PushKey(txHash)
}

func (cache *mapTxCache) PushKey(txHash [32]byte) bool {
	cache.mtx.Lock()
	defer cache.mtx.Unlock()

	if exists := cache.cacheMap.Has(txHash[:]); exists {
		return false
	}

	cache.cacheMap.Set(txHash[:], nil)
	return true
}

// Remove removes the given tx from the cache.
func (cache *mapTxCache) Remove(tx types.Tx) {
	txHash := txKey(tx)
	cache.cacheMap.Del(txHash[:])
}

func (cache *mapTxCache) RemoveKey(key [32]byte) {
	cache.cacheMap.Del(key[:])
}

type nopTxCache struct{}

var _ txCache = (*nopTxCache)(nil)

func (nopTxCache) Reset()                    {}
func (nopTxCache) Push(types.Tx) bool        { return true }
func (nopTxCache) PushKey(key [32]byte) bool { return true }
func (nopTxCache) Remove(types.Tx)           {}
func (nopTxCache) RemoveKey(key [32]byte)    {}

// --------------------------------------------------------------------------------
// txKey is the fixed length array sha256 hash used as the key in maps.
func txKey(tx types.Tx) (retHash [sha256.Size]byte) {
	copy(retHash[:], tx.Hash(types.GetVenusHeight())[:sha256.Size])
	return
}

func txOrTxHashToKey(tx types.Tx, txHash []byte, height int64) (retHash [sha256.Size]byte) {
	if len(txHash) == sha256.Size && types.HigherThanVenus(height) {
		copy(retHash[:], txHash)
		return
	} else {
		return txKey(tx)
	}
}

type txIDStringer struct {
	tx     []byte
	height int64
}

func (txs txIDStringer) String() string {
	return amino.HexEncodeToStringUpper(types.Tx(txs.tx).Hash(txs.height))
}

// txID is the hex encoded hash of the bytes as a types.Tx.
func txID(tx []byte, height int64) string {
	return amino.HexEncodeToStringUpper(types.Tx(tx).Hash(height))
}

// --------------------------------------------------------------------------------
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

func (mem *CListMempool) consumePendingTxQueueJob() {
	for addrNonce := range mem.consumePendingTxQueue {
		mem.consumePendingTx(addrNonce.addr, addrNonce.nonce)
		addressNoncePool.Put(addrNonce)
	}
}

func (mem *CListMempool) simulateTx(tx types.Tx, gasLimit int64) (int64, error) {
	var simuRes SimulationResponse
	res, err := mem.proxyAppConn.QuerySync(abci.RequestQuery{
		Path: "app/simulate/mempool",
		Data: tx,
	})
	if err != nil {
		return 0, err
	}
	err = cdc.UnmarshalBinaryBare(res.Value, &simuRes)
	if err != nil {
		return 0, err
	}
	gas := int64(simuRes.GasUsed) * int64(cfg.DynamicConfig.GetPGUAdjustment()*100) / 100
	mem.pguLogger.Info("simulateTx", "txHash", hex.EncodeToString(tx.Hash(mem.Height())), "gas", gas, "gasLimit", gasLimit)
	if gas > gasLimit {
		gas = gasLimit
	}
	txHash := tx.Hash(mem.Height())
	if err = updatePGU(txHash, gas); err != nil {
		mem.logger.Error("updatePGU", "txHash", hex.EncodeToString(tx.Hash(mem.Height())), "simGas", gas, "error", err)
	}
	return gas, err
}

func (mem *CListMempool) simulationRoutine() {
	for memTx := range mem.simQueue {
		mem.simulationJob(memTx)
	}
}

func (mem *CListMempool) simulationJob(memTx *mempoolTx) {
	defer types.SignatureCache().Remove(memTx.realTx.TxHash())
	if atomic.LoadUint32(&memTx.outdated) != 0 {
		// memTx is outdated
		return
	}
	global.WaitCommit()
	gas, err := mem.simulateTx(memTx.tx, memTx.gasLimit)
	if err != nil {
		mem.logger.Error("simulateTx", "error", err, "txHash", memTx.tx.Hash(mem.Height()))
		return
	}
	atomic.StoreInt64(&memTx.gasWanted, gas)
	atomic.AddUint32(&memTx.isSim, 1)
}

// trySimulate4BlockAfterNext will be called during Update()
// assume that next step is to proposal a block of height `n` through ReapMaxBytesMaxGas
// trySimulate4NextBlock will skip those txs which would be packed into that block,
// and simulate txs to be packed into block of height `n+1`
func (mem *CListMempool) trySimulate4NextBlock() {
	maxGu := cfg.DynamicConfig.GetMaxGasUsedPerBlock()
	if maxGu < 0 || !cfg.DynamicConfig.GetEnablePGU() {
		return
	}

	var gu int64
	var ele *clist.CElement
	// skip the txs that will be packed into next block
	for ele = mem.txs.Front(); ele != nil; ele = ele.Next() {
		gu += ele.Value.(*mempoolTx).gasWanted
		if gu > maxGu {
			break
		}
	}

	// reset gu for next cycle
	gu = 0

	for ; ele != nil && gu < maxGu; ele = ele.Next() {
		memTx := ele.Value.(*mempoolTx)
		var gas int64
		var err error
		if !memTx.hguPrecise {
			gas, err = mem.simulateTx(memTx.tx, memTx.gasLimit)
			if err != nil {
				mem.logger.Error("trySimulate4BlockAfterNext", "error", err, "txHash", memTx.tx.Hash(mem.Height()))
				return
			}
			atomic.StoreInt64(&memTx.gasWanted, gas)
			atomic.AddUint32(&memTx.isSim, 1)
		} else {
			gas = memTx.gasWanted
		}

		gu += gas
	}

}

func (mem *CListMempool) deleteMinGPTxOnlyFull() {
	//check weather exceed mempool size,then need to delet the minimum gas price
	for mem.Size() > cfg.DynamicConfig.GetMempoolSize() || mem.TxsBytes() > mem.config.MaxTxsBytes {
		removeTx := mem.txs.Back()
		mem.removeTx(removeTx)

		removeMemTx := removeTx.Value.(*mempoolTx)
		var removeMemTxHash []byte
		if removeMemTx.realTx != nil {
			removeMemTxHash = removeMemTx.realTx.TxHash()
		}
		mem.logger.Debug("mempool", "delete Tx", hex.EncodeToString(removeMemTxHash), "nonce", removeMemTx.realTx.GetNonce(), "gp", removeMemTx.realTx.GetGasPrice())
		mem.cache.RemoveKey(txOrTxHashToKey(removeMemTx.tx, removeMemTxHash, removeMemTx.Height()))

		if mem.config.PendingRemoveEvent {
			mem.rmPendingTxChan <- types.EventDataRmPendingTx{removeMemTxHash, removeMemTx.realTx.GetEthAddr(), removeMemTx.realTx.GetNonce(), types.MinGasPrice}
		}
	}
}

func (mem *CListMempool) GetEnableDeleteMinGPTx() bool {
	return cfg.DynamicConfig.GetEnableDeleteMinGPTx()
}

func (mem *CListMempool) GetPendingPoolTxsBytes() map[string]map[string]types.WrappedMempoolTx {
	return mem.pendingPool.GetWrappedAddressTxsMap()
}
