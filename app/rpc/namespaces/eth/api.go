package eth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	lru "github.com/hashicorp/golang-lru"
	"github.com/spf13/viper"

	appconfig "github.com/okex/exchain/app/config"
	"github.com/okex/exchain/libs/tendermint/mempool"

	"github.com/okex/exchain/app/config"
	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	"github.com/okex/exchain/app/crypto/hd"
	"github.com/okex/exchain/app/rpc/backend"
	"github.com/okex/exchain/app/rpc/monitor"
	"github.com/okex/exchain/app/rpc/namespaces/eth/simulation"
	rpctypes "github.com/okex/exchain/app/rpc/types"
	ethermint "github.com/okex/exchain/app/types"
	"github.com/okex/exchain/app/utils"
	clientcontext "github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/client/flags"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	"github.com/okex/exchain/libs/cosmos-sdk/crypto/keys"
	cmserver "github.com/okex/exchain/libs/cosmos-sdk/server"
	"github.com/okex/exchain/libs/cosmos-sdk/store/mpt"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	authclient "github.com/okex/exchain/libs/cosmos-sdk/x/auth/client/utils"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	authtypes "github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto/merkle"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/okex/exchain/x/erc20"
	"github.com/okex/exchain/x/evm"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/okex/exchain/x/evm/watcher"
	"github.com/okex/exchain/x/vmbridge"
)

const (
	CacheOfEthCallLru = 40960

	FlagFastQueryThreshold = "fast-query-threshold"

	NameSpace = "eth"

	EvmHookGasEstimate = uint64(60000)
	EvmDefaultGasLimit = uint64(21000)

	FlagAllowUnprotectedTxs = "rpc.allow-unprotected-txs"
)

// PublicEthereumAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicEthereumAPI struct {
	ctx                context.Context
	clientCtx          clientcontext.CLIContext
	chainIDEpoch       *big.Int
	logger             log.Logger
	backend            backend.Backend
	keys               []ethsecp256k1.PrivKey // unlocked keys
	nonceLock          *rpctypes.AddrLocker
	keyringLock        sync.Mutex
	gasPrice           *hexutil.Big
	wrappedBackend     *watcher.Querier
	watcherBackend     *watcher.Watcher
	evmFactory         simulation.EvmFactory
	txPool             *TxPool
	Metrics            *monitor.RpcMetrics
	callCache          *lru.Cache
	cdc                *codec.Codec
	fastQueryThreshold uint64
	rateLimiters       map[string]*rate.Limiter
}

func (api *PublicEthereumAPI) GetRateLimiter(apiName string) *rate.Limiter {
	if api.rateLimiters == nil {
		return nil
	}
	return api.rateLimiters[apiName]
}

// NewAPI creates an instance of the public ETH Web3 API.
func NewAPI(rateLimiters map[string]*rate.Limiter,
	clientCtx clientcontext.CLIContext, log log.Logger, backend backend.Backend, nonceLock *rpctypes.AddrLocker,
	keys ...ethsecp256k1.PrivKey,
) *PublicEthereumAPI {

	epoch, err := ethermint.ParseChainID(clientCtx.ChainID)
	if err != nil {
		panic(err)
	}

	api := &PublicEthereumAPI{
		ctx:                context.Background(),
		clientCtx:          clientCtx,
		chainIDEpoch:       epoch,
		logger:             log.With("module", "json-rpc", "namespace", NameSpace),
		backend:            backend,
		keys:               keys,
		nonceLock:          nonceLock,
		gasPrice:           ParseGasPrice(),
		wrappedBackend:     watcher.NewQuerier(),
		watcherBackend:     watcher.NewWatcher(log),
		fastQueryThreshold: viper.GetUint64(FlagFastQueryThreshold),
		rateLimiters:       rateLimiters,
	}
	api.evmFactory = simulation.NewEvmFactory(clientCtx.ChainID, api.wrappedBackend)
	module := evm.AppModuleBasic{}
	api.cdc = codec.New()
	module.RegisterCodec(api.cdc)
	if watcher.IsWatcherEnabled() {
		callCache, err := lru.New(CacheOfEthCallLru)
		if err != nil {
			panic(err)
		}
		api.callCache = callCache
	}

	if err := api.GetKeyringInfo(); err != nil {
		api.logger.Error("failed to get keybase info", "error", err)
	}

	if viper.GetBool(FlagEnableTxPool) {
		api.txPool = NewTxPool(clientCtx, api)
		go api.txPool.broadcastPeriod(api)
	}

	if viper.GetBool(monitor.FlagEnableMonitor) {
		api.Metrics = monitor.MakeMonitorMetrics(NameSpace)
	}
	return api
}

// GetKeyringInfo checks if the keyring is present on the client context. If not, it creates a new
// instance and sets it to the client context for later usage.
func (api *PublicEthereumAPI) GetKeyringInfo() error {
	api.keyringLock.Lock()
	defer api.keyringLock.Unlock()

	if api.clientCtx.Keybase != nil {
		return nil
	}
	backendType := viper.GetString(flags.FlagKeyringBackend)
	if backendType == keys.BackendFile {
		backendType = keys.BackendFileForRPC
	}
	keybase, err := keys.NewKeyring(
		sdk.KeyringServiceName(),
		backendType,
		viper.GetString(cmserver.FlagUlockKeyHome),
		api.clientCtx.Input,
		hd.EthSecp256k1Options()...,
	)
	if err != nil {
		return err
	}

	api.clientCtx.Keybase = keybase
	return nil
}

// ClientCtx returns the Cosmos SDK client context.
func (api *PublicEthereumAPI) ClientCtx() clientcontext.CLIContext {
	return api.clientCtx
}

func (api *PublicEthereumAPI) GetCodec() *codec.Codec {
	return api.cdc
}

// GetKeys returns the Cosmos SDK client context.
func (api *PublicEthereumAPI) GetKeys() []ethsecp256k1.PrivKey {
	return api.keys
}

// SetKeys sets the given key slice to the set of private keys
func (api *PublicEthereumAPI) SetKeys(keys []ethsecp256k1.PrivKey) {
	api.keys = keys
}

