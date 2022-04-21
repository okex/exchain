// This is a test utility for Ethermint's Web3 JSON-RPC services.
//
// To run these tests please first ensure you have the ethermintd running
// and have started the RPC service with `ethermintcli rest-server`.
//
// You can configure the desired HOST and MODE as well
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	gorpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	"github.com/okex/exchain/app/rpc/backend"
	"github.com/okex/exchain/app/rpc/websockets"
	cosmos_context "github.com/okex/exchain/libs/cosmos-sdk/client/context"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/evm/watcher"
	"github.com/spf13/viper"
	"golang.org/x/net/websocket"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/okex/exchain/app/rpc"
	"github.com/okex/exchain/app/rpc/types"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	apptesting "github.com/okex/exchain/libs/ibc-go/testing"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/rpc/client/local"
	tmrpc "github.com/okex/exchain/libs/tendermint/rpc/test"
)

const (
	addrAStoreKey          = 0
	defaultProtocolVersion = 65
	defaultChainID         = 65
	defaultMinGasPrice     = "0.000000001okt"
	latestBlockNumber      = "latest"
	pendingBlockNumber     = "pending"
)

var (
	receiverAddr   = ethcmn.BytesToAddress([]byte("receiver"))
	inexistentAddr = ethcmn.BytesToAddress([]byte{0})
	inexistentHash = ethcmn.BytesToHash([]byte("inexistent hash"))
	MODE           = os.Getenv("MODE")
	from           = []byte{1}
)

type RPCTestSuite struct {
	suite.Suite

	coordinator *apptesting.Coordinator

	// testing chains used for convenience and readability
	chain apptesting.TestChainI

	apiServer *gorpc.Server
	Mux       *http.ServeMux
}

func (suite *RPCTestSuite) SetupTest() {
	chainId := apptesting.GetChainID(1)
	suite.coordinator = apptesting.NewCoordinator(suite.T(), 2)
	suite.chain = suite.coordinator.GetChain(chainId)

	ctx, logger := NewCLIContext(suite.chain.Codec())
	ctx = ctx.WithChainID("testchain-1")
	tmNode := tmrpc.NewTendermint(suite.chain.App(), &tmrpc.Options{})
	ctx.Client = local.New(tmNode)

	suite.apiServer = gorpc.NewServer()

	viper.Set(rpc.FlagDisableAPI, "")
	viper.Set(backend.FlagApiBackendBlockLruCache, 100)
	viper.Set(backend.FlagApiBackendTxLruCache, 100)
	viper.Set(watcher.FlagFastQueryLru, 100)

	apis := rpc.GetAPIs(ctx, logger, []ethsecp256k1.PrivKey{}...)
	for _, api := range apis {
		if err := suite.apiServer.RegisterName(api.Namespace, api.Service); err != nil {
			panic(err)
		}
	}
	suite.Mux = http.NewServeMux()
	suite.Mux.HandleFunc("/", suite.apiServer.ServeHTTP)
	go func() {
		http.ListenAndServe("0.0.0.0:8030", suite.Mux)
	}()
}

func NewCLIContext(cdc *codec.CodecProxy) (cosmos_context.CLIContext, log.Logger) {
	cliCtx := cosmos_context.NewCLIContext().WithProxy(cdc)
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "rest-server")

	cliCtx.TrustNode = true

	return cliCtx, logger
}
func TestRPCTestSuite(t *testing.T) {
	suite.Run(t, new(RPCTestSuite))
}

func (suite *RPCTestSuite) TestEth_GetBalance() {
	// initial balance of hexAddr2 is 1000000000okt in test.sh
	initialBalance, err := sdk.ParseDecCoin("1000000000okt")
	suite.Require().NoError(err)

	rpcRes, err := CallWithError("eth_getBalance", []interface{}{hexAddr2, latestBlockNumber})
	suite.Require().NoError(err)

	var balance hexutil.Big
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &balance))
	suite.Require().True(initialBalance.Amount.Int.Cmp(balance.ToInt()) == 0)

	// query on certain block height (2)
	rpcRes, err = CallWithError("eth_getBalance", []interface{}{hexAddr2, hexutil.EncodeUint64(2)})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &balance))
	suite.Require().NoError(err)
	suite.Require().True(initialBalance.Amount.Int.Cmp(balance.ToInt()) == 0)

	// query with pending -> no tx in mempool
	rpcRes, err = CallWithError("eth_getBalance", []interface{}{hexAddr2, pendingBlockNumber})
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &balance))
	suite.Require().True(initialBalance.Amount.Int.Cmp(balance.ToInt()) == 0)

	// inexistent addr -> zero balance
	rpcRes, err = CallWithError("eth_getBalance", []interface{}{inexistentAddr, latestBlockNumber})
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &balance))
	suite.Require().True(sdk.ZeroDec().Int.Cmp(balance.ToInt()) == 0)

	// error check
	// empty hex string
	_, err = CallWithError("eth_getBalance", []interface{}{hexAddr2, ""})
	suite.Require().Error(err)

	// missing argument
	_, err = CallWithError("eth_getBalance", []interface{}{hexAddr2})
	suite.Require().Error(err)
}
func (suite *RPCTestSuite) TestEth_Accounts() {
	// all unlocked addresses
	rpcRes, err := CallWithError("eth_accounts", nil)
	suite.Require().NoError(err)
	suite.Require().Equal(1, rpcRes.ID)

	var addrsUnlocked []ethcmn.Address
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &addrsUnlocked))
	//suite.Require().Equal(addrCounter, len(addrsUnlocked))
	//suite.Require().True(addrsUnlocked[0] == hexAddr1)
	//suite.Require().True(addrsUnlocked[1] == hexAddr2)
}

func (suite *RPCTestSuite) TestEth_ProtocolVersion() {
	rpcRes, err := CallWithError("eth_protocolVersion", nil)
	suite.Require().NoError(err)

	var version hexutil.Uint
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &version))
	suite.Require().Equal(version, hexutil.Uint(defaultProtocolVersion))
}

func (suite *RPCTestSuite) TestEth_ChainId() {
	rpcRes, err := CallWithError("eth_chainId", nil)
	suite.Require().NoError(err)

	var chainID hexutil.Uint
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &chainID))
	suite.Require().Equal(chainID, hexutil.Uint(defaultChainID))
}

func (suite *RPCTestSuite) TestEth_Syncing() {
	rpcRes, err := CallWithError("eth_syncing", nil)
	suite.Require().NoError(err)

	// single node for test.sh -> always leading without syncing
	var catchingUp bool
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &catchingUp))
	suite.Require().False(catchingUp)

	// TODO: set an evn in multi-nodes testnet to test the sycing status of a lagging node
}

func (suite *RPCTestSuite) TestEth_Coinbase() {
	// single node -> always the same addr for coinbase
	rpcRes, err := CallWithError("eth_coinbase", nil)
	suite.Require().NoError(err)

	var coinbaseAddr1 ethcmn.Address
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &coinbaseAddr1))

	// wait for 5s as an block interval
	time.Sleep(5 * time.Second)

	// query again
	rpcRes, err = CallWithError("eth_coinbase", nil)
	suite.Require().NoError(err)

	var coinbaseAddr2 ethcmn.Address
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &coinbaseAddr2))

	suite.Require().Equal(coinbaseAddr1, coinbaseAddr2)
}

