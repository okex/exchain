package mempool

import (
	"container/heap"
	"errors"
	"fmt"
	"github.com/okex/exchain/libs/tendermint/libs/clist"
	"github.com/okex/exchain/libs/tendermint/types"
	"strings"
	"sync"
	"sync/atomic"
)

type HeapQueue struct {
	txs map[string]*clist.CList // Per account nonce-sorted list of transactions

	txsMap  sync.Map //txKey -> CElement
	mutex   sync.RWMutex
	txCount int32
	waitCh  chan struct{}

	bcTxs       *clist.CList
	bcTxsMap    sync.Map
	txPriceBump int64
}

func (hq *HeapQueue) Len() int {
	return int(atomic.LoadInt32(&hq.txCount))
}

func (hq *HeapQueue) tryInsert(tx *mempoolTx) (err error) {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()

	if atomic.LoadInt32(&hq.txCount) == 0 {
		close(hq.waitCh)
	}
	var gq *clist.CList = nil
	key := txKeyFromMempoolTx(tx)
	gq, ok := hq.txs[tx.from]
	if !ok {
		gq = clist.New()
		hq.txs[tx.from] = gq
	}
	gasPrice := tx.realTx.GetGasPrice()
	nonce := tx.realTx.GetNonce()
	newElement := clist.NewCElement(tx, tx.from, gasPrice, nonce)

	if ele := gq.InsertElement(newElement); ele != nil {
		hq.txsMap.Store(key, ele)
		atomic.AddInt32(&hq.txCount, 1)

		ele2 := hq.bcTxs.PushBack(tx)
		ele2.Address = tx.from
		hq.bcTxsMap.Store(key, ele2)
	}
	return err
}

func (hq *HeapQueue) Insert(tx *mempoolTx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*clist.SameNonceError); ok {
				expectedGasPrice := MultiPriceBump(e.SameNonceElement.GasPrice, 10)
				if e.InsertElement.GasPrice.Cmp(expectedGasPrice) <= 0 {
					err = fmt.Errorf("failed to replace tx for acccount %s with nonce %d, the provided gas price %d is not bigger enough", e.InsertElement.Address, e.InsertElement.Nonce, e.InsertElement.GasPrice)
					return
				}
				hq.Remove(e.SameNonceElement)
				err = hq.tryInsert(tx)
				return
			} else {
				err = fmt.Errorf("got unknown err: %v", r)
			}

		}
	}()
	return hq.tryInsert(tx)
}

func (hq *HeapQueue) Remove(element *clist.CElement) {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()
	if gq, ok := hq.txs[element.Address]; ok {
		gq.Remove(element)
		key := txKeyFromMempoolTx(element.Value.(*mempoolTx))
		if _, ok := hq.txsMap.LoadAndDelete(key); ok {
			atomic.AddInt32(&hq.txCount, -1)
		}
		if atomic.LoadInt32(&hq.txCount) == 0 {
			hq.waitCh = make(chan struct{})
		}
		if gq.Front() == nil {
			delete(hq.txs, element.Address)
		}
		hq.removeBCElement(key)
	}
}

func (hq *HeapQueue) RemoveByKey(key [32]byte) *clist.CElement {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()
	v, ok := hq.txsMap.LoadAndDelete(key)
	if ok {
		ele := v.(*clist.CElement)
		if gq, ok := hq.txs[ele.Address]; ok {
			gq.Remove(ele)
			atomic.AddInt32(&hq.txCount, -1)
			if atomic.LoadInt32(&hq.txCount) == 0 {
				hq.waitCh = make(chan struct{})
			}
			if gq.Front() == nil {
				delete(hq.txs, ele.Address)
			}
			hq.removeBCElement(key)
		}
		return ele
	}
	return nil
}

func (hq *HeapQueue) Front() *clist.CElement {
	panic(errors.New("HeapQueue not support Front()"))
}

func (hq *HeapQueue) Back() *clist.CElement {
	panic(errors.New("HeapQueue not support Back()"))
}

func (hq *HeapQueue) BroadcastFront() *clist.CElement {
	return hq.bcTxs.Front()
}

func (hq *HeapQueue) BroadcastLen() int {
	return hq.bcTxs.Len()
}

func (hq *HeapQueue) Load(hash [32]byte) (*clist.CElement, bool) {
	v, ok := hq.txsMap.Load(hash)
	if ok {
		return v.(*clist.CElement), ok
	}
	return nil, ok

}

func (hq *HeapQueue) TxsWaitChan() <-chan struct{} {
	return hq.waitCh
}

func (hq *HeapQueue) GetAddressList() []string {
	hq.mutex.RLock()
	defer hq.mutex.RUnlock()
	list := make([]string, 0, len(hq.txs))
	for k, _ := range hq.txs {
		list = append(list, k)
	}
	return list
}

func (hq *HeapQueue) GetAddressNonce(address string) (uint64, bool) {
	hq.mutex.RLock()
	defer hq.mutex.RUnlock()
	gq, ok := hq.txs[address]
	if ok {
		if gq.Back() != nil {
			return gq.Back().Nonce, true
		}
	}
	return 0, false
}

func (hq *HeapQueue) GetAddressTxsCnt(address string) int {
	hq.mutex.RLock()
	defer hq.mutex.RUnlock()
	gq, ok := hq.txs[address]
	if ok {
		return gq.Len()
	}
	return 0
}

