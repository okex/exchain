package keeper

import "C"

import (
	"errors"
	wasmvm "github.com/CosmWasm/wasmvm"
	"github.com/CosmWasm/wasmvm/api"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"unsafe"
)

var (
	wasmKeeper Keeper

	// wasmvm cache param
	filePath            string
	supportedFeatures   string
	contractMemoryLimit uint32 = ContractMemoryLimit
	contractDebugMode   bool
	memoryCacheSize     uint32

	wasmCache api.Cache
)

func SetWasmKeeper(k *Keeper) {
	wasmKeeper = *k
}

func SetWasmCache(cache api.Cache) {
	wasmCache = cache
}

func GetWasmCacheInfo() (wasmvm.GoAPI, api.Cache) {
	return cosmwasmAPI, wasmCache
}

func GetWasmCallInfo(q unsafe.Pointer, contractAddress, storeAddress string) ([]byte, wasmvm.KVStore, wasmvm.Querier, wasmvm.GasMeter, error) {
	goQuerier := *(*wasmvm.Querier)(q)
	qq, ok := goQuerier.(QueryHandler)
	if !ok {
		return nil, nil, nil, nil, errors.New("can not switch the pointer to the QueryHandler")
	}
	return getCallerInfo(qq.Ctx, contractAddress, storeAddress)
}

func getCallerInfo(ctx sdk.Context, contractAddress, storeAddress string) ([]byte, wasmvm.KVStore, wasmvm.Querier, wasmvm.GasMeter, error) {
	cAddr, err := sdk.WasmAddressFromBech32(contractAddress)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// 1. get wasm code from contractAddress
	_, codeInfo, prefixStore, err := wasmKeeper.contractInstance(ctx, cAddr)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// 2. contractAddress == storeAddress and direct return
	if contractAddress == storeAddress {
		queryHandler := wasmKeeper.newQueryHandler(ctx, cAddr)
		return codeInfo.CodeHash, prefixStore, queryHandler, wasmKeeper.gasMeter(ctx), nil
	}
	// 3. get store from storeaddress
	sAddr, err := sdk.WasmAddressFromBech32(storeAddress)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	_, _, prefixStore, err = wasmKeeper.contractInstance(ctx, sAddr)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	queryHandler := wasmKeeper.newQueryHandler(ctx, sAddr)
	return codeInfo.CodeHash, prefixStore, queryHandler, wasmKeeper.gasMeter(ctx), nil
}