func (suite *RPCTestSuite) TestEth_PowAttribute() {
	// eth_mining -> always false
	rpcRes, err := CallWithError("eth_mining", nil)
	suite.Require().NoError(err)

	var mining bool
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &mining))
	suite.Require().False(mining)

	// eth_hashrate -> always 0
	rpcRes, err = CallWithError("eth_hashrate", nil)
	suite.Require().NoError(err)

	var hashrate hexutil.Uint64
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hashrate))
	suite.Require().True(hashrate == 0)

	// eth_getUncleCountByBlockHash -> 0 for any hash
	rpcRes, err = CallWithError("eth_getUncleCountByBlockHash", []interface{}{inexistentHash})
	suite.Require().NoError(err)

	var uncleCount hexutil.Uint
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &uncleCount))
	suite.Require().True(uncleCount == 0)

	// eth_getUncleCountByBlockNumber -> 0 for any block number
	rpcRes, err = CallWithError("eth_getUncleCountByBlockNumber", []interface{}{latestBlockNumber})
	suite.Require().NoError(err)

	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &uncleCount))
	suite.Require().True(uncleCount == 0)

	// eth_getUncleByBlockHashAndIndex -> always "null"
	rand.Seed(time.Now().UnixNano())
	luckyNum := int64(rand.Int())
	randomBlockHash := ethcmn.BigToHash(big.NewInt(luckyNum))
	randomIndex := hexutil.Uint(luckyNum)
	rpcRes, err = CallWithError("eth_getUncleByBlockHashAndIndex", []interface{}{randomBlockHash, randomIndex})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// error check
	// miss argument
	_, err = CallWithError("eth_getUncleByBlockHashAndIndex", []interface{}{randomBlockHash})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getUncleByBlockHashAndIndex", nil)
	suite.Require().Error(err)

	// eth_getUncleByBlockNumberAndIndex -> always "null"
	luckyNum = int64(rand.Int())
	randomBlockHeight := hexutil.Uint(luckyNum)
	randomIndex = hexutil.Uint(luckyNum)
	rpcRes, err = CallWithError("eth_getUncleByBlockNumberAndIndex", []interface{}{randomBlockHeight, randomIndex})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// error check
	// miss argument
	_, err = CallWithError("eth_getUncleByBlockNumberAndIndex", []interface{}{randomBlockHeight})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getUncleByBlockNumberAndIndex", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GasPrice() {
	rpcRes, err := CallWithError("eth_gasPrice", nil)
	suite.Require().NoError(err)

	var gasPrice hexutil.Big
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &gasPrice))

	// min gas price in test.sh is "0.000000001okt"
	mgp, err := sdk.ParseDecCoin(defaultMinGasPrice)
	suite.Require().NoError(err)

	suite.Require().True(mgp.Amount.BigInt().Cmp(gasPrice.ToInt()) == 0)
}

func (suite *RPCTestSuite) TestEth_BlockNumber() {
	rpcRes := Call(suite.T(), "eth_blockNumber", nil)
	var blockNumber1 hexutil.Uint64
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &blockNumber1))

	// wait for 5s as an block interval
	time.Sleep(5 * time.Second)

	rpcRes = Call(suite.T(), "eth_blockNumber", nil)
	var blockNumber2 hexutil.Uint64
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &blockNumber2))

	suite.Require().True(blockNumber2 > blockNumber1)
}

