package eth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/viper"

	ethermint "github.com/okex/exchain/app/types"
	clientCtx "github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/server"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerror "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	authexported "github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	"github.com/okex/exchain/libs/cosmos-sdk/x/supply"
	"github.com/okex/exchain/x/token"
	wasmkeeper "github.com/okex/exchain/x/wasm/keeper"
	wasmtypes "github.com/okex/exchain/x/wasm/types"
)

const (
	DefaultEVMErrorCode          = -32000
	VMExecuteException           = -32015
	VMExecuteExceptionInEstimate = 3
	AccountNotExistsCode         = 9

	RPCEthCall           = "eth_call"
	RPCEthEstimateGas    = "eth_estimateGas"
	RPCEthGetBlockByHash = "eth_getBlockByHash"

	RPCUnknowErr = "unknow"
	RPCNullData  = "null"
)

// gasPrice: to get "minimum-gas-prices" config or to get ethermint.DefaultGasPrice
func ParseGasPrice() *hexutil.Big {
	gasPrices, err := sdk.ParseDecCoins(viper.GetString(server.FlagMinGasPrices))
	if err == nil && gasPrices != nil && len(gasPrices) > 0 {
		return (*hexutil.Big)(gasPrices[0].Amount.BigInt())
	}
	//return the default gas price : DefaultGasPrice
	defaultGP := sdk.NewDecFromBigIntWithPrec(big.NewInt(ethermint.DefaultGasPrice), sdk.Precision/2+1).BigInt()
	return (*hexutil.Big)(defaultGP)
}

type cosmosError struct {
	Code      int    `json:"code"`
	Log       string `json:"log"`
	Codespace string `json:"codespace"`
}

func (c cosmosError) Error() string {
	return c.Log
}

func newCosmosError(code int, log, codeSpace string) cosmosError {
	return cosmosError{
		Code:      code,
		Log:       log,
		Codespace: codeSpace,
	}
}

func newWrappedCosmosError(code int, log, codeSpace string) cosmosError {
	e := newCosmosError(code, log, codeSpace)
	b, _ := json.Marshal(e)
	e.Log = string(b)
	return e
}

func parseCosmosError(err error) (*cosmosError, bool) {
	msg := err.Error()
	var realErr cosmosError
	if len(msg) == 0 {
		return nil, false
	}
	if err := json.Unmarshal([]byte(msg), &realErr); err != nil {
		return nil, false
	}
	return &realErr, true
}

type wrappedEthError struct {
	Wrap ethDataError `json:"0x00000000000000000000000000000000"`
}

type ethDataError struct {
	Error          string `json:"error"`
	ProgramCounter int    `json:"program_counter"`
	Reason         string `json:"reason"`
	Ret            string `json:"return"`
}

type DataError struct {
	code int         `json:"code"`
	Msg  string      `json:"msg"`
	data interface{} `json:"data,omitempty"`
}

func (d DataError) Error() string {
	return d.Msg
}

func (d DataError) ErrorData() interface{} {
	return d.data
}

func (d DataError) ErrorCode() int {
	return d.code
}

func newDataError(revert string, data string) *wrappedEthError {
	return &wrappedEthError{
		Wrap: ethDataError{
			Error:          "revert",
			ProgramCounter: 0,
			Reason:         revert,
			Ret:            data,
		}}
}

func TransformDataError(err error, method string) error {
	realErr, ok := parseCosmosError(err)
	if !ok {
		return DataError{
			code: DefaultEVMErrorCode,
			Msg:  err.Error(),
			data: RPCNullData,
		}
	}

	if method == RPCEthGetBlockByHash {
		return DataError{
			code: DefaultEVMErrorCode,
			Msg:  realErr.Error(),
			data: RPCNullData,
		}
	}
	return preProcessError(realErr, err.Error())
}

// realErr is a cosmosError, which is formatted from wrappedError. Msgs are concatenated with ':' between each of them
// Main cause always appears in the first place, thus this function only get the first part of the error out of realErr.
func preProcessError(realErr *cosmosError, origErrorMsg string) error {
	lastSeg := strings.IndexAny(realErr.Log, ":")
	if lastSeg < 0 {
		return DataError{
			code: DefaultEVMErrorCode,
			Msg:  origErrorMsg,
			data: RPCNullData,
		}
	}

	errorSeg := realErr.Log[0:lastSeg]

	return DataError{
		code: DefaultEVMErrorCode,
		Msg:  errorSeg,
		data: RPCNullData,
	}
}

func CheckError(txRes sdk.TxResponse) (common.Hash, error) {
	switch txRes.Code {
	case sdkerror.ErrTxInMempoolCache.ABCICode():
		return common.Hash{}, sdkerror.ErrTxInMempoolCache
	case sdkerror.ErrMempoolIsFull.ABCICode():
		return common.Hash{}, sdkerror.ErrMempoolIsFull
	case sdkerror.ErrTxTooLarge.ABCICode():
		return common.Hash{}, sdkerror.Wrapf(sdkerror.ErrTxTooLarge, txRes.RawLog)
	}
	return common.Hash{}, fmt.Errorf(txRes.RawLog)
}

func getStorageByAddressKey(addr common.Address, key []byte) common.Hash {
	prefix := addr.Bytes()
	compositeKey := make([]byte, len(prefix)+len(key))

	copy(compositeKey, prefix)
	copy(compositeKey[len(prefix):], key)

	return ethcrypto.Keccak256Hash(compositeKey)
}

func accountType(account authexported.Account, cliCtx clientCtx.CLIContext, wasmAddr sdk.WasmAddress) token.AccType {
	switch account.(type) {
	case *ethermint.EthAccount:
		ethAcc, _ := account.(*ethermint.EthAccount)
		if !bytes.Equal(ethAcc.CodeHash, ethcrypto.Keccak256(nil)) {
			return token.ContractAccount
		}
		// Determine whether it is a wasm contract
		route := fmt.Sprintf("custom/%s/%s/%s", wasmtypes.QuerierRoute, wasmkeeper.QueryGetContract, wasmAddr.String())
		_, _, err := cliCtx.Query(route)
		// Here, the address format must be valid, and only wasmtypes.ErrNotFound error may occur.
		if err == nil {
			return token.WasmAccount
		}
		return token.UserAccount
	case *supply.ModuleAccount:
		return token.ModuleAccount
	default:
		return token.OtherAccount
	}
}

func isAccountNotExistErr(err error) bool {
	cosmosErr, ok := parseCosmosError(err)
	if !ok {
		return false
	}
	return cosmosErr.Code == AccountNotExistsCode
}
