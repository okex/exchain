package consensus

import (
	"os"
	"time"

	"github.com/okex/exchain/libs/tendermint/delta"
	"github.com/okex/exchain/libs/tendermint/delta/redis-cgi"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/types"
	"github.com/spf13/viper"
)

type BlockContext struct {
	deltaBroker      delta.DeltaBroker
	enableBlockRedis bool
	logger           log.Logger
}

func newBlockContext() *BlockContext {
	bc := &BlockContext{
		enableBlockRedis: types.DownloadDelta,
		logger:           log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "consensus"),
	}
	return bc
}

func (bc *BlockContext) init() {
	// todo use flag
	if true {
		url := viper.GetString(types.FlagRedisUrl)
		auth := viper.GetString(types.FlagRedisAuth)
		expire := time.Duration(viper.GetInt(types.FlagRedisExpire)) * time.Second
		dbNum := viper.GetInt(types.FlagRedisDB)
		if dbNum < 0 || dbNum > 15 {
			panic("redis-db only support 0~15")
		}
		bc.deltaBroker = redis_cgi.NewRedisClient(url, auth, expire, dbNum, bc.logger)
		bc.logger.Info("Init redis broker", "url", url)
	}
}