func (suite *RPCTestSuite) TestEth_SendTransaction_Transfer() {
	value := sdk.NewDec(1024)
	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = hexAddr1.Hex()
	param[0]["to"] = receiverAddr.Hex()
	param[0]["value"] = (*hexutil.Big)(value.BigInt()).String()
	param[0]["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()

	rpcRes := Call(suite.T(), "eth_sendTransaction", param)

	var hash ethcmn.Hash
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	receipt := WaitForReceipt(suite.T(), hash)
	suite.Require().NotNil(receipt)
	suite.Require().Equal("0x1", receipt["status"].(string))
	suite.T().Logf("%s transfers %sokt to %s successfully\n", hexAddr1.Hex(), value.String(), receiverAddr.Hex())

	// TODO: logic bug, fix it later
	// ignore gas price -> default 'ethermint.DefaultGasPrice' on node -> successfully
	//delete(param[0], "gasPrice")
	//rpcRes = Call(suite.T(), "eth_sendTransaction", param)
	//
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	//receipt = WaitForReceipt(suite.T(), hash)
	//suite.Require().NotNil(receipt)
	//suite.Require().Equal("0x1", receipt["status"].(string))
	//suite.T().Logf("%s transfers %sokt to %s successfully with nil gas price \n", hexAddr1.Hex(), value.String(), receiverAddr.Hex())

	// error check
	// sender is not unlocked on the node
	param[0]["from"] = receiverAddr.Hex()
	param[0]["to"] = hexAddr1.Hex()
	rpcRes, err := CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// data.Data and data.Input are not same
	param[0]["from"], param[0]["to"] = param[0]["to"], param[0]["from"]
	param[0]["data"] = "0x1234567890abcdef"
	param[0]["input"] = param[0]["data"][:len(param[0]["data"])-2]
	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// input and toAddr are all empty
	delete(param[0], "to")
	delete(param[0], "input")
	delete(param[0], "data")

	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// 0 gas price
	param[0]["to"] = receiverAddr.Hex()
	param[0]["gasPrice"] = (*hexutil.Big)(sdk.ZeroDec().BigInt()).String()
	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_SendTransaction_ContractDeploy() {
	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = hexAddr1.Hex()
	param[0]["data"] = "0x6080604052348015600f57600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a2603580604b6000396000f3fe6080604052600080fdfea165627a7a723058206cab665f0f557620554bb45adf266708d2bd349b8a4314bdff205ee8440e3c240029"
	param[0]["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()
	rpcRes := Call(suite.T(), "eth_sendTransaction", param)

	var hash ethcmn.Hash
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	receipt := WaitForReceipt(suite.T(), hash)
	suite.Require().NotNil(receipt)
	suite.Require().Equal("0x1", receipt["status"].(string))
	suite.T().Logf("%s deploys contract (filled \"data\") successfully with tx hash %s\n", hexAddr1.Hex(), hash.String())

	// TODO: logic bug, fix it later
	// ignore gas price -> default 'ethermint.DefaultGasPrice' on node -> successfully
	//delete(param[0], "gasPrice")
	//rpcRes = Call(suite.T(), "eth_sendTransaction", param)
	//
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	//receipt = WaitForReceipt(suite.T(), hash)
	//suite.Require().NotNil(receipt)
	//suite.Require().Equal("0x1", receipt["status"].(string))
	//suite.T().Logf("%s deploys contract successfully with tx hash %s and nil gas price\n", hexAddr1.Hex(), hash.String())

	// same payload filled in both 'input' and 'data' -> ok
	param[0]["input"] = param[0]["data"]
	rpcRes = Call(suite.T(), "eth_sendTransaction", param)

	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	receipt = WaitForReceipt(suite.T(), hash)
	suite.Require().NotNil(receipt)
	suite.Require().Equal("0x1", receipt["status"].(string))
	suite.T().Logf("%s deploys contract (filled \"input\" and \"data\") successfully with tx hash %s\n", hexAddr1.Hex(), hash.String())

	// TODO: logic bug, fix it later
	// filled in 'input' -> ok
	//delete(param[0], "data")
	//rpcRes = Call(suite.T(), "eth_sendTransaction", param)
	//
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
	//receipt = WaitForReceipt(suite.T(), hash)
	//suite.Require().NotNil(receipt)
	//suite.Require().Equal("0x1", receipt["status"].(string))
	//suite.T().Logf("%s deploys contract (filled \"input\") successfully with tx hash %s\n", hexAddr1.Hex(), hash.String())

	// error check
	// sender is not unlocked on the node
	param[0]["from"] = receiverAddr.Hex()
	rpcRes, err := CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// data.Data and data.Input are not same
	param[0]["from"] = hexAddr1.Hex()
	param[0]["input"] = param[0]["data"][:len(param[0]["data"])-2]
	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// 0 gas price
	delete(param[0], "input")
	param[0]["gasPrice"] = (*hexutil.Big)(sdk.ZeroDec().BigInt()).String()
	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)

	// no payload of contract deployment
	delete(param[0], "data")

	rpcRes, err = CallWithError("eth_sendTransaction", param)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetStorageAt() {
	expectedRes := hexutil.Bytes{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	rpcRes := Call(suite.T(), "eth_getStorageAt", []string{hexAddr1.Hex(), fmt.Sprint(addrAStoreKey), latestBlockNumber})

	var storage hexutil.Bytes
	suite.Require().NoError(storage.UnmarshalJSON(rpcRes.Result))

	suite.T().Logf("Got value [%X] for %s with key %X\n", storage, hexAddr1.Hex(), addrAStoreKey)

	suite.Require().True(bytes.Equal(storage, expectedRes), "expected: %d (%d bytes) got: %d (%d bytes)", expectedRes, len(expectedRes), storage, len(storage))

	// error check
	// miss argument
	_, err := CallWithError("eth_getStorageAt", []string{hexAddr1.Hex(), fmt.Sprint(addrAStoreKey)})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getStorageAt", []string{hexAddr1.Hex()})
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetTransactionByHash() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	rpcRes := Call(suite.T(), "eth_getTransactionByHash", []interface{}{hash})

	var transaction watcher.Transaction
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transaction))
	suite.Require().True(hexAddr1 == transaction.From)
	suite.Require().True(receiverAddr == *transaction.To)
	suite.Require().True(hash == transaction.Hash)
	suite.Require().True(transaction.Value.ToInt().Cmp(big.NewInt(1024)) == 0)
	suite.Require().True(transaction.GasPrice.ToInt().Cmp(defaultGasPrice.Amount.BigInt()) == 0)
	// no input for a transfer tx
	suite.Require().Equal(0, len(transaction.Input))

	// hash not found -> rpcRes.Result -> "null"
	rpcRes, err := CallWithError("eth_getTransactionByHash", []interface{}{inexistentHash})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)
	suite.Require().Nil(rpcRes.Error)
}

func (suite *RPCTestSuite) TestEth_GetTransactionCount() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)
	height := getBlockHeightFromTxHash(suite.T(), hash)

	rpcRes := Call(suite.T(), "eth_getTransactionCount", []interface{}{hexAddr1, height.String()})

	var nonce, preNonce hexutil.Uint64
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &nonce))

	// query height - 1
	rpcRes = Call(suite.T(), "eth_getTransactionCount", []interface{}{hexAddr1, (height - 1).String()})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &preNonce))

	suite.Require().True(nonce-preNonce == 1)

	// latestBlock query
	rpcRes = Call(suite.T(), "eth_getTransactionCount", []interface{}{hexAddr1, latestBlockNumber})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &preNonce))
	suite.Require().Equal(nonce, preNonce)

	// pendingBlock query
	rpcRes = Call(suite.T(), "eth_getTransactionCount", []interface{}{hexAddr1, pendingBlockNumber})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &nonce))
	suite.Require().Equal(preNonce, nonce)

	// error check
	// miss argument
	_, err := CallWithError("eth_getTransactionCount", []interface{}{hexAddr1})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getTransactionCount", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetBlockTransactionCountByHash() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)
	blockHash := getBlockHashFromTxHash(suite.T(), hash)
	suite.Require().NotNil(blockHash)

	rpcRes := Call(suite.T(), "eth_getBlockTransactionCountByHash", []interface{}{*blockHash})

	var txCount hexutil.Uint
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &txCount))
	// only 1 tx on that height in this single node testnet
	suite.Require().True(txCount == 1)

	// inexistent hash -> return nil
	rpcRes = Call(suite.T(), "eth_getBlockTransactionCountByHash", []interface{}{inexistentHash})
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// error check
	// miss argument
	_, err := CallWithError("eth_getBlockTransactionCountByHash", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetBlockTransactionCountByNumber() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)
	height := getBlockHeightFromTxHash(suite.T(), hash)
	suite.Require().True(height != 0)

	rpcRes := Call(suite.T(), "eth_getBlockTransactionCountByNumber", []interface{}{height.String()})

	var txCount hexutil.Uint
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &txCount))
	// only 1 tx on that height in this single node testnet
	suite.Require().True(txCount == 1)

	// latestBlock query
	rpcRes = Call(suite.T(), "eth_getBlockTransactionCountByNumber", []interface{}{latestBlockNumber})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &txCount))
	// there is no tx on latest block
	suite.Require().True(txCount == 0)

	// pendingBlock query
	rpcRes = Call(suite.T(), "eth_getBlockTransactionCountByNumber", []interface{}{pendingBlockNumber})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &txCount))
	// there is no tx on latest block and mempool
	suite.Require().True(txCount == 0)

	// error check
	// miss argument
	_, err := CallWithError("eth_getBlockTransactionCountByNumber", nil)
	suite.Require().Error(err)
	fmt.Println(err)
}

