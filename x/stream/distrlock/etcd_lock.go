package distrlock

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/etcd-io/etcd/clientv3/concurrency"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	FlagEtcdLock = "etcd_lock"
)

type EtcdLock struct {
	Client  *clientv3.Client
	Session *concurrency.Session
	Mut     *concurrency.Mutex
	logger  log.Logger
}

func (lock *EtcdLock) TryLockBlock(key, token string) {
	mut := concurrency.NewMutex(lock.Session, key)
	err := mut.Lock(context.Background())
	if err != nil {
		lock.logger.Error(fmt.Sprintf("etcd mutex lock failed : %s", err.Error()))
		panic(err)
	}
	_, err = lock.Client.Put(context.Background(), key, token)
	if err != nil {
		lock.logger.Error(fmt.Sprintf("etcd client put key: %s value: %s failed : %s", key, token, err.Error()))
		panic(err)
	}
	lock.Mut = mut
}

func (lock *EtcdLock) UnLock(key, token string) {
	_, err := lock.Client.Delete(context.Background(), key)
	if err != nil {
		lock.logger.Error(fmt.Sprintf("etcd client delete key: %s value: %s failed : %s", key, token, err.Error()))
		panic(err)
	}
	if lock.Mut == nil {
		lock.logger.Error("etcd mutex is nil")
		panic(err)
	}
	err = lock.Mut.Unlock(context.Background())
	if err != nil {
		lock.logger.Error(fmt.Sprintf("etcd mutex unlock failed : %s", err.Error()))
		panic(err)
	}
}

// NewEtcdDistrLock creates EtcdLock
func NewEtcdDistrLock(url string, logger log.Logger) (*EtcdLock, error) {
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{url}, DialTimeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	se, err := concurrency.NewSession(cli, concurrency.WithTTL(1))
	if err != nil {
		return nil, err
	}

	redisLock := &EtcdLock{
		Client:  cli,
		Session: se,
		logger:  logger,
	}
	return redisLock, nil
}