func (hq *HeapQueue) GetAddressTxs(address string, max int) types.Txs {
	hq.mutex.RLock()
	defer hq.mutex.RUnlock()
	txs := make([]types.Tx, 0)

	gq, ok := hq.txs[address]
	if ok {
		if max <= 0 || max > gq.Len() {
			max = gq.Len()
		}

		i := 0
		for e := gq.Front(); i < max && e != nil; e = e.Next() {
			txs = append(txs, e.Value.(*mempoolTx).tx)
			i++
		}
	}

	return txs

}

func (hq *HeapQueue) CleanItems(address string, nonce uint64) {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()
	gq, ok := hq.txs[address]
	if ok {
		for e := gq.Front(); e != nil; {
			if e.Nonce <= nonce {
				temp := e
				e = e.Next()
				gq.Remove(temp)
				key := txKeyFromMempoolTx(temp.Value.(*mempoolTx))
				_, ok := hq.txsMap.LoadAndDelete(key)
				if ok {
					atomic.AddInt32(&hq.txCount, -1)
				}
				if atomic.LoadInt32(&hq.txCount) == 0 {
					hq.waitCh = make(chan struct{})
				}
				if gq.Front() == nil {
					delete(hq.txs, address)
				}
				hq.removeBCElement(key)
			} else {
				break
			}
		}
	}
}

func (hq *HeapQueue) removeBCElement(key [32]byte) {
	if v, ok := hq.bcTxsMap.LoadAndDelete(key); ok {
		ele := v.(*clist.CElement)
		hq.bcTxs.Remove(ele)
		ele.DetachPrev()
	}
}

func (hq *HeapQueue) Init() mempoolTxsByPrice {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()
	heads := make(mempoolTxsByPrice, 0, len(hq.txs))
	for _, accTxs := range hq.txs {
		e := accTxs.Front()
		if e != nil {
			heads = append(heads, e)
		}
	}
	heap.Init(&heads)
	return heads
}

var headsPool = sync.Pool{
	New: func() interface{} {
		return make(mempoolTxsByPriceReverse, 0)
	}}

func (hq *HeapQueue) InitReverse() mempoolTxsByPriceReverse {
	hq.mutex.Lock()
	defer hq.mutex.Unlock()
	heads := headsPool.Get().(mempoolTxsByPriceReverse)
	for _, accTxs := range hq.txs {
		e := accTxs.Back()
		if e != nil {
			heads = append(heads, e)
		}
	}
	heap.Init(&heads)
	return heads
}

func (hq *HeapQueue) Puts(heads *mempoolTxsByPriceReverse) {
	headsPool.Put(*heads)
}

// Peek returns the next transaction by price.
func (*HeapQueue) Peek(heads mempoolTxsByPrice) *mempoolTx {
	if len(heads) == 0 {
		return nil
	}
	return heads[0].Value.(*mempoolTx)
}

// Peek returns the next transaction by price.
func (*HeapQueue) PeekReverse(heads mempoolTxsByPriceReverse) *clist.CElement {
	if len(heads) == 0 {
		return nil
	}
	return heads[0]
}

// Shift replaces the current best head with the next one from the same account.
func (*HeapQueue) Shift(heads *mempoolTxsByPrice) {
	if e := (*heads)[0].Next(); e != nil {
		(*heads)[0] = e
		heap.Fix(heads, 0)
		return
	}
	heap.Pop(heads)
}

// Shift replaces the current best head with the next one from the same account.
func (*HeapQueue) ShiftReverse(heads *mempoolTxsByPriceReverse) {
	if e := (*heads)[0].Prev(); e != nil {
		(*heads)[0] = e
		heap.Fix(heads, 0)
		return
	}
	heap.Pop(heads)
}

func (q *HeapQueue) Type() int {
	return HeapQueueType
}

func NewHeapQueue(txPriceBump int64) ITransactionQueue {
	return &HeapQueue{txs: make(map[string]*clist.CList), bcTxs: clist.New(), waitCh: make(chan struct{}), txPriceBump: txPriceBump}
}

type mempoolTxsByPrice []*clist.CElement

func (s mempoolTxsByPrice) Len() int { return len(s) }
func (s mempoolTxsByPrice) Less(i, j int) bool {
	// If the prices are equal, use the time the transaction was first seen for
	// deterministic sorting
	cmp := s[i].GasPrice.Cmp(s[j].GasPrice)
	if cmp == 0 {
		return strings.Compare(s[i].Address, s[j].Address) >= 0
	}
	return cmp > 0
}
func (s mempoolTxsByPrice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s *mempoolTxsByPrice) Push(x interface{}) {
	*s = append(*s, x.(*clist.CElement))
}

func (s *mempoolTxsByPrice) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

type mempoolTxsByPriceReverse []*clist.CElement

func (s mempoolTxsByPriceReverse) Len() int { return len(s) }
func (s mempoolTxsByPriceReverse) Less(i, j int) bool {
	// If the prices are equal, use the time the transaction was first seen for
	// deterministic sorting
	cmp := s[i].GasPrice.Cmp(s[j].GasPrice)
	if cmp == 0 {
		return strings.Compare(s[i].Address, s[j].Address) < 0
	}
	return cmp < 0
}
func (s mempoolTxsByPriceReverse) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s *mempoolTxsByPriceReverse) Push(x interface{}) {
	*s = append(*s, x.(*clist.CElement))
}

func (s *mempoolTxsByPriceReverse) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}