func (suite *RPCTestSuite) TestEth_GetCode() {
	// TODO: logic bug, fix it later
	// erc20 contract
	//hash, receipet := deployTestContract(hexAddr1, erc20ContractKind)
	//height := getBlockHeightFromTxHash(hash)
	//suite.Require().True(height != 0)
	//
	//rpcRes := Call(suite.T(), "eth_getCode", []interface{}{receipet["contractAddress"], height.String()})
	//var code hexutil.Bytes
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &code))
	//suite.Require().True(strings.EqualFold(erc20ContractByteCode, code.String()))

	// test contract
	// TODO: logic bug, fix it later
	//hash, receipet := deployTestContract(hexAddr1, testContractKind)
	//height := getBlockHeightFromTxHash(hash)
	//suite.Require().True(height != 0)
	//
	//rpcRes := Call(suite.T(), "eth_getCode", []interface{}{receipet["contractAddress"], height.String()})
	//var code hexutil.Bytes
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &code))
	//fmt.Println(testContractByteCode)
	//fmt.Println(code.String())
	//suite.Require().True(strings.EqualFold(testContractByteCode, code.String()))

	// error check
	// miss argument
	// TODO: use a valid contract address as the first argument in params
	_, err := CallWithError("eth_getCode", []interface{}{hexAddr1})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getCode", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetTransactionLogs() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)

	rpcRes := Call(suite.T(), "eth_getTransactionLogs", []interface{}{hash})
	var transactionLogs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transactionLogs))
	// no transaction log for an evm transfer
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// test contract that emits an event in its constructor
	hash, receipt := deployTestContract(suite.T(), hexAddr1, testContractKind)

	rpcRes = Call(suite.T(), "eth_getTransactionLogs", []interface{}{hash})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transactionLogs))
	suite.Require().Equal(1, len(transactionLogs))
	suite.Require().True(ethcmn.HexToAddress(receipt["contractAddress"].(string)) == transactionLogs[0].Address)
	suite.Require().True(hash == transactionLogs[0].TxHash)
	// event in test contract constructor keeps the value: 1024
	suite.Require().True(transactionLogs[0].Topics[1].Big().Cmp(big.NewInt(1024)) == 0)

	// inexistent tx hash
	_, err := CallWithError("eth_getTransactionLogs", []interface{}{inexistentHash})
	suite.Require().Error(err)

	// error check
	// miss argument
	_, err = CallWithError("eth_getTransactionLogs", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_Sign() {
	data := []byte("context to sign")
	expectedSignature, err := signWithAccNameAndPasswd("alice", defaultPassWd, data)
	suite.Require().NoError(err)

	rpcRes := Call(suite.T(), "eth_sign", []interface{}{hexAddr1, hexutil.Bytes(data)})
	var sig hexutil.Bytes
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &sig))

	suite.Require().True(bytes.Equal(expectedSignature, sig))

	// error check
	// inexistent signer
	_, err = CallWithError("eth_sign", []interface{}{receiverAddr, hexutil.Bytes(data)})
	suite.Require().Error(err)

	// miss argument
	_, err = CallWithError("eth_sign", []interface{}{receiverAddr})
	suite.Require().Error(err)

	_, err = CallWithError("eth_sign", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_Call() {
	// simulate evm transfer
	callArgs := make(map[string]string)
	callArgs["from"] = hexAddr1.Hex()
	callArgs["to"] = receiverAddr.Hex()
	callArgs["value"] = hexutil.Uint(1024).String()
	callArgs["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()
	_, err := CallWithError("eth_call", []interface{}{callArgs, latestBlockNumber})
	suite.Require().NoError(err)

	// simulate contract deployment
	delete(callArgs, "to")
	delete(callArgs, "value")
	callArgs["data"] = erc20ContractDeployedByteCode
	_, err = CallWithError("eth_call", []interface{}{callArgs, latestBlockNumber})
	suite.Require().NoError(err)

	// error check
	// miss argument
	_, err = CallWithError("eth_call", []interface{}{callArgs})
	suite.Require().Error(err)

	_, err = CallWithError("eth_call", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_EstimateGas_WithoutArgs() {
	// error check
	// miss argument
	res, err := CallWithError("eth_estimateGas", nil)
	suite.Require().Error(err)
	suite.Require().Nil(res)
}

func (suite *RPCTestSuite) TestEth_EstimateGas_Transfer() {
	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = "0x" + fmt.Sprintf("%x", from)
	param[0]["to"] = "0x1122334455667788990011223344556677889900"
	param[0]["value"] = "0x1"
	param[0]["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()
	rpcRes := Call(suite.T(), "eth_estimateGas", param)
	suite.Require().NotNil(rpcRes)
	suite.Require().NotEmpty(rpcRes.Result)

	var gas string
	err := json.Unmarshal(rpcRes.Result, &gas)
	suite.Require().NoError(err, string(rpcRes.Result))

	suite.Require().Equal("0x100bb", gas)
}

func (suite *RPCTestSuite) TestEth_EstimateGas_ContractDeployment() {
	bytecode := "0x608060405234801561001057600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a260d08061004d6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063eb8ac92114602d575b600080fd5b606060048036036040811015604157600080fd5b8101908080359060200190929190803590602001909291905050506062565b005b8160008190555080827ff3ca124a697ba07e8c5e80bebcfcc48991fc16a63170e8a9206e30508960d00360405160405180910390a3505056fea265627a7a723158201d94d2187aaf3a6790527b615fcc40970febf0385fa6d72a2344848ebd0df3e964736f6c63430005110032"

	param := make([]map[string]string, 1)
	param[0] = make(map[string]string)
	param[0]["from"] = "0x" + fmt.Sprintf("%x", from)
	param[0]["data"] = bytecode

	rpcRes := Call(suite.T(), "eth_estimateGas", param)
	suite.Require().NotNil(rpcRes)
	suite.Require().NotEmpty(rpcRes.Result)

	var gas hexutil.Uint64
	err := json.Unmarshal(rpcRes.Result, &gas)
	suite.Require().NoError(err, string(rpcRes.Result))

	suite.Require().Equal("0x1b243", gas.String())
}

func (suite *RPCTestSuite) TestEth_GetBlockByHash() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)
	time.Sleep(3 * time.Second)
	expectedBlockHash := getBlockHashFromTxHash(suite.T(), hash)

	// TODO: OKExChain only supports the block query with txs' hash inside no matter what the second bool argument is.
	// 		eth rpc: 	false -> txs' hash inside
	//				  	true  -> txs full content

	// TODO: block hash bug , wait for pr merge
	//rpcRes := Call(suite.T(), "eth_getBlockByHash", []interface{}{expectedBlockHash, false})
	//var res map[string]interface{}
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &res))
	//suite.Require().True(strings.EqualFold(expectedBlockHash, res["hash"].(string)))
	//
	//rpcRes = Call(suite.T(), "eth_getBlockByHash", []interface{}{expectedBlockHash, true})
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &res))
	//suite.Require().True(strings.EqualFold(expectedBlockHash, res["hash"].(string)))

	// inexistent hash
	//rpcRes, err := CallWithError("eth_getBlockByHash", []interface{}{inexistentHash, false})

	// error check
	// miss argument
	_, err := CallWithError("eth_getBlockByHash", []interface{}{expectedBlockHash})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getBlockByHash", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetBlockByNumber() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)
	expectedHeight := getBlockHeightFromTxHash(suite.T(), hash)

	// TODO: OKExChain only supports the block query with txs' hash inside no matter what the second bool argument is.
	// 		eth rpc: 	false -> txs' hash inside
	rpcRes := Call(suite.T(), "eth_getBlockByNumber", []interface{}{expectedHeight, false})
	var res map[string]interface{}
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &res))
	suite.Require().True(strings.EqualFold(expectedHeight.String(), res["number"].(string)))

	rpcRes = Call(suite.T(), "eth_getBlockByNumber", []interface{}{expectedHeight, true})
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &res))
	suite.Require().True(strings.EqualFold(expectedHeight.String(), res["number"].(string)))

	// error check
	// future block height -> return nil without error
	rpcRes = Call(suite.T(), "eth_blockNumber", nil)
	var currentBlockHeight hexutil.Uint64
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &currentBlockHeight))

	rpcRes, err := CallWithError("eth_getBlockByNumber", []interface{}{currentBlockHeight + 100, false})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// miss argument
	_, err = CallWithError("eth_getBlockByNumber", []interface{}{currentBlockHeight})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getBlockByNumber", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetTransactionByBlockHashAndIndex() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(5 * time.Second)
	blockHash, index := getBlockHashFromTxHash(suite.T(), hash), hexutil.Uint(0)
	rpcRes := Call(suite.T(), "eth_getTransactionByBlockHashAndIndex", []interface{}{blockHash, index})
	var transaction watcher.Transaction
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transaction))
	suite.Require().True(hash == transaction.Hash)
	suite.Require().True(*blockHash == *transaction.BlockHash)
	suite.Require().True(hexutil.Uint64(index) == *transaction.TransactionIndex)

	// inexistent block hash
	// TODO: error:{"code":1,"log":"internal","height":1497,"codespace":"undefined"}, fix it later
	//rpcRes, err := CallWithError("eth_getTransactionByBlockHashAndIndex", []interface{}{inexistentHash, index})
	//fmt.Println(err)

	// inexistent transaction index -> nil
	rpcRes, err := CallWithError("eth_getTransactionByBlockHashAndIndex", []interface{}{blockHash, index + 100})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// error check
	// miss argument
	rpcRes, err = CallWithError("eth_getTransactionByBlockHashAndIndex", []interface{}{blockHash})
	suite.Require().Error(err)

	rpcRes, err = CallWithError("eth_getTransactionByBlockHashAndIndex", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetTransactionReceipt() {
	hash := sendTestTransaction(suite.T(), hexAddr1, receiverAddr, 1024)

	// sleep for a while
	time.Sleep(3 * time.Second)
	rpcRes := Call(suite.T(), "eth_getTransactionReceipt", []interface{}{hash})

	var receipt map[string]interface{}
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &receipt))
	suite.Require().True(strings.EqualFold(hexAddr1.Hex(), receipt["from"].(string)))
	suite.Require().True(strings.EqualFold(receiverAddr.Hex(), receipt["to"].(string)))
	suite.Require().True(strings.EqualFold(hexutil.Uint(1).String(), receipt["status"].(string)))
	suite.Require().True(strings.EqualFold(hash.Hex(), receipt["transactionHash"].(string)))

	// contract deployment
	hash, receipt = deployTestContract(suite.T(), hexAddr1, erc20ContractKind)
	suite.Require().True(strings.EqualFold(hexAddr1.Hex(), receipt["from"].(string)))
	suite.Require().True(strings.EqualFold(hexutil.Uint(1).String(), receipt["status"].(string)))
	suite.Require().True(strings.EqualFold(hash.Hex(), receipt["transactionHash"].(string)))

	// inexistent hash -> nil without error
	rpcRes, err := CallWithError("eth_getTransactionReceipt", []interface{}{inexistentHash})
	suite.Require().NoError(err)
	assertNullFromJSONResponse(suite.T(), rpcRes.Result)

	// error check
	// miss argument
	_, err = CallWithError("eth_getTransactionReceipt", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_PendingTransactions() {
	// there will be no pending tx in mempool because of the quick grab of block building
	rpcRes := Call(suite.T(), "eth_pendingTransactions", nil)
	var transactions []watcher.Transaction
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transactions))
	suite.Require().Zero(len(transactions))
}

