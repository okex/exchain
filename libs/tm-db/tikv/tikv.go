package tikv

import (
	"context"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/okex/exchain/libs/tm-db/common"
	"github.com/tikv/client-go/v2/rawkv"
)

func init() {
	dbCreator := func(name string, addr string) (dbm.DB, error) {
		return NewTiKV(name, addr)
	}
	dbm.RegisterDBCreator(dbm.TiKVBackend, dbCreator, false)
}

func NewTiKV(name, addr string) (dbm.DB, error) {
	ret := &TiKV{}
	var err error
	ret.client, err = rawkv.NewClientWithOpts(context.TODO(), []string{addr})
	if err != nil {
		return nil, err
	}

	return ret, nil
}

type TiKV struct {
	common.PlaceHolder
	client *rawkv.Client
}

var _ dbm.DB = (*TiKV)(nil)

func (t *TiKV) Get(key []byte) ([]byte, error) {
	key = dbm.NonNilBytes(key)

	return t.client.Get(context.TODO(), key)
}

func (t *TiKV) GetUnsafeValue(key []byte, processor dbm.UnsafeValueProcessor) (interface{}, error) {
	v, err := t.Get(key)
	if err != nil {
		return nil, err
	}
	return processor(v)
}

func (t *TiKV) Has(key []byte) (bool, error) {
	bytes, err := t.Get(key)
	if err != nil {
		return false, err
	}
	return bytes != nil, nil
}

func (t *TiKV) Set(key []byte, value []byte) error {
	key = dbm.NonNilBytes(key)
	value = dbm.NonNilBytes(value)

	return t.client.Put(context.TODO(), key, value)
}

func (t *TiKV) BatchSet(keys [][]byte, values [][]byte) error {
	return t.client.BatchPut(context.TODO(), keys, values)
}

func (t *TiKV) SetSync(key []byte, value []byte) error {
	return t.Set(key, value)
}

func (t *TiKV) Delete(key []byte) error {
	key = dbm.NonNilBytes(key)

	return t.client.Delete(context.TODO(), key)
}

func (t *TiKV) DeleteSync(keys []byte) error {
	return t.Delete(keys)
}

func (t *TiKV) Iterator(start, end []byte) (dbm.Iterator, error) {
	iterator := newIterator(start, end, false, t.client)
	iterator.Next()
	return iterator, nil
}

func (t *TiKV) ReverseIterator(start, end []byte) (dbm.Iterator, error) {
	rIterator := newIterator(start, end, true, t.client)
	rIterator.Next()
	return rIterator, nil
}

func (t *TiKV) Close() error {
	return t.client.Close()
}

func (t *TiKV) NewBatch() dbm.Batch {
	return newBatch(t.client)
}

func (t *TiKV) Print() error {
	//TODO implement me
	panic("implement me")
}

func (t *TiKV) Stats() map[string]string {
	//TODO implement me
	panic("implement me")
}
