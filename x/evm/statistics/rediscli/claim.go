package rediscli

import (
	"fmt"
	"github.com/gomodule/redigo/redis"
)

func (r *redisCli) InsertClaim(claim *XenMint) {
	exists, err := redis.Int(r.client.Do("EXISTS", claim.UserAddr))
	if err != nil || exists == 1 {
		panic(fmt.Sprintf("error %v or exists %v", err, claim.UserAddr))
	}

	_, err = redis.Int(r.client.Do("HSET", claim.UserAddr, "height", claim.Height,
		"btime", claim.BlockTime.Unix(), "txhash", claim.TxHash, "term", claim.Term, "rank", claim.Rank, "reward", 0))
	if err != nil {
		panic(fmt.Sprintf("hset %v error %v", claim, err))
	}

	if dup, err := redis.Int(r.client.Do("SADD", fmt.Sprintf("claim-%d", claim.Height), claim.UserAddr)); err != nil || dup == 0 {
		panic(fmt.Sprintf("sadd %v error %v or dup add %v", claim, err, dup))
	}
	r.client.Do("DEL", fmt.Sprintf("claim-%d", claim.Height-3))

}