func (suite *RPCTestSuite) TestBlockBloom() {
	hash, receipt := deployTestContract(suite.T(), hexAddr1, testContractKind)

	rpcRes := Call(suite.T(), "eth_getBlockByNumber", []interface{}{receipt["blockNumber"].(string), false})
	var blockInfo map[string]interface{}
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &blockInfo))
	logsBloom := hexToBloom(suite.T(), blockInfo["logsBloom"].(string))

	// get the transaction log with tx hash
	rpcRes = Call(suite.T(), "eth_getTransactionLogs", []interface{}{hash})
	var transactionLogs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &transactionLogs))
	suite.Require().Equal(1, len(transactionLogs))

	// all the topics in the transactionLogs should be included in the logs bloom of the block
	suite.Require().True(logsBloom.Test(transactionLogs[0].Topics[0].Bytes()))
	suite.Require().True(logsBloom.Test(transactionLogs[0].Topics[1].Bytes()))
	// check the consistency of tx hash
	suite.Require().True(strings.EqualFold(hash.Hex(), blockInfo["transactions"].([]interface{})[0].(string)))
}

func (suite *RPCTestSuite) TestEth_GetLogs_NoLogs() {
	param := make([]map[string][]string, 1)
	param[0] = make(map[string][]string)
	// inexistent topics
	inexistentTopicsHash := ethcmn.BytesToHash([]byte("inexistent topics")).Hex()
	param[0]["topics"] = []string{inexistentTopicsHash}
	rpcRes, err := CallWithError("eth_getLogs", param)
	suite.Require().NoError(err)

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &logs))
	suite.Require().Zero(len(logs))

	// error check
	_, err = CallWithError("eth_getLogs", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetLogs_GetTopicsFromHistory() {
	_, receipt := deployTestContract(suite.T(), hexAddr1, testContractKind)
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	param[0]["topics"] = []string{helloTopic, worldTopic}
	param[0]["fromBlock"] = receipt["blockNumber"].(string)

	time.Sleep(time.Second * 5)
	rpcRes := Call(suite.T(), "eth_getLogs", param)

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &logs))
	suite.Require().Equal(1, len(logs))
	suite.Require().Equal(2, len(logs[0].Topics))
	suite.Require().True(logs[0].Topics[0].Hex() == helloTopic)
	suite.Require().True(logs[0].Topics[1].Hex() == worldTopic)

	// get block number from receipt
	blockNumber, err := hexutil.DecodeUint64(receipt["blockNumber"].(string))
	suite.Require().NoError(err)

	// get current block height -> there is no logs from that height
	param[0]["fromBlock"] = hexutil.Uint64(blockNumber + 1).String()

	rpcRes, err = CallWithError("eth_getLogs", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &logs))
	suite.Require().Zero(len(logs))
}

