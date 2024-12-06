package redis_cgi

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/okex/exchain/libs/tendermint/delta/internal"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/types"
	"sync"
	"time"
)

const (
	lockerExpire = 4 * time.Second
)

var (
	mostRecentHeightKey string
	deltaLockerKey      string
)

var once sync.Once

// init initialize the mostRecentHeightKey and deltaLockerKey
// the keys are based types.DeltaVersion, which can specified by user.
func (r *RedisClient) init() {
	const (
		mostRecentHeight = "MostRecentHeight"
		deltaLocker      = "DeltaLocker"
	)
	once.Do(func() {
		mostRecentHeightKey = fmt.Sprintf("dds:%d:%s", types.DeltaVersion, mostRecentHeight)
		deltaLockerKey = fmt.Sprintf("dds:%d:%s", types.DeltaVersion, deltaLocker)
	})
}

type RedisClient struct {
	rdb    *redis.Client
	ttl    time.Duration
	logger log.Logger
}

func NewRedisClientWithURL(redisURL string, ttl time.Duration, l log.Logger) (*RedisClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		l.Error("parse redis url error", err)
		return nil, err
	}

	rdb := redis.NewClient(opt)
	redisClient := RedisClient{rdb, ttl, l}
	redisClient.init()

	return &redisClient, nil
}

func NewRedisClient(url, auth string, ttl time.Duration, db int, l log.Logger) *RedisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:     url,
		Password: auth, // no password set
		DB:       db,   // use select DB
	})
	redisClient := RedisClient{rdb, ttl, l}
	redisClient.init()

	return &redisClient
}

func (r *RedisClient) GetLocker() bool {
	res, err := r.rdb.SetNX(context.Background(), deltaLockerKey, true, lockerExpire).Result()
	if err != nil {
		r.logger.Error("GetLocker err", err)
		return false
	}
	return res
}

func (r *RedisClient) ReleaseLocker() {
	_, err := r.rdb.Del(context.Background(), deltaLockerKey).Result()
	if err != nil {
		r.logger.Error("Failed to Release Locker", "err", err)
	}
}

// return bool: if change the value of latest_height, need to upload
func (r *RedisClient) ResetMostRecentHeightAfterUpload(targetHeight int64, upload func(int64) bool) (bool, int64, error) {
	var res bool
	mrh, err := r.rdb.Get(context.Background(), mostRecentHeightKey).Int64()
	if err != nil && err != redis.Nil {
		return res, mrh, err
	}

	if mrh < targetHeight && upload(mrh) {
		err = r.rdb.Set(context.Background(), mostRecentHeightKey, targetHeight, 0).Err()
		if err == nil {
			res = true
			r.logger.Info("Reset most recent height", "new-mrh", targetHeight, "old-mrh", mrh)
		} else {
			r.logger.Error("Failed to reset most recent height",
				"target-mrh", targetHeight,
				"existing-mrh", mrh, "err", err)
		}
	}
	return res, mrh, err
}

func (r *RedisClient) SetBlock(height int64, bytes []byte) error {
	if len(bytes) == 0 {
		return fmt.Errorf("block is empty")
	}
	req := r.rdb.SetNX(context.Background(), genBlockKey(height), bytes, r.ttl)
	return req.Err()
}

func (r *RedisClient) SetDeltas(height int64, bytes []byte) error {
	if len(bytes) == 0 {
		return fmt.Errorf("delta is empty")
	}
	req := r.rdb.SetNX(context.Background(), genDeltaKey(height), bytes, r.ttl)
	return req.Err()
}

func (r *RedisClient) GetBlock(height int64) ([]byte, error) {
	bytes, err := r.rdb.Get(context.Background(), genBlockKey(height)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("get empty block")
	}
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (r *RedisClient) GetDeltas(height int64) ([]byte, error, int64) {
	mrh := r.getMostRecentHeight()
	bytes, err := r.rdb.Get(context.Background(), genDeltaKey(height)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("get empty delta"), mrh
	}
	return bytes, err, mrh
}

func (r *RedisClient) getMostRecentHeight() (mrh int64) {
	mrh = -1
	h, err := r.rdb.Get(context.Background(), mostRecentHeightKey).Int64()
	if err == nil {
		mrh = h
	} else if err == redis.Nil {
		mrh = 0
	}
	return
}

func genBlockKey(height int64) string {
	return fmt.Sprintf("BH:%d", height)
}

func genDeltaKey(height int64) string {
	return internal.GenDeltaKey(height)
}
