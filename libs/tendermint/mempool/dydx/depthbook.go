package dydx

import (
	"container/list"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type DepthBook struct {
	buyOrders  *OrderList
	sellOrders *OrderList
}

func NewDepthBook() *DepthBook {
	return &DepthBook{
		buyOrders:  NewOrderList(true),
		sellOrders: NewOrderList(false),
	}
}

func (d *DepthBook) Insert(order *WrapOrder) error {
	if order.isBuy() {
		d.buyOrders.Insert(order)
	} else {
		d.sellOrders.Insert(order)
	}
	return nil
}

func (d *DepthBook) SellFront() *WrapOrder {
	ele := d.sellOrders.Front()
	wodr, ok := ele.Value.(*WrapOrder)
	if !ok {
		//TODO log err
		return nil
	}
	return wodr
}

func (d *DepthBook) Delete(hash common.Hash) *list.Element {
	if ele := d.buyOrders.Get(hash); ele != nil {
		return d.buyOrders.Remove(ele)
	}
	if ele := d.sellOrders.Get(hash); ele != nil {
		return d.sellOrders.Remove(ele)
	}
	return nil
}

func (d *DepthBook) Update(results *MatchResult) {
	if results == nil || len(results.MatchedRecords) == 0 {
		return
	}
	succeed := <-results.OnChain
	if !succeed {
		for _, record := range results.MatchedRecords {
			record.Maker.Unfrozen(record.Fill.Amount)
			record.Taker.Unfrozen(record.Fill.Amount)
		}
	}
}

type OrderList struct {
	sync.RWMutex

	reverse bool
	orders  *list.List
	index   map[common.Hash]*list.Element
}

func NewOrderList(reverse bool) *OrderList {
	ol := &OrderList{
		reverse: reverse,
		orders:  list.New(),
		index:   make(map[common.Hash]*list.Element),
	}
	go ol.prune()
	return ol
}

func (o *OrderList) Front() *list.Element {
	o.RLock()
	defer o.RUnlock()
	return o.orders.Front()
}

func (o *OrderList) Get(hash common.Hash) *list.Element {
	o.RLock()
	defer o.RUnlock()
	return o.index[hash]
}

func (o *OrderList) Insert(order *WrapOrder) *list.Element {
	o.Lock()
	defer o.Unlock()

	ele := o.orders.Front()
	for ele != nil {
		cur := ele.Value.(*WrapOrder)
		if o.less(order, cur) {
			newEle := o.orders.InsertBefore(order, ele)
			o.index[order.Hash()] = newEle
			return newEle
		}
		ele = ele.Next()
	}
	newEle := o.orders.PushBack(order)
	o.index[order.Hash()] = newEle
	return newEle
}

func (o *OrderList) Pop() *list.Element {
	o.Lock()
	defer o.Unlock()

	front := o.orders.Front()
	o.orders.Remove(front)
	delete(o.index, front.Value.(*WrapOrder).Hash())
	return front
}

func (o *OrderList) Remove(ele *list.Element) *list.Element {
	o.Lock()
	defer o.Unlock()
	order, ok := ele.Value.(*WrapOrder)
	if !ok {
		//TODO: log error
		return nil
	}
	if _, ok = o.index[order.Hash()]; ok {
		o.orders.Remove(ele)
		delete(o.index, order.Hash())
		return ele
	}
	return nil
}

func (o *OrderList) List() []*WrapOrder {
	o.Lock()
	defer o.Unlock()
	var orders []*WrapOrder
	for ele := o.orders.Front(); ele != nil; ele = ele.Next() {
		orders = append(orders, ele.Value.(*WrapOrder))
	}
	return orders
}

func (o *OrderList) Len() int {
	o.RLock()
	defer o.RUnlock()
	return len(o.index)
}

//TODO, use block.timestamp?
func (o *OrderList) prune() {
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case <-ticker.C:
			o.Lock()
			for ele := o.orders.Front(); ele != nil; ele = ele.Next() {
				if ele.Value.(*WrapOrder).Expiration.Uint64() < uint64(time.Now().Unix()) {
					o.orders.Remove(ele)
				}
			}
			o.Unlock()
		}
	}
}

func (o *OrderList) less(order1, order2 *WrapOrder) bool {
	if o.reverse {
		return order1.Price().Cmp(order2.Price()) > 0
	}
	return order1.Price().Cmp(order2.Price()) < 0
}