func (suite *RPCTestSuite) TestEth_GetProof() {
	// initial balance of hexAddr2 is 1000000000okt in test.sh
	initialBalance, err := sdk.ParseDecCoin("1000000000okt")
	suite.Require().NoError(err)

	rpcRes := Call(suite.T(), "eth_getProof", []interface{}{hexAddr2, []string{fmt.Sprint(addrAStoreKey)}, "latest"})
	suite.Require().NotNil(rpcRes)

	var accRes types.AccountResult
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &accRes))
	suite.Require().True(accRes.Address == hexAddr2)
	suite.Require().True(initialBalance.Amount.Int.Cmp(accRes.Balance.ToInt()) == 0)
	suite.Require().NotEmpty(accRes.AccountProof)
	suite.Require().NotEmpty(accRes.StorageProof)

	// inexistentAddr -> zero value account result
	rpcRes, err = CallWithError("eth_getProof", []interface{}{inexistentAddr, []string{fmt.Sprint(addrAStoreKey)}, "latest"})
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &accRes))
	suite.Require().True(accRes.Address == inexistentAddr)
	suite.Require().True(sdk.ZeroDec().Int.Cmp(accRes.Balance.ToInt()) == 0)

	// error check
	// miss argument
	_, err = CallWithError("eth_getProof", []interface{}{hexAddr2, []string{fmt.Sprint(addrAStoreKey)}})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getProof", []interface{}{hexAddr2})
	suite.Require().Error(err)

	_, err = CallWithError("eth_getProof", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_NewFilter() {
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	// random topics
	param[0]["topics"] = []ethcmn.Hash{ethcmn.BytesToHash([]byte("random topics"))}
	rpcRes := Call(suite.T(), "eth_newFilter", param)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// fromBlock: latest, toBlock: latest -> no error
	delete(param[0], "topics")
	param[0]["fromBlock"] = latestBlockNumber
	param[0]["toBlock"] = latestBlockNumber
	rpcRes, err := CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// fromBlock: nil, toBlock: latest -> no error
	delete(param[0], "fromBlock")
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// fromBlock: latest, toBlock: nil -> no error
	delete(param[0], "toBlock")
	param[0]["fromBlock"] = latestBlockNumber
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// fromBlock: pending, toBlock: pending -> no error
	param[0]["fromBlock"] = pendingBlockNumber
	param[0]["toBlock"] = pendingBlockNumber
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// fromBlock: latest, toBlock: pending -> no error
	param[0]["fromBlock"] = latestBlockNumber
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// toBlock > fromBlock -> no error
	param[0]["fromBlock"] = (*hexutil.Big)(big.NewInt(2)).String()
	param[0]["toBlock"] = (*hexutil.Big)(big.NewInt(3)).String()
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().NoError(err)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// error check
	// miss argument
	_, err = CallWithError("eth_newFilter", nil)
	suite.Require().Error(err)

	// fromBlock > toBlock -> error: invalid from and to block combination: from > to
	param[0]["fromBlock"] = (*hexutil.Big)(big.NewInt(3)).String()
	param[0]["toBlock"] = (*hexutil.Big)(big.NewInt(2)).String()
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().Error(err)

	// fromBlock: pending, toBlock: latest
	param[0]["fromBlock"] = pendingBlockNumber
	param[0]["toBlock"] = latestBlockNumber
	rpcRes, err = CallWithError("eth_newFilter", param)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_NewBlockFilter() {
	rpcRes := Call(suite.T(), "eth_newBlockFilter", nil)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_BlockFilter() {
	rpcRes := Call(suite.T(), "eth_newBlockFilter", nil)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))

	// wait for block generation
	time.Sleep(5 * time.Second)

	changesRes := Call(suite.T(), "eth_getFilterChanges", []interface{}{ID})
	var hashes []ethcmn.Hash
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &hashes))
	suite.Require().GreaterOrEqual(len(hashes), 1)

	// error check
	// miss argument
	_, err := CallWithError("eth_getFilterChanges", nil)
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_NoLogs() {
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	param[0]["topics"] = []ethcmn.Hash{ethcmn.BytesToHash([]byte("random topics"))}

	rpcRes := Call(suite.T(), "eth_newFilter", param)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))

	changesRes := Call(suite.T(), "eth_getFilterChanges", []interface{}{ID})

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	// no logs
	suite.Require().Empty(logs)
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_WrongID() {
	// ID's length is 16
	inexistentID := "0x1234567890abcdef"
	_, err := CallWithError("eth_getFilterChanges", []interface{}{inexistentID})
	suite.Require().Error(err)
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_NoTopics() {
	// create a new filter with no topics and latest block height for "fromBlock"
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	param[0]["fromBlock"] = latestBlockNumber

	rpcRes := Call(suite.T(), "eth_newFilter", param)
	suite.Require().Nil(rpcRes.Error)
	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.T().Logf("create filter successfully with ID %s\n", ID)

	// deploy contract with emitting events
	_, _ = deployTestContract(suite.T(), hexAddr1, testContractKind)

	// get filter changes
	changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	suite.Require().Equal(1, len(logs))
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_Addresses() {
	// TODO: logic bug, fix it later
	//// deploy contract with emitting events
	//_, receipt := deployTestContract(hexAddr1, testContractKind)
	//contractAddrHex := receipt["contractAddress"].(string)
	//blockHeight := receipt["blockNumber"].(string)
	//// create a filter
	//param := make([]map[string]interface{}, 1)
	//param[0] = make(map[string]interface{})
	//// focus on the contract by its address
	//param[0]["addresses"] = []string{contractAddrHex}
	//param[0]["topics"] = []string{helloTopic, worldTopic}
	//param[0]["fromBlock"] = blockHeight
	//rpcRes := Call(suite.T(), "eth_newFilter", param)
	//
	//var ID string
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	//suite.T().Logf("create filter focusing on contract %s successfully with ID %s\n", contractAddrHex, ID)
	//
	//// get filter changes
	//changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})
	//
	//var logs []ethtypes.Log
	//suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	//suite.Require().Equal(1, len(logs))
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_BlockHash() {
	// TODO: logic bug, fix it later
	//// deploy contract with emitting events
	//_, receipt := deployTestContract(hexAddr1, testContractKind)
	//blockHash := receipt["blockHash"].(string)
	//contractAddrHex := receipt["contractAddress"].(string)
	//// create a filter
	//param := make([]map[string]interface{}, 1)
	//param[0] = make(map[string]interface{})
	//// focus on the contract by its address
	//param[0]["blockHash"] = blockHash
	//param[0]["addresses"] = []string{contractAddrHex}
	//param[0]["topics"] = []string{helloTopic, worldTopic}
	//rpcRes := Call(suite.T(), "eth_newFilter", param)
	//
	//var ID string
	//suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	//suite.T().Logf("create filter focusing on contract %s in the block with block hash %s successfully with ID %s\n", contractAddrHex, blockHash, ID)
	//
	//// get filter changes
	//changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})
	//
	//var logs []ethtypes.Log
	//suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	//suite.Require().Equal(1, len(logs))
}

// Tests topics case where there are topics in first two positions
func (suite *RPCTestSuite) TestEth_GetFilterChanges_Topics_AB() {
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	// set topics in filter with A && B
	param[0]["topics"] = []string{helloTopic, worldTopic}
	param[0]["fromBlock"] = latestBlockNumber

	// create new filter
	rpcRes := Call(suite.T(), "eth_newFilter", param)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.T().Logf("create filter successfully with ID %s\n", ID)

	// deploy contract with emitting events
	_, _ = deployTestContract(suite.T(), hexAddr1, testContractKind)

	// get filter changes
	changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	suite.Require().Equal(1, len(logs))
}

func (suite *RPCTestSuite) TestEth_GetFilterChanges_Topics_XB() {
	param := make([]map[string]interface{}, 1)
	param[0] = make(map[string]interface{})
	// set topics in filter with X && B
	param[0]["topics"] = []interface{}{nil, worldTopic}
	param[0]["fromBlock"] = latestBlockNumber

	// create new filter
	rpcRes := Call(suite.T(), "eth_newFilter", param)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.T().Logf("create filter successfully with ID %s\n", ID)

	// deploy contract with emitting events
	_, _ = deployTestContract(suite.T(), hexAddr1, testContractKind)

	// get filter changes
	changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})

	var logs []ethtypes.Log
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &logs))
	suite.Require().Equal(1, len(logs))
}

//func (suite *RPCTestSuite)TestEth_GetFilterChanges_Topics_XXC() {
//	t.Skip()
//	// TODO: call test function, need tx receipts to determine contract address
//}

