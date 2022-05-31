package types

import (
	"sync"
	"sync/atomic"
)

var evmParamsCache = NewCache()

type Cache struct {
	paramsCache      Params
	needParamsUpdate bool
	paramsMutex      sync.RWMutex

	blockedContractMethodsCache map[string]BlockedContract
	needBlockedUpdate           bool
	blockedMutex                sync.RWMutex
}

func NewCache() *Cache {
	return &Cache{
		paramsCache:                 DefaultParams(),
		blockedContractMethodsCache: make(map[string]BlockedContract, 0),
		needParamsUpdate:            true,
		needBlockedUpdate:           true,
	}
}

func (c *Cache) UpdateParams(params Params) {
	c.paramsMutex.Lock()
	defer c.paramsMutex.Unlock()
	c.paramsCache = params
	c.needParamsUpdate = false
}

func (c *Cache) SetNeedParamsUpdate() {
	c.paramsMutex.Lock()
	defer c.paramsMutex.Unlock()
	c.needParamsUpdate = true
}

func (c *Cache) IsNeedParamsUpdate() bool {
	c.paramsMutex.RLock()
	defer c.paramsMutex.RUnlock()
	return c.needParamsUpdate
}

func (c *Cache) GetParams() Params {
	c.paramsMutex.RLock()
	defer c.paramsMutex.RUnlock()
	return NewParams(c.paramsCache.EnableCreate,
		c.paramsCache.EnableCall,
		c.paramsCache.EnableContractDeploymentWhitelist,
		c.paramsCache.EnableContractBlockedList,
		c.paramsCache.MaxGasLimitPerTx,
		c.paramsCache.ExtraEIPs...)
}

func (c *Cache) SetNeedBlockedUpdate() {
	c.blockedMutex.Lock()
	defer c.blockedMutex.Unlock()
	c.needBlockedUpdate = true
}

func (c *Cache) IsNeedBlockedUpdate() bool {
	c.blockedMutex.RLock()
	defer c.blockedMutex.RUnlock()
	return c.needBlockedUpdate
}

func (c *Cache) GetBlockedContractMethod(addr string) (contract *BlockedContract) {
	c.blockedMutex.RLock()
	bc, ok := c.blockedContractMethodsCache[addr]
	c.blockedMutex.RUnlock()
	if ok {
		return NewBlockContract(bc.Address, bc.BlockMethods)
	}
	return nil
}

func (c *Cache) UpdateBlockedContractMethod(bcl BlockedContractList) {
	c.blockedMutex.Lock()
	c.blockedContractMethodsCache = make(map[string]BlockedContract, 0)
	for i, _ := range bcl {
		c.blockedContractMethodsCache[bcl[i].Address.String()] = bcl[i]
	}
	c.blockedMutex.Unlock()
	c.needBlockedUpdate = false
}

func SetEvmParamsNeedUpdate() {
	GetEvmParamsCache().SetNeedParamsUpdate()
}

func GetEvmParamsCache() *Cache {
	return evmParamsCache
}

var maxGasLimitPerTx uint64 = DefaultMaxGasLimitPerTx

// SetMaxGasLimitPerTx sets maxGasLimitPerTx safely.
func SetMaxGasLimitPerTx(maxGasLimitPerTx uint64) {
	atomic.StoreUint64(&maxGasLimitPerTx, maxGasLimitPerTx)
}

// GetMaxGasLimitPerTx gets maxGasLimitPerTx safely.
func GetMaxGasLimitPerTx() uint64 {
	return atomic.LoadUint64(&maxGasLimitPerTx)
}