// ProtocolVersion returns the supported Ethereum protocol version.
func (api *PublicEthereumAPI) ProtocolVersion() hexutil.Uint {
	monitor := monitor.GetMonitor("eth_protocolVersion", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()
	return hexutil.Uint(ethermint.ProtocolVersion)
}

// ChainId returns the chain's identifier in hex format
func (api *PublicEthereumAPI) ChainId() (hexutil.Uint, error) { // nolint
	api.logger.Debug("eth_chainId")
	return hexutil.Uint(uint(api.chainIDEpoch.Uint64())), nil
}

// Syncing returns whether or not the current node is syncing with other peers. Returns false if not, or a struct
// outlining the state of the sync if it is.
func (api *PublicEthereumAPI) Syncing() (interface{}, error) {
	monitor := monitor.GetMonitor("eth_syncing", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()
	status, err := api.clientCtx.Client.Status()
	if err != nil {
		return false, err
	}

	if !status.SyncInfo.CatchingUp {
		return false, nil
	}

	return map[string]interface{}{
		"startingBlock": hexutil.Uint64(status.SyncInfo.EarliestBlockHeight),
		"currentBlock":  hexutil.Uint64(status.SyncInfo.LatestBlockHeight),
		"highestBlock":  hexutil.Uint64(0), // NA
		// "pulledStates":  nil, // NA
		// "knownStates":   nil, // NA
	}, nil
}

// Coinbase is the address that staking rewards will be send to (alias for Etherbase).
func (api *PublicEthereumAPI) Coinbase() (common.Address, error) {
	api.logger.Debug("eth_coinbase")

	node, err := api.clientCtx.GetNode()
	if err != nil {
		return common.Address{}, err
	}

	status, err := node.Status()
	if err != nil {
		return common.Address{}, err
	}

	return common.BytesToAddress(status.ValidatorInfo.Address.Bytes()), nil
}

// Mining returns whether or not this node is currently mining. Always false.
func (api *PublicEthereumAPI) Mining() bool {
	api.logger.Debug("eth_mining")
	return false
}

// Hashrate returns the current node's hashrate. Always 0.
func (api *PublicEthereumAPI) Hashrate() hexutil.Uint64 {
	api.logger.Debug("eth_hashrate")
	return 0
}

// GasPrice returns the current gas price based on Ethermint's gas price oracle.
func (api *PublicEthereumAPI) GasPrice() *hexutil.Big {
	monitor := monitor.GetMonitor("eth_gasPrice", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()

	minGP := (*big.Int)(api.gasPrice)
	maxGP := new(big.Int).Mul(minGP, big.NewInt(5000))

	rgp := new(big.Int).Set(minGP)
	if appconfig.GetOecConfig().GetDynamicGpMode() != tmtypes.MinimalGpMode {
		// If current block is not congested, rgp == minimal gas price.
		if mempool.IsCongested {
			rgp.Set(mempool.GlobalRecommendedGP)
		}

		if rgp.Cmp(minGP) == -1 {
			rgp.Set(minGP)
		}

		if appconfig.GetOecConfig().GetDynamicGpCoefficient() > 1 {
			coefficient := big.NewInt(int64(appconfig.GetOecConfig().GetDynamicGpCoefficient()))
			rgp = new(big.Int).Mul(rgp, coefficient)
		}

		if rgp.Cmp(maxGP) == 1 {
			rgp.Set(maxGP)
		}
	}

	return (*hexutil.Big)(rgp)
}

func (api *PublicEthereumAPI) GasPriceIn3Gears() *rpctypes.GPIn3Gears {
	monitor := monitor.GetMonitor("eth_gasPriceIn3Gears", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()

	minGP := (*big.Int)(api.gasPrice)
	maxGP := new(big.Int).Mul(minGP, big.NewInt(5000))

	avgGP := new(big.Int).Set(minGP)
	if appconfig.GetOecConfig().GetDynamicGpMode() != tmtypes.MinimalGpMode {
		if mempool.IsCongested {
			avgGP.Set(mempool.GlobalRecommendedGP)
		}

		if avgGP.Cmp(minGP) == -1 {
			avgGP.Set(minGP)
		}

		if appconfig.GetOecConfig().GetDynamicGpCoefficient() > 1 {
			coefficient := big.NewInt(int64(appconfig.GetOecConfig().GetDynamicGpCoefficient()))
			avgGP = new(big.Int).Mul(avgGP, coefficient)
		}

		if avgGP.Cmp(maxGP) == 1 {
			avgGP.Set(maxGP)
		}
	}

	// safe low GP = average GP * 0.5, but it will not be less than the minimal GP.
	safeGp := new(big.Int).Quo(avgGP, big.NewInt(2))
	if safeGp.Cmp(minGP) == -1 {
		safeGp.Set(minGP)
	}
	// fastest GP = average GP * 1.5, but it will not be greater than the max GP.
	fastestGp := new(big.Int).Add(avgGP, new(big.Int).Quo(avgGP, big.NewInt(2)))
	if fastestGp.Cmp(maxGP) == 1 {
		fastestGp.Set(maxGP)
	}

	res := rpctypes.NewGPIn3Gears(safeGp, avgGP, fastestGp)
	return &res
}

// Accounts returns the list of accounts available to this node.
func (api *PublicEthereumAPI) Accounts() ([]common.Address, error) {
	monitor := monitor.GetMonitor("eth_accounts", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()
	return api.accounts()
}

func (api *PublicEthereumAPI) accounts() ([]common.Address, error) {
	api.keyringLock.Lock()
	defer api.keyringLock.Unlock()

	addresses := make([]common.Address, 0) // return [] instead of nil if empty

	infos, err := api.clientCtx.Keybase.List()
	if err != nil {
		return addresses, err
	}

	for _, info := range infos {
		addressBytes := info.GetPubKey().Address().Bytes()
		addresses = append(addresses, common.BytesToAddress(addressBytes))
	}

	return addresses, nil
}

// BlockNumber returns the current block number.
func (api *PublicEthereumAPI) BlockNumber() (hexutil.Uint64, error) {
	monitor := monitor.GetMonitor("eth_blockNumber", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd()
	return api.backend.BlockNumber()
}

// GetBalance returns the provided account's balance up to the provided block number.
func (api *PublicEthereumAPI) GetBalance(address common.Address, blockNrOrHash rpctypes.BlockNumberOrHash) (*hexutil.Big, error) {
	monitor := monitor.GetMonitor("eth_getBalance", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "block number", blockNrOrHash)
	blockNum, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	useWatchBackend := api.useWatchBackend(blockNum)
	if useWatchBackend {
		acc, err := api.wrappedBackend.MustGetAccount(address.Bytes())
		if err == nil {
			balance := acc.GetCoins().AmountOf(sdk.DefaultBondDenom).BigInt()
			if balance == nil {
				return (*hexutil.Big)(sdk.ZeroInt().BigInt()), nil
			}
			return (*hexutil.Big)(balance), nil
		}
	}

	clientCtx := api.clientCtx
	if !(blockNum == rpctypes.PendingBlockNumber || blockNum == rpctypes.LatestBlockNumber) && !useWatchBackend {
		clientCtx = api.clientCtx.WithHeight(blockNum.Int64())
	}

	bs, err := api.clientCtx.Codec.MarshalJSON(auth.NewQueryAccountParams(address.Bytes()))
	if err != nil {
		return nil, err
	}

	res, _, err := clientCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", auth.QuerierRoute, auth.QueryAccount), bs)
	if err != nil {
		if isAccountNotExistErr(err) {
			if useWatchBackend {
				api.saveZeroAccount(address)
			}
			return (*hexutil.Big)(sdk.ZeroInt().BigInt()), nil
		}
		return nil, err
	}

	var account ethermint.EthAccount
	if err := api.clientCtx.Codec.UnmarshalJSON(res, &account); err != nil {
		return nil, err
	}

	val := account.Balance(sdk.DefaultBondDenom).BigInt()
	if useWatchBackend {
		api.watcherBackend.CommitAccountToRpcDb(account)
	}

	if blockNum != rpctypes.PendingBlockNumber {
		return (*hexutil.Big)(val), nil
	}

	// update the address balance with the pending transactions value (if applicable)
	pendingTxs, err := api.backend.UserPendingTransactions(address.String(), -1)
	if err != nil {
		return nil, err
	}

	for _, tx := range pendingTxs {
		if tx == nil {
			continue
		}

		if tx.From == address {
			val = new(big.Int).Sub(val, tx.Value.ToInt())
		}
		if *tx.To == address {
			val = new(big.Int).Add(val, tx.Value.ToInt())
		}
	}

	return (*hexutil.Big)(val), nil
}

// GetAccount returns the provided account's balance up to the provided block number.
func (api *PublicEthereumAPI) GetAccount(address common.Address) (*ethermint.EthAccount, error) {
	acc, err := api.wrappedBackend.MustGetAccount(address.Bytes())
	if err == nil {
		return acc, nil
	}
	clientCtx := api.clientCtx

	bs, err := api.clientCtx.Codec.MarshalJSON(auth.NewQueryAccountParams(address.Bytes()))
	if err != nil {
		return nil, err
	}

	res, _, err := clientCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", auth.QuerierRoute, auth.QueryAccount), bs)
	if err != nil {
		return nil, err
	}

	var account ethermint.EthAccount
	if err := api.clientCtx.Codec.UnmarshalJSON(res, &account); err != nil {
		return nil, err
	}

	api.watcherBackend.CommitAccountToRpcDb(account)

	return &account, nil
}

func (api *PublicEthereumAPI) getStorageAt(address common.Address, key []byte, blockNum rpctypes.BlockNumber, directlyKey bool) (hexutil.Bytes, error) {
	clientCtx := api.clientCtx.WithHeight(blockNum.Int64())
	useWatchBackend := api.useWatchBackend(blockNum)

	qWatchdbKey := key
	if useWatchBackend {
		if !directlyKey {
			qWatchdbKey = evmtypes.GetStorageByAddressKey(address.Bytes(), key).Bytes()
		}
		res, err := api.wrappedBackend.MustGetState(address, qWatchdbKey)
		if err == nil {
			return res, nil
		}
	}

	var queryStr = ""
	if !directlyKey {
		queryStr = fmt.Sprintf("custom/%s/storage/%s/%X", evmtypes.ModuleName, address.Hex(), key)
	} else {
		queryStr = fmt.Sprintf("custom/%s/storageKey/%s/%X", evmtypes.ModuleName, address.Hex(), key)
	}

	res, _, err := clientCtx.QueryWithData(queryStr, nil)
	if err != nil {
		return nil, err
	}

	var out evmtypes.QueryResStorage
	api.clientCtx.Codec.MustUnmarshalJSON(res, &out)
	if useWatchBackend {
		api.watcherBackend.CommitStateToRpcDb(address, qWatchdbKey, out.Value)
	}
	return out.Value, nil
}

// GetStorageAt returns the contract storage at the given address, block number, and key.
func (api *PublicEthereumAPI) GetStorageAt(address common.Address, key string, blockNrOrHash rpctypes.BlockNumberOrHash) (hexutil.Bytes, error) {
	monitor := monitor.GetMonitor("eth_getStorageAt", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "key", key, "block number", blockNrOrHash)
	rateLimiter := api.GetRateLimiter("eth_getStorageAt")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	blockNum, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	return api.getStorageAt(address, common.HexToHash(key).Bytes(), blockNum, false)
}

// GetStorageAtInternal returns the contract storage at the given address, block number, and key.
func (api *PublicEthereumAPI) GetStorageAtInternal(address common.Address, key []byte) (hexutil.Bytes, error) {
	return api.getStorageAt(address, key, 0, true)
}

// GetTransactionCount returns the number of transactions at the given address up to the given block number.
func (api *PublicEthereumAPI) GetTransactionCount(address common.Address, blockNrOrHash rpctypes.BlockNumberOrHash) (*hexutil.Uint64, error) {
	monitor := monitor.GetMonitor("eth_getTransactionCount", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "block number", blockNrOrHash)
	rateLimiter := api.GetRateLimiter("eth_getTransactionCount")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	blockNum, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	// do not support block number param when node is pruning everything
	if api.backend.PruneEverything() && blockNum != rpctypes.PendingBlockNumber {
		blockNum = rpctypes.LatestBlockNumber
	}

	clientCtx := api.clientCtx
	pending := blockNum == rpctypes.PendingBlockNumber
	// pass the given block height to the context if the height is not pending or latest
	if !pending && blockNum != rpctypes.LatestBlockNumber {
		clientCtx = api.clientCtx.WithHeight(blockNum.Int64())
	}
	useWatchBackend := api.useWatchBackend(blockNum)
	nonce, err := api.accountNonce(clientCtx, address, pending, useWatchBackend)
	if err != nil {
		return nil, err
	}

	n := hexutil.Uint64(nonce)
	return &n, nil
}

// GetBlockTransactionCountByHash returns the number of transactions in the block identified by hash.
func (api *PublicEthereumAPI) GetBlockTransactionCountByHash(hash common.Hash) *hexutil.Uint {
	monitor := monitor.GetMonitor("eth_getBlockTransactionCountByHash", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("hash", hash)
	rateLimiter := api.GetRateLimiter("eth_getBlockTransactionCountByHash")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil
	}
	res, _, err := api.clientCtx.Query(fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryHashToHeight, hash.Hex()))
	if err != nil {
		return nil
	}

	var out evmtypes.QueryResBlockNumber
	if err := api.clientCtx.Codec.UnmarshalJSON(res, &out); err != nil {
		return nil
	}

	resBlock, err := api.backend.Block(&out.Number)
	if err != nil {
		return nil
	}

	n := hexutil.Uint(len(resBlock.Block.Txs))
	return &n
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block identified by its height.
func (api *PublicEthereumAPI) GetBlockTransactionCountByNumber(blockNum rpctypes.BlockNumber) *hexutil.Uint {
	monitor := monitor.GetMonitor("eth_getBlockTransactionCountByNumber", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("block number", blockNum)
	var (
		height  int64
		err     error
		txCount hexutil.Uint
		txs     int
	)

	switch blockNum {
	case rpctypes.PendingBlockNumber:
		height, err = api.backend.LatestBlockNumber()
		if err != nil {
			return nil
		}
		resBlock, err := api.backend.Block(&height)
		if err != nil {
			return nil
		}
		// get the pending transaction count
		pendingCnt, err := api.backend.PendingTransactionCnt()
		if err != nil {
			return nil
		}
		txs = len(resBlock.Block.Txs) + pendingCnt
	case rpctypes.LatestBlockNumber:
		height, err = api.backend.LatestBlockNumber()
		if err != nil {
			return nil
		}
		resBlock, err := api.backend.Block(&height)
		if err != nil {
			return nil
		}
		txs = len(resBlock.Block.Txs)
	default:
		height = blockNum.Int64()
		resBlock, err := api.backend.Block(&height)
		if err != nil {
			return nil
		}
		txs = len(resBlock.Block.Txs)
	}

	txCount = hexutil.Uint(txs)
	return &txCount
}

// GetUncleCountByBlockHash returns the number of uncles in the block idenfied by hash. Always zero.
func (api *PublicEthereumAPI) GetUncleCountByBlockHash(_ common.Hash) hexutil.Uint {
	return 0
}

// GetUncleCountByBlockNumber returns the number of uncles in the block idenfied by number. Always zero.
func (api *PublicEthereumAPI) GetUncleCountByBlockNumber(_ rpctypes.BlockNumber) hexutil.Uint {
	return 0
}

// GetCode returns the contract code at the given address and block number.
func (api *PublicEthereumAPI) GetCode(address common.Address, blockNrOrHash rpctypes.BlockNumberOrHash) (hexutil.Bytes, error) {
	monitor := monitor.GetMonitor("eth_getCode", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "block number", blockNrOrHash)
	rateLimiter := api.GetRateLimiter("eth_getCode")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	blockNumber, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	code, err := api.wrappedBackend.GetCode(address, uint64(blockNumber))
	if err == nil {
		return code, nil
	}

	clientCtx := api.clientCtx
	if !(blockNumber == rpctypes.PendingBlockNumber || blockNumber == rpctypes.LatestBlockNumber) {
		clientCtx = api.clientCtx.WithHeight(blockNumber.Int64())
	}
	res, _, err := clientCtx.QueryWithData(fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryCode, address.Hex()), nil)
	if err != nil {
		return nil, err
	}

	var out evmtypes.QueryResCode
	api.clientCtx.Codec.MustUnmarshalJSON(res, &out)
	return out.Code, nil
}

// GetCodeByHash returns the contract code at the given address and block number.
func (api *PublicEthereumAPI) GetCodeByHash(hash common.Hash) (hexutil.Bytes, error) {
	code, err := api.wrappedBackend.GetCodeByHash(hash.Bytes())
	if err == nil {
		return code, nil
	}
	clientCtx := api.clientCtx
	res, _, err := clientCtx.QueryWithData(fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryCodeByHash, hash.Hex()), nil)
	if err != nil {
		return nil, err
	}

	var out evmtypes.QueryResCode
	api.clientCtx.Codec.MustUnmarshalJSON(res, &out)

	api.watcherBackend.CommitCodeHashToDb(hash.Bytes(), out.Code)

	return out.Code, nil
}

// GetTransactionLogs returns the logs given a transaction hash.
func (api *PublicEthereumAPI) GetTransactionLogs(txHash common.Hash) ([]*ethtypes.Log, error) {
	api.logger.Debug("eth_getTransactionLogs", "hash", txHash)
	rateLimiter := api.GetRateLimiter("eth_getTransactionLogs")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	return api.backend.GetTransactionLogs(txHash)
}

// Sign signs the provided data using the private key of address via Geth's signature standard.
func (api *PublicEthereumAPI) Sign(address common.Address, data hexutil.Bytes) (hexutil.Bytes, error) {
	monitor := monitor.GetMonitor("eth_sign", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "data", data)
	// TODO: Change this functionality to find an unlocked account by address
	key, exist := rpctypes.GetKeyByAddress(api.keys, address)
	if !exist {
		return nil, keystore.ErrLocked
	}

	// Sign the requested hash with the wallet
	sig, err := crypto.Sign(accounts.TextHash(data), key.ToECDSA())
	if err != nil {
		return nil, err
	}

	sig[crypto.RecoveryIDOffset] += 27 // transform V from 0/1 to 27/28

	return sig, nil
}

// SendTransaction sends an Ethereum transaction.
func (api *PublicEthereumAPI) SendTransaction(args rpctypes.SendTxArgs) (common.Hash, error) {
	monitor := monitor.GetMonitor("eth_sendTransaction", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("args", args)
	// TODO: Change this functionality to find an unlocked account by address

	height, err := api.BlockNumber()
	if err != nil {
		return common.Hash{}, err
	}
	key, exist := rpctypes.GetKeyByAddress(api.keys, *args.From)
	if !exist {
		api.logger.Debug("failed to find key in keyring", "key", args.From)
		return common.Hash{}, keystore.ErrLocked
	}

	// Mutex lock the address' nonce to avoid assigning it to multiple requests
	if args.Nonce == nil {
		api.nonceLock.LockAddr(*args.From)
		defer api.nonceLock.UnlockAddr(*args.From)
	}

	// Assemble transaction from fields
	tx, err := api.generateFromArgs(args)
	if err != nil {
		api.logger.Debug("failed to generate tx", "error", err)
		return common.Hash{}, err
	}

	if err := tx.ValidateBasic(); err != nil {
		api.logger.Debug("tx failed basic validation", "error", err)
		return common.Hash{}, err
	}

	// Sign transaction
	if err := tx.Sign(api.chainIDEpoch, key.ToECDSA()); err != nil {
		api.logger.Debug("failed to sign tx", "error", err)
		return common.Hash{}, err
	}

	var txEncoder sdk.TxEncoder
	if tmtypes.HigherThanVenus(int64(height)) {
		txEncoder = authclient.GetTxEncoder(nil, authclient.WithEthereumTx())
	} else {
		txEncoder = authclient.GetTxEncoder(api.clientCtx.Codec)
	}

	// Encode transaction by RLP encoder
	txBytes, err := txEncoder(tx)
	if err != nil {
		return common.Hash{}, err
	}

	// send chanData to txPool
	if tmtypes.HigherThanVenus(int64(height)) && api.txPool != nil {
		return broadcastTxByTxPool(api, tx, txBytes)
	}

	// Broadcast transaction in sync mode (default)
	// NOTE: If error is encountered on the node, the broadcast will not return an error
	res, err := api.clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return common.Hash{}, err
	}

	if res.Code != abci.CodeTypeOK {
		return CheckError(res)
	}

	// Return transaction hash
	return common.HexToHash(res.TxHash), nil
}

// SendRawTransaction send a raw Ethereum transaction.
func (api *PublicEthereumAPI) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	monitor := monitor.GetMonitor("eth_sendRawTransaction", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("data", data)
	height, err := api.BlockNumber()
	if err != nil {
		return common.Hash{}, err
	}
	txBytes := data
	tx := new(evmtypes.MsgEthereumTx)

	// RLP decode raw transaction bytes
	if err := authtypes.EthereumTxDecode(data, tx); err != nil {
		// Return nil is for when gasLimit overflows uint64
		return common.Hash{}, err
	}

	if !tx.Protected() && !viper.GetBool(FlagAllowUnprotectedTxs) {
		return common.Hash{}, errors.New("only replay-protected (EIP-155) transactions allowed over RPC")
	}

	if !tmtypes.HigherThanVenus(int64(height)) {
		txBytes, err = authclient.GetTxEncoder(api.clientCtx.Codec)(tx)
		if err != nil {
			return common.Hash{}, err
		}
	}

	// send chanData to txPool
	if tmtypes.HigherThanVenus(int64(height)) && api.txPool != nil {
		return broadcastTxByTxPool(api, tx, txBytes)
	}

	// TODO: Possibly log the contract creation address (if recipient address is nil) or tx data
	// If error is encountered on the node, the broadcast will not return an error
	res, err := api.clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return common.Hash{}, err
	}

	if res.Code != abci.CodeTypeOK {
		return CheckError(res)
	}
	// Return transaction hash
	return common.HexToHash(res.TxHash), nil
}

func (api *PublicEthereumAPI) buildKey(args rpctypes.CallArgs) common.Hash {
	latest, err := api.wrappedBackend.GetLatestBlockNumber()
	if err != nil {
		return common.Hash{}
	}
	return sha256.Sum256([]byte(args.String() + strconv.Itoa(int(latest))))
}

func (api *PublicEthereumAPI) getFromCallCache(key common.Hash) ([]byte, bool) {
	if api.callCache == nil {
		return nil, false
	}
	nilKey := common.Hash{}
	if key == nilKey {
		return nil, false
	}
	cacheData, ok := api.callCache.Get(key)
	if ok {
		ret, ok := cacheData.([]byte)
		if ok {
			return ret, true
		}
	}
	return nil, false
}

func (api *PublicEthereumAPI) addCallCache(key common.Hash, data []byte) {
	if api.callCache == nil {
		return
	}
	api.callCache.Add(key, data)
}

// Call performs a raw contract call.
func (api *PublicEthereumAPI) Call(args rpctypes.CallArgs, blockNrOrHash rpctypes.BlockNumberOrHash, overrides *evmtypes.StateOverrides) (hexutil.Bytes, error) {
	monitor := monitor.GetMonitor("eth_call", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("args", args, "block number", blockNrOrHash)
	rateLimiter := api.GetRateLimiter("eth_call")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	if overrides != nil {
		if err := overrides.Check(); err != nil {
			return nil, err
		}
	}
	var key common.Hash
	if overrides == nil {
		key = api.buildKey(args)
		if cacheData, ok := api.getFromCallCache(key); ok {
			return cacheData, nil
		}
	}

	blockNr, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	simRes, err := api.doCall(args, blockNr, big.NewInt(ethermint.DefaultRPCGasLimit), false, overrides)
	if err != nil {
		return []byte{}, TransformDataError(err, "eth_call")
	}

	data, err := evmtypes.DecodeResultData(simRes.Result.Data)
	if err != nil {
		return []byte{}, TransformDataError(err, "eth_call")
	}
	if overrides == nil {
		api.addCallCache(key, data.Ret)
	}
	return data.Ret, nil
}

// DoCall performs a simulated call operation through the evmtypes. It returns the
// estimated gas used on the operation or an error if fails.
func (api *PublicEthereumAPI) doCall(
	args rpctypes.CallArgs,
	blockNum rpctypes.BlockNumber,
	globalGasCap *big.Int,
	isEstimate bool,
	overrides *evmtypes.StateOverrides,
) (*sdk.SimulationResponse, error) {
	var err error
	clientCtx := api.clientCtx
	// pass the given block height to the context if the height is not pending or latest
	if !(blockNum == rpctypes.PendingBlockNumber || blockNum == rpctypes.LatestBlockNumber) {
		clientCtx = api.clientCtx.WithHeight(blockNum.Int64())
	}

	// Set default gas & gas price if none were set
	// Change this to uint64(math.MaxUint64 / 2) if gas cap can be configured
	gas := uint64(ethermint.DefaultRPCGasLimit)
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}
	if globalGasCap != nil && globalGasCap.Uint64() < gas {
		api.logger.Debug("Caller gas above allowance, capping", "requested", gas, "cap", globalGasCap)
		gas = globalGasCap.Uint64()
	}

	// Set gas price using default or parameter if passed in
	gasPrice := new(big.Int).SetUint64(ethermint.DefaultGasPrice)
	if args.GasPrice != nil {
		gasPrice = args.GasPrice.ToInt()
	}

	// Set value for transaction
	value := new(big.Int)
	if args.Value != nil {
		value = args.Value.ToInt()
	}

	// Set Data if provided
	var data []byte
	if args.Data != nil {
		data = []byte(*args.Data)
	}

	// Set sender address or use a default if none specified
	var addr common.Address
	if args.From != nil {
		addr = *args.From
	}

	nonce := uint64(0)
	if isEstimate && args.To == nil && args.Data != nil {
		//only get real nonce when estimate gas and the action is contract deploy
		nonce, _ = api.accountNonce(api.clientCtx, addr, true, true)
	}

	// Create new call message
	msg := evmtypes.NewMsgEthereumTx(nonce, args.To, value, gas, gasPrice, data)
	var overridesBytes []byte
	if overrides != nil {
		if overridesBytes, err = overrides.GetBytes(); err != nil {
			return nil, fmt.Errorf("fail to encode overrides")
		}
	}
	sim := api.evmFactory.BuildSimulator(api)

	// evm tx to cm tx is no need watch db query
	useWatch := api.useWatchBackend(blockNum)
	if useWatch && args.To != nil &&
		api.JudgeEvm2CmTx(args.To.Bytes(), data) {
		useWatch = false
	}

	//only worked when fast-query has been enabled
	if sim != nil && useWatch {
		simRes, err := sim.DoCall(msg, addr.String(), overridesBytes, api.evmFactory.PutBackStorePool)
		if err != nil {
			return simRes, err
		}
		data, err := evmtypes.DecodeResultData(simRes.Result.Data)
		if err != nil {
			return simRes, err
		}
		tempHooks := evm.NewLogProcessEvmHook(
			erc20.NewSendToIbcEventHandler(erc20.Keeper{}),
			erc20.NewSendNative20ToIbcEventHandler(erc20.Keeper{}),
			vmbridge.NewSendToWasmEventHandler(vmbridge.Keeper{}),
			vmbridge.NewCallToWasmEventHandler(vmbridge.Keeper{}),
		)
		if ok := tempHooks.IsCanHooked(data.Logs); !ok {
			return simRes, nil
		}
	}

	//Generate tx to be used to simulate (signature isn't needed)
	var txEncoder sdk.TxEncoder

	// get block height
	height := global.GetGlobalHeight()
	if tmtypes.HigherThanVenus(height) {
		txEncoder = authclient.GetTxEncoder(nil, authclient.WithEthereumTx())
	} else {
		txEncoder = authclient.GetTxEncoder(clientCtx.Codec)
	}

	// rlp encoder need pointer type, amino encoder will first dereference pointers.
	txBytes, err := txEncoder(msg)
	if err != nil {
		return nil, err
	}
	// Transaction simulation through query. only pass from when eth_estimateGas.
	// eth_call's from maybe nil
	var simulatePath string
	var queryData []byte
	if overrides != nil {
		simulatePath = fmt.Sprintf("app/simulateWithOverrides/%s", addr.String())
		queryOverridesData := sdk.SimulateData{
			TxBytes:        txBytes,
			OverridesBytes: overridesBytes,
		}
		queryData, err = json.Marshal(queryOverridesData)
		if err != nil {
			return nil, fmt.Errorf("fail to encode queryData for simulateWithOverrides")
		}

	} else {
		simulatePath = fmt.Sprintf("app/simulate/%s", addr.String())
		queryData = txBytes
	}

	res, _, err := clientCtx.QueryWithData(simulatePath, queryData)
	if err != nil {
		return nil, err
	}

	var simResponse sdk.SimulationResponse
	if err := clientCtx.Codec.UnmarshalBinaryBare(res, &simResponse); err != nil {
		return nil, err
	}

	return &simResponse, nil
}
func (api *PublicEthereumAPI) simDoCall(args rpctypes.CallArgs, cap uint64) (uint64, error) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(gas uint64) (*sdk.SimulationResponse, error) {
		if gas != 0 {
			args.Gas = (*hexutil.Uint64)(&gas)
		}
		return api.doCall(args, 0, big.NewInt(int64(cap)), true, nil)
	}

	// get exact gas limit
	exactResponse, err := executable(0)
	if err != nil {
		return 0, err
	}

	// return if gas is provided by args
	if args.Gas != nil {
		return exactResponse.GasUsed, nil
	}

	// use exact gas to run verify again
	// https://github.com/okex/oec/issues/1784
	verifiedResponse, err := executable(exactResponse.GasInfo.GasUsed)
	if err == nil {
		return verifiedResponse.GasInfo.GasUsed, nil
	}

	//
	// Execute the binary search and hone in on an executable gas limit
	lo := exactResponse.GasInfo.GasUsed
	hi := cap
	for lo+1 < hi {
		mid := (hi + lo) / 2
		_, err := executable(mid)

		// If the error is not nil(consensus error), it means the provided message
		// call or transaction will never be accepted no matter how much gas it is
		// assigned. Return the error directly, don't struggle any more.
		if err != nil {
			lo = mid
		} else {
			hi = mid
		}
	}

	return hi, nil
}

// EstimateGas returns an estimate of gas usage for the given smart contract call.
func (api *PublicEthereumAPI) EstimateGas(args rpctypes.CallArgs, blockNrOrHash rpctypes.BlockNumberOrHash) (hexutil.Uint64, error) {
	monitor := monitor.GetMonitor("eth_estimateGas", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("args", args)
	rateLimiter := api.GetRateLimiter("eth_estimateGas")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return 0, rpctypes.ErrServerBusy
	}
	params, err := api.getEvmParams()
	if err != nil {
		return 0, TransformDataError(err, "eth_estimateGas")
	}
	maxGasLimitPerTx := params.MaxGasLimitPerTx

	if args.GasPrice == nil || args.GasPrice.ToInt().Sign() <= 0 {
		// set the default value for possible check of GasPrice
		args.GasPrice = api.gasPrice
	}

	estimatedGas, err := api.simDoCall(args, maxGasLimitPerTx)
	if err != nil {
		return 0, TransformDataError(err, "eth_estimateGas")
	}

	if estimatedGas > maxGasLimitPerTx {
		errMsg := fmt.Sprintf("estimate gas %v greater than system max gas limit per tx %v", estimatedGas, maxGasLimitPerTx)
		return 0, TransformDataError(sdk.ErrOutOfGas(errMsg), "eth_estimateGas")
	}

	// The gasLimit of evm ordinary tx is 21000 by default.
	// Using gasBuffer will cause the gasLimit in MetaMask to be too large, which will affect the user experience.
	// Therefore, if an ordinary tx is received, just return the default gasLimit of evm.
	if estimatedGas == EvmDefaultGasLimit && args.Data == nil {
		return hexutil.Uint64(estimatedGas), nil
	}

	gasBuffer := estimatedGas / 100 * config.GetOecConfig().GetGasLimitBuffer()
	//EvmHookGasEstimate: evm tx with cosmos hook,we cannot estimate hook gas
	//simple add EvmHookGasEstimate,run tx will refund the extra gas
	gas := estimatedGas + gasBuffer + EvmHookGasEstimate
	if gas > maxGasLimitPerTx {
		gas = maxGasLimitPerTx
	}

	return hexutil.Uint64(gas), nil
}

// GetBlockByHash returns the block identified by hash.
func (api *PublicEthereumAPI) GetBlockByHash(hash common.Hash, fullTx bool) (*watcher.Block, error) {
	monitor := monitor.GetMonitor("eth_getBlockByHash", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("hash", hash, "full", fullTx)
	rateLimiter := api.GetRateLimiter("eth_getBlockByHash")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	blockRes, err := api.backend.GetBlockByHash(hash, fullTx)
	if err != nil {
		return nil, TransformDataError(err, RPCEthGetBlockByHash)
	}
	return blockRes, err
}

func (api *PublicEthereumAPI) getBlockByNumber(blockNum rpctypes.BlockNumber, fullTx bool) (blockRes *watcher.Block, err error) {
	if blockNum != rpctypes.PendingBlockNumber {
		blockRes, err = api.backend.GetBlockByNumber(blockNum, fullTx)
		return
	}

	height, err := api.backend.LatestBlockNumber()
	if err != nil {
		return nil, err
	}

	// latest block info
	latestBlock, err := api.backend.Block(&height)
	if err != nil {
		return nil, err
	}

	// number of pending txs queried from the mempool
	unconfirmedTxs, err := api.clientCtx.Client.UnconfirmedTxs(1000)
	if err != nil {
		return nil, err
	}

	gasUsed, ethTxs, err := rpctypes.EthTransactionsFromTendermint(api.clientCtx, unconfirmedTxs.Txs, common.BytesToHash(latestBlock.Block.Hash()), uint64(height))
	if err != nil {
		return nil, err
	}

	return rpctypes.FormatBlock(
		tmtypes.Header{
			Version:         latestBlock.Block.Version,
			ChainID:         api.clientCtx.ChainID,
			Height:          height + 1,
			Time:            time.Unix(0, 0),
			LastBlockID:     latestBlock.Block.LastBlockID,
			ValidatorsHash:  latestBlock.Block.NextValidatorsHash,
			ProposerAddress: latestBlock.Block.ProposerAddress,
		},
		0,
		latestBlock.Block.Hash(),
		0,
		gasUsed,
		ethTxs,
		ethtypes.Bloom{},
		fullTx,
	), nil
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicEthereumAPI) GetBlockByNumber(blockNum rpctypes.BlockNumber, fullTx bool) (*watcher.Block, error) {
	monitor := monitor.GetMonitor("eth_getBlockByNumber", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("number", blockNum, "full", fullTx)
	rateLimiter := api.GetRateLimiter("eth_getBlockByNumber")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	blockRes, err := api.getBlockByNumber(blockNum, fullTx)
	return blockRes, err
}

// GetTransactionByHash returns the transaction identified by hash.
func (api *PublicEthereumAPI) GetTransactionByHash(hash common.Hash) (*watcher.Transaction, error) {
	monitor := monitor.GetMonitor("eth_getTransactionByHash", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("hash", hash)
	tx, err := api.backend.GetTransactionByHash(hash)
	if err == nil {
		return tx, nil
	}
	// check if the tx is on the mempool
	pendingTx, pendingErr := api.PendingTransactionsByHash(hash)
	if pendingErr != nil {
		//to keep consistent with rpc of ethereum, should be return nil
		return nil, nil
	}
	return pendingTx, nil
}

// GetTransactionByBlockHashAndIndex returns the transaction identified by hash and index.
func (api *PublicEthereumAPI) GetTransactionByBlockHashAndIndex(hash common.Hash, idx hexutil.Uint) (*watcher.Transaction, error) {
	monitor := monitor.GetMonitor("eth_getTransactionByBlockHashAndIndex", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("hash", hash, "index", idx)
	res, _, err := api.clientCtx.Query(fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryHashToHeight, hash.Hex()))
	if err != nil {
		return nil, nil
	}

	var out evmtypes.QueryResBlockNumber
	api.clientCtx.Codec.MustUnmarshalJSON(res, &out)

	resBlock, err := api.backend.Block(&out.Number)
	if err != nil {
		return nil, nil
	}

	return api.getTransactionByBlockAndIndex(resBlock.Block, idx)
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (api *PublicEthereumAPI) GetTransactionByBlockNumberAndIndex(blockNum rpctypes.BlockNumber, idx hexutil.Uint) (*watcher.Transaction, error) {
	monitor := monitor.GetMonitor("eth_getTransactionByBlockNumberAndIndex", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("blockNum", blockNum, "index", idx)
	tx, e := api.wrappedBackend.GetTransactionByBlockNumberAndIndex(uint64(blockNum), uint(idx))
	if e == nil && tx != nil {
		return tx, nil
	}
	var (
		height int64
		err    error
	)

	switch blockNum {
	case rpctypes.PendingBlockNumber:
		// get all the EVM pending txs
		pendingTxs, err := api.backend.PendingTransactions()
		if err != nil {
			return nil, err
		}

		// return if index out of bounds
		if uint64(idx) >= uint64(len(pendingTxs)) {
			return nil, nil
		}

		// change back to pendingTxs[idx] once pending queue is fixed.
		return pendingTxs[int(idx)], nil

	case rpctypes.LatestBlockNumber:
		height, err = api.backend.LatestBlockNumber()
		if err != nil {
			return nil, err
		}

	default:
		height = blockNum.Int64()
	}

	resBlock, err := api.backend.Block(&height)
	if err != nil {
		return nil, err
	}

	return api.getTransactionByBlockAndIndex(resBlock.Block, idx)
}

func (api *PublicEthereumAPI) getTransactionByBlockAndIndex(block *tmtypes.Block, idx hexutil.Uint) (*watcher.Transaction, error) {
	// return if index out of bounds
	if uint64(idx) >= uint64(len(block.Txs)) {
		return nil, nil
	}

	ethTx, err := rpctypes.RawTxToEthTx(api.clientCtx, block.Txs[idx], block.Height)
	if err != nil {
		// return nil error if the transaction is not a MsgEthereumTx
		return nil, nil
	}

	height := uint64(block.Height)
	txHash := common.BytesToHash(ethTx.Hash)
	blockHash := common.BytesToHash(block.Hash())
	return watcher.NewTransaction(ethTx, txHash, blockHash, height, uint64(idx))
}

// GetTransactionReceipt returns the transaction receipt identified by hash.
func (api *PublicEthereumAPI) GetTransactionReceipt(hash common.Hash) (*watcher.TransactionReceipt, error) {
	monitor := monitor.GetMonitor("eth_getTransactionReceipt", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("hash", hash)
	rateLimiter := api.GetRateLimiter("eth_getTransactionReceipt")
	if rateLimiter != nil && !rateLimiter.Allow() {
		return nil, rpctypes.ErrServerBusy
	}
	res, e := api.wrappedBackend.GetTransactionReceipt(hash)
	if e == nil {
		return res, nil
	}

	tx, err := api.clientCtx.Client.Tx(hash.Bytes(), false)
	if err != nil {
		// Return nil for transaction when not found
		return nil, nil
	}

	// Query block for consensus hash
	block, err := api.backend.Block(&tx.Height)
	if err != nil {
		return nil, err
	}

	blockHash := common.BytesToHash(block.Block.Hash())

	// Convert tx bytes to eth transaction
	ethTx, err := rpctypes.RawTxToEthTx(api.clientCtx, tx.Tx, tx.Height)
	if err != nil {
		return nil, err
	}

	err = ethTx.VerifySig(api.chainIDEpoch, tx.Height)
	if err != nil {
		return nil, err
	}

	cumulativeGasUsed := uint64(tx.TxResult.GasUsed)
	if tx.Index != 0 {
		cumulativeGasUsed += rpctypes.GetBlockCumulativeGas(api.clientCtx.Codec, block.Block, int(tx.Index))
	}

	// Set status codes based on tx result
	var status hexutil.Uint64
	if tx.TxResult.IsOK() {
		status = hexutil.Uint64(1)
	} else {
		status = hexutil.Uint64(0)
	}

	txData := tx.TxResult.GetData()

	data, err := evmtypes.DecodeResultData(txData)
	if err != nil {
		status = 0 // transaction failed
	}

	if len(data.Logs) == 0 || status == 0 {
		data.Logs = []*ethtypes.Log{}
		data.Bloom = ethtypes.BytesToBloom(make([]byte, 256))
	}

	contractAddr := &data.ContractAddress
	if data.ContractAddress == common.HexToAddress("0x00000000000000000000") {
		contractAddr = nil
	}

	// fix gasUsed when deliverTx ante handler check sequence invalid
	gasUsed := tx.TxResult.GasUsed
	if tx.TxResult.Code == sdkerrors.ErrInvalidSequence.ABCICode() {
		gasUsed = 0
	}

	receipt := &watcher.TransactionReceipt{
		Status:            status,
		CumulativeGasUsed: hexutil.Uint64(cumulativeGasUsed),
		LogsBloom:         data.Bloom,
		Logs:              data.Logs,
		TransactionHash:   hash.String(),
		ContractAddress:   contractAddr,
		GasUsed:           hexutil.Uint64(gasUsed),
		BlockHash:         blockHash.String(),
		BlockNumber:       hexutil.Uint64(tx.Height),
		TransactionIndex:  hexutil.Uint64(tx.Index),
		From:              ethTx.GetFrom(),
		To:                ethTx.To(),
	}

	return receipt, nil
}

// PendingTransactions returns the transactions that are in the transaction pool
// and have a from address that is one of the accounts this node manages.
func (api *PublicEthereumAPI) PendingTransactions() ([]*watcher.Transaction, error) {
	api.logger.Debug("eth_pendingTransactions")
	return api.backend.PendingTransactions()
}

func (api *PublicEthereumAPI) PendingTransactionsByHash(target common.Hash) (*watcher.Transaction, error) {
	api.logger.Debug("eth_pendingTransactionsByHash")
	return api.backend.PendingTransactionsByHash(target)
}

// GetUncleByBlockHashAndIndex returns the uncle identified by hash and index. Always returns nil.
func (api *PublicEthereumAPI) GetUncleByBlockHashAndIndex(hash common.Hash, idx hexutil.Uint) map[string]interface{} {
	return nil
}

// GetUncleByBlockNumberAndIndex returns the uncle identified by number and index. Always returns nil.
func (api *PublicEthereumAPI) GetUncleByBlockNumberAndIndex(number hexutil.Uint, idx hexutil.Uint) map[string]interface{} {
	return nil
}

// GetProof returns an account object with proof and any storage proofs
func (api *PublicEthereumAPI) GetProof(address common.Address, storageKeys []string, blockNrOrHash rpctypes.BlockNumberOrHash) (*rpctypes.AccountResult, error) {
	monitor := monitor.GetMonitor("eth_getProof", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("address", address, "keys", storageKeys, "number", blockNrOrHash)
	blockNum, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if blockNum == rpctypes.LatestBlockNumber {
		n, err := api.BlockNumber()
		if err != nil {
			return nil, err
		}
		blockNum = rpctypes.BlockNumber(n)
	}

	clientCtx := api.clientCtx.WithHeight(int64(blockNum))
	path := fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryAccount, address.Hex())

	// query eth account at block height
	resBz, _, err := clientCtx.Query(path)
	if err != nil {
		return nil, err
	}
	var account *evmtypes.QueryResAccount
	clientCtx.Codec.MustUnmarshalJSON(resBz, &account)

	// query eth proof storage after MarsHeight
	if tmtypes.HigherThanMars(int64(blockNum)) {
		return api.getStorageProofInMpt(address, storageKeys, int64(blockNum), account)
	}

	/*
	 * query cosmos proof before MarsHeight
	 */
	storageProofs := make([]rpctypes.StorageResult, len(storageKeys))
	for i, k := range storageKeys {
		data := append(evmtypes.AddressStoragePrefix(address), getStorageByAddressKey(address, common.HexToHash(k).Bytes()).Bytes()...)
		// Get value for key
		req := abci.RequestQuery{
			Path:   fmt.Sprintf("store/%s/key", evmtypes.StoreKey),
			Data:   data,
			Height: int64(blockNum),
			Prove:  true,
		}

		vRes, err := clientCtx.QueryABCI(req)
		if err != nil {
			return nil, err
		}

		var value evmtypes.QueryResStorage
		value.Value = vRes.GetValue()

		// check for proof
		proof := vRes.GetProof()
		proofStr := new(merkle.Proof).String()
		if proof != nil {
			proofStr = proof.String()
		}

		storageProofs[i] = rpctypes.StorageResult{
			Key:   k,
			Value: (*hexutil.Big)(common.BytesToHash(value.Value).Big()),
			Proof: []string{proofStr},
		}
	}

	req := abci.RequestQuery{
		Path:   fmt.Sprintf("store/%s/key", auth.StoreKey),
		Data:   auth.AddressStoreKey(sdk.AccAddress(address.Bytes())),
		Height: int64(blockNum),
		Prove:  true,
	}

	res, err := clientCtx.QueryABCI(req)
	if err != nil {
		return nil, err
	}

	// check for proof
	accountProof := res.GetProof()
	accProofStr := new(merkle.Proof).String()
	if accountProof != nil {
		accProofStr = accountProof.String()
	}

	return &rpctypes.AccountResult{
		Address:      address,
		AccountProof: []string{accProofStr},
		Balance:      (*hexutil.Big)(utils.MustUnmarshalBigInt(account.Balance)),
		CodeHash:     common.BytesToHash(account.CodeHash),
		Nonce:        hexutil.Uint64(account.Nonce),
		StorageHash:  common.Hash{}, // Ethermint doesn't have a storage hash
		StorageProof: storageProofs,
	}, nil
}

func (api *PublicEthereumAPI) getStorageProofInMpt(address common.Address, storageKeys []string, blockNum int64, account *evmtypes.QueryResAccount) (*rpctypes.AccountResult, error) {
	clientCtx := api.clientCtx.WithHeight(blockNum)

	// query storage proof
	storageProofs := make([]rpctypes.StorageResult, len(storageKeys))
	for i, k := range storageKeys {
		queryStr := fmt.Sprintf("custom/%s/%s/%s/%X", evmtypes.ModuleName, evmtypes.QueryStorageProof, address.Hex(), common.HexToHash(k).Bytes())
		res, _, err := clientCtx.QueryWithData(queryStr, nil)
		if err != nil {
			return nil, err
		}

		var out evmtypes.QueryResStorageProof
		api.clientCtx.Codec.MustUnmarshalJSON(res, &out)

		storageProofs[i] = rpctypes.StorageResult{
			Key:   k,
			Value: (*hexutil.Big)(common.BytesToHash(out.Value).Big()),
			Proof: toHexSlice(out.Proof),
		}
	}

	// query account proof
	req := abci.RequestQuery{
		Path:   fmt.Sprintf("store/%s/key", mpt.StoreKey),
		Data:   auth.AddressStoreKey(sdk.AccAddress(address.Bytes())),
		Height: int64(blockNum),
		Prove:  true,
	}
	res, err := clientCtx.QueryABCI(req)
	if err != nil {
		return nil, err
	}
	var accProofList mpt.ProofList
	clientCtx.Codec.MustUnmarshalBinaryLengthPrefixed(res.GetProof().Ops[0].Data, &accProofList)

	// query account storage Hash
	queryStr := fmt.Sprintf("custom/%s/%s/%s", evmtypes.ModuleName, evmtypes.QueryStorageRoot, address.Hex())
	storageRootBytes, _, err := clientCtx.QueryWithData(queryStr, nil)
	if err != nil {
		return nil, err
	}

	// return result
	return &rpctypes.AccountResult{
		Address:      address,
		AccountProof: toHexSlice(accProofList),
		Balance:      (*hexutil.Big)(utils.MustUnmarshalBigInt(account.Balance)),
		CodeHash:     common.BytesToHash(account.CodeHash),
		Nonce:        hexutil.Uint64(account.Nonce),
		StorageHash:  common.BytesToHash(storageRootBytes),
		StorageProof: storageProofs,
	}, nil
}

// toHexSlice creates a slice of hex-strings based on []byte.
func toHexSlice(b [][]byte) []string {
	r := make([]string, len(b))
	for i := range b {
		r[i] = hexutil.Encode(b[i])
	}
	return r
}

// generateFromArgs populates tx message with args (used in RPC API)
func (api *PublicEthereumAPI) generateFromArgs(args rpctypes.SendTxArgs) (*evmtypes.MsgEthereumTx, error) {
	var (
		nonce, gasLimit uint64
		err             error
	)

	amount := (*big.Int)(args.Value)
	gasPrice := (*big.Int)(args.GasPrice)

	if args.GasPrice == nil {
		// Set default gas price
		// TODO: Change to min gas price from context once available through server/daemon
		gasPrice = api.gasPrice.ToInt()
	}

	if args.Nonce != nil && (uint64)(*args.Nonce) > 0 {
		nonce = (uint64)(*args.Nonce)
	} else {
		// get the nonce from the account retriever and the pending transactions
		nonce, err = api.accountNonce(api.clientCtx, *args.From, true, true)
		if err != nil {
			return nil, err
		}
	}

	if args.Data != nil && args.Input != nil && !bytes.Equal(*args.Data, *args.Input) {
		return nil, errors.New("both 'data' and 'input' are set and not equal. Please use 'input' to pass transaction call data")
	}

	// Sets input to either Input or Data, if both are set and not equal error above returns
	var input hexutil.Bytes
	if args.Input != nil {
		input = *args.Input
	} else if args.Data != nil {
		input = *args.Data
	}

	if args.To == nil && len(input) == 0 {
		// Contract creation
		return nil, fmt.Errorf("contract creation without any data provided")
	}

	if args.Gas == nil {
		callArgs := rpctypes.CallArgs{
			From:     args.From,
			To:       args.To,
			Gas:      args.Gas,
			GasPrice: args.GasPrice,
			Value:    args.Value,
			Data:     &input,
		}
		gl, err := api.EstimateGas(callArgs, rpctypes.BlockNumberOrHash{})
		if err != nil {
			return nil, err
		}
		gasLimit = uint64(gl)
	} else {
		gasLimit = (uint64)(*args.Gas)
	}
	msg := evmtypes.NewMsgEthereumTx(nonce, args.To, amount, gasLimit, gasPrice, input)

	return msg, nil
}

// pendingMsgs constructs an array of sdk.Msg. This method will check pending transactions and convert
// those transactions into ethermint messages.
func (api *PublicEthereumAPI) pendingMsgs() ([]sdk.Msg, error) {
	// nolint: prealloc
	var msgs []sdk.Msg

	pendingTxs, err := api.PendingTransactions()
	if err != nil {
		return nil, err
	}

	for _, pendingTx := range pendingTxs {
		// NOTE: we have to construct the EVM transaction instead of just casting from the tendermint
		// transactions because PendingTransactions only checks for MsgEthereumTx messages.

		pendingGas, err := hexutil.DecodeUint64(pendingTx.Gas.String())
		if err != nil {
			return nil, err
		}

		pendingValue := pendingTx.Value.ToInt()
		pendingGasPrice := new(big.Int).SetUint64(ethermint.DefaultGasPrice)
		if pendingTx.GasPrice != nil {
			pendingGasPrice = pendingTx.GasPrice.ToInt()
		}

		pendingData := pendingTx.Input
		nonce, _ := api.accountNonce(api.clientCtx, pendingTx.From, true, true)

		msg := evmtypes.NewMsgEthereumTx(nonce, pendingTx.To, pendingValue, pendingGas,
			pendingGasPrice, pendingData)

		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// accountNonce returns looks up the transaction nonce count for a given address. If the pending boolean
// is set to true, it will add to the counter all the uncommitted EVM transactions sent from the address.
// NOTE: The function returns no error if the account doesn't exist.
func (api *PublicEthereumAPI) accountNonce(
	clientCtx clientcontext.CLIContext, address common.Address, pending bool, useWatchBackend bool,
) (uint64, error) {
	if pending {
		// nonce is continuous in mempool txs
		pendingNonce, ok := api.backend.GetPendingNonce(address.String())
		if ok {
			return pendingNonce + 1, nil
		}
	}

	// Get nonce (sequence) of account from  watch db
	if useWatchBackend {
		acc, err := api.wrappedBackend.MustGetAccount(address.Bytes())
		if err == nil {
			return acc.GetSequence(), nil
		}
	}

	// Get nonce (sequence) of account from  chain db
	account, err := getAccountFromChain(clientCtx, address)
	if err != nil {
		if isAccountNotExistErr(err) {
			return 0, nil
		}
		return 0, err
	}
	if useWatchBackend {
		api.watcherBackend.CommitAccountToRpcDb(account)
	}
	return account.GetSequence(), nil
}

func getAccountFromChain(clientCtx clientcontext.CLIContext, address common.Address) (exported.Account, error) {
	accRet := authtypes.NewAccountRetriever(clientCtx)
	from := sdk.AccAddress(address.Bytes())
	return accRet.GetAccount(from)
}

func (api *PublicEthereumAPI) saveZeroAccount(address common.Address) {
	zeroAccount := ethermint.EthAccount{BaseAccount: &auth.BaseAccount{}}
	zeroAccount.SetAddress(address.Bytes())
	zeroAccount.SetBalance(sdk.DefaultBondDenom, sdk.ZeroDec())
	api.watcherBackend.CommitAccountToRpcDb(zeroAccount)
}

func (api *PublicEthereumAPI) FeeHistory(blockCount rpc.DecimalOrHex, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (*rpctypes.FeeHistoryResult, error) {
	api.logger.Debug("eth_feeHistory")
	return nil, fmt.Errorf("unsupported rpc function: eth_FeeHistory")
}

// FillTransaction fills the defaults (nonce, gas, gasPrice or 1559 fields)
// on a given unsigned transaction, and returns it to the caller for further
// processing (signing + broadcast).
func (api *PublicEthereumAPI) FillTransaction(args rpctypes.SendTxArgs) (*rpctypes.SignTransactionResult, error) {

	monitor := monitor.GetMonitor("eth_fillTransaction", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("args", args)

	height, err := api.BlockNumber()
	if err != nil {
		return nil, err
	}

	// Mutex lock the address' nonce to avoid assigning it to multiple requests
	if args.Nonce == nil {
		api.nonceLock.LockAddr(*args.From)
		defer api.nonceLock.UnlockAddr(*args.From)
	}

	// Assemble transaction from fields
	tx, err := api.generateFromArgs(args)
	if err != nil {
		api.logger.Debug("failed to generate tx", "error", err)
		return nil, err
	}

	if err := tx.ValidateBasic(); err != nil {
		api.logger.Debug("tx failed basic validation", "error", err)
		return nil, err
	}

	var txEncoder sdk.TxEncoder
	if tmtypes.HigherThanVenus(int64(height)) {
		txEncoder = authclient.GetTxEncoder(nil, authclient.WithEthereumTx())
	} else {
		txEncoder = authclient.GetTxEncoder(api.clientCtx.Codec)
	}

	// Encode transaction by RLP encoder
	txBytes, err := txEncoder(tx)
	if err != nil {
		return nil, err
	}
	rpcTx := rpctypes.ToTransaction(tx, args.From)
	return &rpctypes.SignTransactionResult{
		Raw: txBytes,
		Tx:  rpcTx,
	}, nil
}

func (api *PublicEthereumAPI) useWatchBackend(blockNum rpctypes.BlockNumber) bool {
	if !api.watcherBackend.Enabled() {
		return false
	}
	return blockNum == rpctypes.LatestBlockNumber || api.fastQueryThreshold <= 0 || global.GetGlobalHeight()-blockNum.Int64() <= int64(api.fastQueryThreshold)
}

func (api *PublicEthereumAPI) getEvmParams() (*evmtypes.Params, error) {
	if api.watcherBackend.Enabled() {
		params, err := api.wrappedBackend.GetParams()
		if err == nil {
			return params, nil
		}
	}

	paramsPath := fmt.Sprintf("custom/%s/%s", evmtypes.ModuleName, evmtypes.QueryParameters)
	res, _, err := api.clientCtx.QueryWithData(paramsPath, nil)
	var evmParams evmtypes.Params
	if err != nil {
		return nil, err
	}
	if err = api.clientCtx.Codec.UnmarshalJSON(res, &evmParams); err != nil {
		return nil, err
	}

	return &evmParams, nil
}

func (api *PublicEthereumAPI) JudgeEvm2CmTx(toAddr, payLoad []byte) bool {
	if !evm.IsMatchSystemContractFunction(payLoad) {
		return false
	}
	route := fmt.Sprintf("custom/%s/%s", evmtypes.ModuleName, evmtypes.QuerySysContractAddress)
	addr, _, err := api.clientCtx.QueryWithData(route, nil)
	if err == nil && len(addr) != 0 {
		return bytes.Equal(toAddr, addr)
	}
	return false
}
