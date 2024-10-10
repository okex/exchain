package delta

import "github.com/go-redis/redis/v8"

type DeltaBroker interface {
	GetLocker() bool
	ReleaseLocker()
	ResetMostRecentHeightAfterUpload(height int64, upload func(int64) bool) (bool, int64, error)
	SetDeltas(height int64, bytes []byte) error
	GetDeltas(height int64) ([]byte, error, int64)
	SetBlock(height int64, round int, bytes []byte) error
	GetBlock(height int64, round int) ([]byte, error)
	SetBlockParts(height int64, round int, bytes []byte) error
	GetBlockParts(height int64, round int) ([]byte, error)
	Pub(msg []byte)
	SubChannel() *redis.PubSub
}