func (suite *RPCTestSuite) TestEth_PendingTransactionFilter() {
	rpcRes := Call(suite.T(), "eth_newPendingTransactionFilter", nil)

	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))

	for i := 0; i < 5; i++ {
		_, _ = deployTestContract(suite.T(), hexAddr1, erc20ContractKind)
	}

	time.Sleep(10 * time.Second)

	// get filter changes
	changesRes := Call(suite.T(), "eth_getFilterChanges", []string{ID})
	suite.Require().NotNil(changesRes)

	var txs []hexutil.Bytes
	suite.Require().NoError(json.Unmarshal(changesRes.Result, &txs))

	suite.Require().True(len(txs) >= 2, "could not get any txs", "changesRes.Result", string(changesRes.Result))
}

func (suite *RPCTestSuite) TestEth_UninstallFilter() {
	// create a new filter, get id
	rpcRes := Call(suite.T(), "eth_newBlockFilter", nil)
	var ID string
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &ID))
	suite.Require().NotZero(ID)

	// based on id, uninstall filter
	rpcRes = Call(suite.T(), "eth_uninstallFilter", []string{ID})
	suite.Require().NotNil(rpcRes)
	var status bool
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &status))
	suite.Require().Equal(true, status)

	// uninstall a non-existent filter
	rpcRes = Call(suite.T(), "eth_uninstallFilter", []string{ID})
	suite.Require().NotNil(rpcRes)
	suite.Require().NoError(json.Unmarshal(rpcRes.Result, &status))
	suite.Require().Equal(false, status)

}

func (suite *RPCTestSuite) TestEth_Subscribe_And_UnSubscribe() {
	// create websocket
	origin, url := "http://127.0.0.1:8546/", "ws://127.0.0.1:8546"
	ws, err := websocket.Dial(url, "", origin)
	suite.Require().NoError(err)
	defer func() {
		// close websocket
		err = ws.Close()
		suite.Require().NoError(err)
	}()

	// send valid message
	validMessage := []byte(`{"id": 2, "method": "eth_subscribe", "params": ["newHeads"]}`)
	excuteValidMessage(suite.T(), ws, validMessage)

	// send invalid message
	invalidMessage := []byte(`{"id": 2, "method": "eth_subscribe", "params": ["non-existent method"]}`)
	excuteInvalidMessage(suite.T(), ws, invalidMessage)

	invalidMessage = []byte(`{"id": 2, "method": "eth_subscribe", "params": [""]}`)
	excuteInvalidMessage(suite.T(), ws, invalidMessage)
}

func excuteValidMessage(t *testing.T, ws *websocket.Conn, message []byte) {
	fmt.Println("Send:", string(message))
	_, err := ws.Write(message)
	require.NoError(t, err)

	msg := make([]byte, 10240)
	// receive subscription id
	n, err := ws.Read(msg)
	require.NoError(t, err)
	var res Response
	require.NoError(t, json.Unmarshal(msg[:n], &res))
	subscriptionId := string(res.Result)

	// receive message three times
	for i := 0; i < 3; i++ {
		n, err = ws.Read(msg)
		require.NoError(t, err)
		fmt.Println("Receive:", string(msg[:n]))
	}

	// cancel the subscription
	cancelMsg := fmt.Sprintf(`{"id": 2, "method": "eth_unsubscribe", "params": [%s]}`, subscriptionId)
	fmt.Println("Send:", cancelMsg)
	_, err = ws.Write([]byte(cancelMsg))
	require.NoError(t, err)

	// receive the result of eth_unsubscribe
	n, err = ws.Read(msg)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg[:n], &res))
	require.Equal(t, "true", string(res.Result))
}

func excuteInvalidMessage(t *testing.T, ws *websocket.Conn, message []byte) {
	fmt.Println("Send:", string(message))
	_, err := ws.Write(message)
	require.NoError(t, err)

	msg := make([]byte, 10240)
	// receive error msg
	n, err := ws.Read(msg)
	require.NoError(t, err)

	var res Response
	require.NoError(t, json.Unmarshal(msg[:n], &res))
	require.Equal(t, -32600, res.Error.Code)
	require.Equal(t, 1, res.ID)
}

func (suite *RPCTestSuite) TestWebsocket_PendingTransaction() {
	// create websocket
	origin, url := "http://127.0.0.1:8546/", "ws://127.0.0.1:8546"
	ws, err := websocket.Dial(url, "", origin)
	suite.Require().NoError(err)
	defer func() {
		// close websocket
		err = ws.Close()
		suite.Require().NoError(err)
	}()

	// send message to call newPendingTransactions ws api
	_, err = ws.Write([]byte(`{"id": 2, "method": "eth_subscribe", "params": ["newPendingTransactions"]}`))
	suite.Require().NoError(err)

	msg := make([]byte, 10240)
	// receive subscription id
	n, err := ws.Read(msg)
	suite.Require().NoError(err)
	var res Response
	suite.Require().NoError(json.Unmarshal(msg[:n], &res))
	subscriptionId := string(res.Result)

	// send transactions
	var expectedHashList [3]ethcmn.Hash
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			param := make([]map[string]string, 1)
			param[0] = make(map[string]string)
			param[0]["from"] = hexAddr1.Hex()
			param[0]["data"] = "0x6080604052348015600f57600080fd5b5060117f775a94827b8fd9b519d36cd827093c664f93347070a554f65e4a6f56cd73889860405160405180910390a2603580604b6000396000f3fe6080604052600080fdfea165627a7a723058206cab665f0f557620554bb45adf266708d2bd349b8a4314bdff205ee8440e3c240029"
			param[0]["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()
			rpcRes := Call(suite.T(), "eth_sendTransaction", param)

			var hash ethcmn.Hash
			suite.Require().NoError(json.Unmarshal(rpcRes.Result, &hash))
			expectedHashList[i] = hash
		}
	}()
	var actualHashList [3]ethcmn.Hash
	// receive message three times
	for i := 0; i < 3; i++ {
		n, err = ws.Read(msg)
		suite.Require().NoError(err)
		var notification websockets.SubscriptionNotification
		suite.Require().NoError(json.Unmarshal(msg[:n], &notification))
		actualHashList[i] = ethcmn.HexToHash(notification.Params.Result.(string))
	}
	wg.Wait()
	suite.Require().EqualValues(expectedHashList, actualHashList)

	// cancel the subscription
	cancelMsg := fmt.Sprintf(`{"id": 2, "method": "eth_unsubscribe", "params": [%s]}`, subscriptionId)
	_, err = ws.Write([]byte(cancelMsg))
	suite.Require().NoError(err)
}

//{} or nil          matches any topic list
//{A}                matches topic A in first position
//{{}, {B}}          matches any topic in first position AND B in second position
//{{A}, {B}}         matches topic A in first position AND B in second position
//{{A, B}, {C, D}}   matches topic (A OR B) in first position AND (C OR D) in second position
func (suite *RPCTestSuite) TestWebsocket_Logs() {
	t := suite.T()
	contractAddr1, contractAddr2, contractAddr3 := deployTestTokenContract(t), deployTestTokenContract(t), deployTestTokenContract(t)

	// init test cases
	tests := []struct {
		addressList string // input
		topicsList  string // input
		expected    int    // expected result
	}{
		// case 0: matches any contract address & any topics
		{"", "", 21},
		// case 1: matches one contract address & any topics
		{fmt.Sprintf(`"address":"%s"`, contractAddr1), "", 7},
		// case 2: matches two contract addressses & any topics
		{fmt.Sprintf(`"address":["%s","%s"]`, contractAddr1, contractAddr2), "", 14},
		// case 3: matches two contract addressses & one topic in first position
		{fmt.Sprintf(`"address":["%s","%s"]`, contractAddr1, contractAddr2), fmt.Sprintf(`"topics":["%s"]`, approveFuncHash), 6},
		// case 4: matches two contract addressses & one topic in third position
		{fmt.Sprintf(`"address":["%s","%s"]`, contractAddr1, contractAddr2), fmt.Sprintf(`"topics":[null, null, ["%s"]]`, recvAddr1Hash), 4},
		// case 5: matches two contract addressses & two topics in first、third position
		{fmt.Sprintf(`"address":["%s","%s"]`, contractAddr1, contractAddr2), fmt.Sprintf(`"topics":[["%s"], null, ["%s"]]`, approveFuncHash, recvAddr1Hash), 2},
		// case 6: matches two contract addressses & two topic lists in first、third position
		{fmt.Sprintf(`"address":["%s","%s"]`, contractAddr1, contractAddr2), fmt.Sprintf(`"topics":[["%s","%s"], null, ["%s","%s"]]`, approveFuncHash, transferFuncHash, recvAddr1Hash, recvAddr2Hash), 8},
	}

	go func() {
		time.Sleep(time.Minute * 2)
		panic("the tasks have been running for too long time, over 2 minutes")
	}()
	// the approximate running time is one minute
	var wg sync.WaitGroup
	wg.Add(len(tests) + 1)
	for i, test := range tests {
		go verifyWebSocketRecvNum(suite.T(), &wg, i, test.addressList, test.topicsList, test.expected)
	}
	go sendTxs(suite.T(), &wg, contractAddr1, contractAddr2, contractAddr3)
	wg.Wait()
}

func deployTestTokenContract(t *testing.T) string {
	param := make([]map[string]string, 1)
	param[0] = map[string]string{
		"from":     hexAddr1.Hex(),
		"data":     ttokenContractByteCode,
		"gasPrice": (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String(),
	}
	rpcRes := Call(t, "eth_sendTransaction", param)
	var hash ethcmn.Hash
	require.NoError(t, json.Unmarshal(rpcRes.Result, &hash))
	receipt := WaitForReceipt(t, hash)
	require.NotNil(t, receipt)
	contractAddr, ok := receipt["contractAddress"].(string)
	require.True(t, ok)
	return contractAddr
}

func verifyWebSocketRecvNum(t *testing.T, wg *sync.WaitGroup, index int, addressList, topicsList string, expected int) {
	defer wg.Done()

	// create websocket
	origin, url := "http://127.0.0.1:8546/", "ws://127.0.0.1:8546"
	ws, err := websocket.Dial(url, "", origin)
	require.NoError(t, err)
	defer func() {
		// close websocket
		err := ws.Close()
		require.NoError(t, err)
	}()

	// fulfill parameters
	param := assembleParameters(addressList, topicsList)
	_, err = ws.Write([]byte(param))
	require.NoError(t, err)

	msg := make([]byte, 10240)
	// receive subscription id
	n, err := ws.Read(msg)
	var res Response
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg[:n], &res))
	require.Nil(t, res.Error)
	subscriptionId := string(res.Result)
	//log.Printf("test case %d: websocket %s is created successfully, expect receive %d logs \n", index, subscriptionId, expected)

	for i := 0; i < expected; i++ {
		n, err = ws.Read(msg)
		require.NoError(t, err)
		var notification websockets.SubscriptionNotification
		require.NoError(t, json.Unmarshal(msg[:n], &notification))
	}

	// cancel the subscription
	cancelMsg := fmt.Sprintf(`{"id": 2, "method": "eth_unsubscribe", "params": [%s]}`, subscriptionId)
	_, err = ws.Write([]byte(cancelMsg))
	require.NoError(t, err)
	//log.Printf("test case %d: webdocket %s receive %d logs, then close successfully", index, subscriptionId, expected)
}

func assembleParameters(addressList string, topicsList string) string {
	var param string
	if addressList == "" {
		param = topicsList
	}
	if topicsList == "" {
		param = addressList
	}
	if addressList != "" && topicsList != "" {
		param = addressList + "," + topicsList
	}
	return fmt.Sprintf(`{"id": 2, "method": "eth_subscribe", "params": ["logs",{%s}]}`, param)
}

func sendTxs(t *testing.T, wg *sync.WaitGroup, contractAddrs ...string) {
	dataList := []string{
		// 0. mint  4294967295coin -> 0x2cf4ea7df75b513509d95946b43062e26bd88035
		"0x40c10f190000000000000000000000002cf4ea7df75b513509d95946b43062e26bd8803500000000000000000000000000000000000000000000000000000000ffffffff",
		// 1. approve 12345678coin -> 0x9ad84c8630e0282f78e5479b46e64e17779e3cfb
		"0x095ea7b30000000000000000000000009ad84c8630e0282f78e5479b46e64e17779e3cfb0000000000000000000000000000000000000000000000000000000000bc614e",
		// 2. approve 12345678coin -> 0xc9c9b43322f5e1dc401252076fa4e699c9122cd6
		"0x095ea7b3000000000000000000000000c9c9b43322f5e1dc401252076fa4e699c9122cd60000000000000000000000000000000000000000000000000000000000bc614e",
		// 3. approve 12345678coin -> 0x2B5Cf24AeBcE90f0B8f80Bc42603157b27cFbf47
		"0x095ea7b30000000000000000000000002b5cf24aebce90f0b8f80bc42603157b27cfbf470000000000000000000000000000000000000000000000000000000000bc614e",
		// 4. transfer 1234coin    -> 0x9ad84c8630e0282f78e5479b46e64e17779e3cfb
		"0xa9059cbb0000000000000000000000009ad84c8630e0282f78e5479b46e64e17779e3cfb00000000000000000000000000000000000000000000000000000000000004d2",
		// 5. transfer 1234coin    -> 0xc9c9b43322f5e1dc401252076fa4e699c9122cd6
		"0xa9059cbb000000000000000000000000c9c9b43322f5e1dc401252076fa4e699c9122cd600000000000000000000000000000000000000000000000000000000000004d2",
		// 6. transfer 1234coin    -> 0x2B5Cf24AeBcE90f0B8f80Bc42603157b27cFbf47
		"0xa9059cbb0000000000000000000000002b5cf24aebce90f0b8f80bc42603157b27cfbf4700000000000000000000000000000000000000000000000000000000000004d2",
	}
	defer wg.Done()
	for _, contractAddr := range contractAddrs {
		for i := 0; i < 7; i++ {
			param := make([]map[string]string, 1)
			param[0] = make(map[string]string)
			param[0]["from"] = hexAddr1.Hex()
			param[0]["to"] = contractAddr
			param[0]["data"] = dataList[i]
			param[0]["gasPrice"] = (*hexutil.Big)(defaultGasPrice.Amount.BigInt()).String()
			rpcRes := Call(t, "eth_sendTransaction", param)
			var hash ethcmn.Hash
			require.NoError(t, json.Unmarshal(rpcRes.Result, &hash))

			time.Sleep(time.Second * 1)
		}
	}
}
