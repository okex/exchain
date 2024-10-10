package app_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcmn "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/okex/exchain/app"
	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	apptypes "github.com/okex/exchain/app/types"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	"github.com/okex/exchain/libs/cosmos-sdk/simapp/helpers"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	authexported "github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	evmtypes "github.com/okex/exchain/x/evm/types"
	tokentypes "github.com/okex/exchain/x/token/types"
	wasmtypes "github.com/okex/exchain/x/wasm/types"
)

var (
	testPrecompileCodeA = "60806040526101006000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555034801561005257600080fd5b50610b76806100626000396000f3fe60806040526004361061004a5760003560e01c80635b3082c21461004f57806363de1b5d1461007f5780636bbb9b13146100af5780638381f58a146100df578063be2b0ac21461010a575b600080fd5b610069600480360381019061006491906106cc565b610147565b60405161007691906108ba565b60405180910390f35b61009960048036038101906100949190610670565b610161565b6040516100a69190610898565b60405180910390f35b6100c960048036038101906100c49190610744565b610314565b6040516100d69190610898565b60405180910390f35b3480156100eb57600080fd5b506100f46104ca565b6040516101019190610913565b60405180910390f35b34801561011657600080fd5b50610131600480360381019061012c91906105de565b6104d0565b60405161013e91906108ba565b60405180910390f35b606060405180602001604052806000815250905092915050565b60606001805461017191906109c7565b60018190555060008060008054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1634866040516024016101c391906108ba565b6040516020818303038152906040527fbe2b0ac2000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161024d9190610881565b60006040518083038185875af1925050503d806000811461028a576040519150601f19603f3d011682016040523d82523d6000602084013e61028f565b606091505b509150915083156102f557816102a457600080fd5b6000818060200190518101906102ba9190610627565b90507fe390e3d6b4766bc311796e6b5ce75dd6d51f0cb55cea58be963a5e7972ade65c816040516102eb91906108ba565b60405180910390a1505b6001805461030391906109c7565b600181905550809250505092915050565b60606001805461032491906109c7565b60018190555060008060008054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163487876040516024016103789291906108dc565b6040516020818303038152906040527f5b3082c2000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff83818316178352505050506040516104029190610881565b60006040518083038185875af1925050503d806000811461043f576040519150601f19603f3d011682016040523d82523d6000602084013e610444565b606091505b509150915083156104aa578161045957600080fd5b60008180602001905181019061046f9190610627565b90507fe390e3d6b4766bc311796e6b5ce75dd6d51f0cb55cea58be963a5e7972ade65c816040516104a091906108ba565b60405180910390a1505b600180546104b891906109c7565b60018190555080925050509392505050565b60015481565b6060604051806020016040528060008152509050919050565b60006104fc6104f784610953565b61092e565b90508281526020810184848401111561051857610517610b09565b5b610523848285610a33565b509392505050565b600061053e61053984610953565b61092e565b90508281526020810184848401111561055a57610559610b09565b5b610565848285610a42565b509392505050565b60008135905061057c81610b29565b92915050565b600082601f83011261059757610596610b04565b5b81356105a78482602086016104e9565b91505092915050565b600082601f8301126105c5576105c4610b04565b5b81516105d584826020860161052b565b91505092915050565b6000602082840312156105f4576105f3610b13565b5b600082013567ffffffffffffffff81111561061257610611610b0e565b5b61061e84828501610582565b91505092915050565b60006020828403121561063d5761063c610b13565b5b600082015167ffffffffffffffff81111561065b5761065a610b0e565b5b610667848285016105b0565b91505092915050565b6000806040838503121561068757610686610b13565b5b600083013567ffffffffffffffff8111156106a5576106a4610b0e565b5b6106b185828601610582565b92505060206106c28582860161056d565b9150509250929050565b600080604083850312156106e3576106e2610b13565b5b600083013567ffffffffffffffff81111561070157610700610b0e565b5b61070d85828601610582565b925050602083013567ffffffffffffffff81111561072e5761072d610b0e565b5b61073a85828601610582565b9150509250929050565b60008060006060848603121561075d5761075c610b13565b5b600084013567ffffffffffffffff81111561077b5761077a610b0e565b5b61078786828701610582565b935050602084013567ffffffffffffffff8111156107a8576107a7610b0e565b5b6107b486828701610582565b92505060406107c58682870161056d565b9150509250925092565b60006107da82610984565b6107e4818561099a565b93506107f4818560208601610a42565b6107fd81610b18565b840191505092915050565b600061081382610984565b61081d81856109ab565b935061082d818560208601610a42565b80840191505092915050565b60006108448261098f565b61084e81856109b6565b935061085e818560208601610a42565b61086781610b18565b840191505092915050565b61087b81610a29565b82525050565b600061088d8284610808565b915081905092915050565b600060208201905081810360008301526108b281846107cf565b905092915050565b600060208201905081810360008301526108d48184610839565b905092915050565b600060408201905081810360008301526108f68185610839565b9050818103602083015261090a8184610839565b90509392505050565b60006020820190506109286000830184610872565b92915050565b6000610938610949565b90506109448282610a75565b919050565b6000604051905090565b600067ffffffffffffffff82111561096e5761096d610ad5565b5b61097782610b18565b9050602081019050919050565b600081519050919050565b600081519050919050565b600082825260208201905092915050565b600081905092915050565b600082825260208201905092915050565b60006109d282610a29565b91506109dd83610a29565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff03821115610a1257610a11610aa6565b5b828201905092915050565b60008115159050919050565b6000819050919050565b82818337600083830152505050565b60005b83811015610a60578082015181840152602081019050610a45565b83811115610a6f576000848401525b50505050565b610a7e82610b18565b810181811067ffffffffffffffff82111715610a9d57610a9c610ad5565b5b80604052505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f8301169050919050565b610b3281610a1d565b8114610b3d57600080fd5b5056fea264697066735822122099b3fbd7a2bf1822c7f366e7e6685aa6801d09d9932acbf59c0687cae6df69da64736f6c63430008070033"

	contractJson = `{"abi":[{"inputs":[],"name":"add","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"retrieve","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"num","type":"uint256"}],"name":"store","outputs":[],"stateMutability":"nonpayable","type":"function"}],"bin":"608060405234801561001057600080fd5b50610205806100206000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80632e64cec1146100465780634f2be91f146100645780636057361d1461006e575b600080fd5b61004e61008a565b60405161005b91906100d1565b60405180910390f35b61006c610093565b005b6100886004803603810190610083919061011d565b6100ae565b005b60008054905090565b60016000808282546100a59190610179565b92505081905550565b8060008190555050565b6000819050919050565b6100cb816100b8565b82525050565b60006020820190506100e660008301846100c2565b92915050565b600080fd5b6100fa816100b8565b811461010557600080fd5b50565b600081359050610117816100f1565b92915050565b600060208284031215610133576101326100ec565b5b600061014184828501610108565b91505092915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b6000610184826100b8565b915061018f836100b8565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156101c4576101c361014a565b5b82820190509291505056fea2646970667358221220742b7232e733bee3592cb9e558bdae3fbd0006bcbdba76abc47b6020744037b364736f6c634300080a0033"}`

	testPrecompileABIAJson = "{\"abi\":[{\"inputs\":[{\"internalType\":\"string\",\"name\":\"wasmAddr\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"data\",\"type\":\"string\"}],\"name\":\"callToWasm\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"response\",\"type\":\"string\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"wasmAddr\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"msgData\",\"type\":\"string\"},{\"internalType\":\"bool\",\"name\":\"requireASuccess\",\"type\":\"bool\"}],\"name\":\"callWasm\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"response\",\"type\":\"bytes\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"string\",\"name\":\"data\",\"type\":\"string\"}],\"name\":\"pushLog\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"msgData\",\"type\":\"string\"},{\"internalType\":\"bool\",\"name\":\"requireASuccess\",\"type\":\"bool\"}],\"name\":\"queryWasm\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"response\",\"type\":\"bytes\"}],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"number\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"data\",\"type\":\"string\"}],\"name\":\"queryToWasm\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"response\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"}],\"bin\":\"60806040526101006000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555034801561005257600080fd5b50610b76806100626000396000f3fe60806040526004361061004a5760003560e01c80635b3082c21461004f57806363de1b5d1461007f5780636bbb9b13146100af5780638381f58a146100df578063be2b0ac21461010a575b600080fd5b610069600480360381019061006491906106cc565b610147565b60405161007691906108ba565b60405180910390f35b61009960048036038101906100949190610670565b610161565b6040516100a69190610898565b60405180910390f35b6100c960048036038101906100c49190610744565b610314565b6040516100d69190610898565b60405180910390f35b3480156100eb57600080fd5b506100f46104ca565b6040516101019190610913565b60405180910390f35b34801561011657600080fd5b50610131600480360381019061012c91906105de565b6104d0565b60405161013e91906108ba565b60405180910390f35b606060405180602001604052806000815250905092915050565b60606001805461017191906109c7565b60018190555060008060008054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1634866040516024016101c391906108ba565b6040516020818303038152906040527fbe2b0ac2000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161024d9190610881565b60006040518083038185875af1925050503d806000811461028a576040519150601f19603f3d011682016040523d82523d6000602084013e61028f565b606091505b509150915083156102f557816102a457600080fd5b6000818060200190518101906102ba9190610627565b90507fe390e3d6b4766bc311796e6b5ce75dd6d51f0cb55cea58be963a5e7972ade65c816040516102eb91906108ba565b60405180910390a1505b6001805461030391906109c7565b600181905550809250505092915050565b60606001805461032491906109c7565b60018190555060008060008054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163487876040516024016103789291906108dc565b6040516020818303038152906040527f5b3082c2000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff83818316178352505050506040516104029190610881565b60006040518083038185875af1925050503d806000811461043f576040519150601f19603f3d011682016040523d82523d6000602084013e610444565b606091505b509150915083156104aa578161045957600080fd5b60008180602001905181019061046f9190610627565b90507fe390e3d6b4766bc311796e6b5ce75dd6d51f0cb55cea58be963a5e7972ade65c816040516104a091906108ba565b60405180910390a1505b600180546104b891906109c7565b60018190555080925050509392505050565b60015481565b6060604051806020016040528060008152509050919050565b60006104fc6104f784610953565b61092e565b90508281526020810184848401111561051857610517610b09565b5b610523848285610a33565b509392505050565b600061053e61053984610953565b61092e565b90508281526020810184848401111561055a57610559610b09565b5b610565848285610a42565b509392505050565b60008135905061057c81610b29565b92915050565b600082601f83011261059757610596610b04565b5b81356105a78482602086016104e9565b91505092915050565b600082601f8301126105c5576105c4610b04565b5b81516105d584826020860161052b565b91505092915050565b6000602082840312156105f4576105f3610b13565b5b600082013567ffffffffffffffff81111561061257610611610b0e565b5b61061e84828501610582565b91505092915050565b60006020828403121561063d5761063c610b13565b5b600082015167ffffffffffffffff81111561065b5761065a610b0e565b5b610667848285016105b0565b91505092915050565b6000806040838503121561068757610686610b13565b5b600083013567ffffffffffffffff8111156106a5576106a4610b0e565b5b6106b185828601610582565b92505060206106c28582860161056d565b9150509250929050565b600080604083850312156106e3576106e2610b13565b5b600083013567ffffffffffffffff81111561070157610700610b0e565b5b61070d85828601610582565b925050602083013567ffffffffffffffff81111561072e5761072d610b0e565b5b61073a85828601610582565b9150509250929050565b60008060006060848603121561075d5761075c610b13565b5b600084013567ffffffffffffffff81111561077b5761077a610b0e565b5b61078786828701610582565b935050602084013567ffffffffffffffff8111156107a8576107a7610b0e565b5b6107b486828701610582565b92505060406107c58682870161056d565b9150509250925092565b60006107da82610984565b6107e4818561099a565b93506107f4818560208601610a42565b6107fd81610b18565b840191505092915050565b600061081382610984565b61081d81856109ab565b935061082d818560208601610a42565b80840191505092915050565b60006108448261098f565b61084e81856109b6565b935061085e818560208601610a42565b61086781610b18565b840191505092915050565b61087b81610a29565b82525050565b600061088d8284610808565b915081905092915050565b600060208201905081810360008301526108b281846107cf565b905092915050565b600060208201905081810360008301526108d48184610839565b905092915050565b600060408201905081810360008301526108f68185610839565b9050818103602083015261090a8184610839565b90509392505050565b60006020820190506109286000830184610872565b92915050565b6000610938610949565b90506109448282610a75565b919050565b6000604051905090565b600067ffffffffffffffff82111561096e5761096d610ad5565b5b61097782610b18565b9050602081019050919050565b600081519050919050565b600081519050919050565b600082825260208201905092915050565b600081905092915050565b600082825260208201905092915050565b60006109d282610a29565b91506109dd83610a29565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff03821115610a1257610a11610aa6565b5b828201905092915050565b60008115159050919050565b6000819050919050565b82818337600083830152505050565b60005b83811015610a60578082015181840152602081019050610a45565b83811115610a6f576000848401525b50505050565b610a7e82610b18565b810181811067ffffffffffffffff82111715610a9d57610a9c610ad5565b5b80604052505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f8301169050919050565b610b3281610a1d565b8114610b3d57600080fd5b5056fea264697066735822122099b3fbd7a2bf1822c7f366e7e6685aa6801d09d9932acbf59c0687cae6df69da64736f6c63430008070033\"}"
	callWasmMsgFormat      = "{\"transfer\":{\"amount\":\"%d\",\"recipient\":\"%s\"}}"
)

type Env struct {
	priv []ethsecp256k1.PrivKey
	addr []sdk.AccAddress
}

type Chain struct {
	app          *app.OKExChainApp
	codec        *codec.Codec
	priv         []ethsecp256k1.PrivKey
	addr         []sdk.AccAddress
	acc          []apptypes.EthAccount
	seq          []uint64
	num          []uint64
	chainIdStr   string
	chainIdInt   *big.Int
	ContractAddr []byte

	erc20ABI abi.ABI
	//vmb: evm->wasm
	VMBContractA    ethcmn.Address
	VMBWasmContract sdk.WasmAddress
	//vmb: wasm->evm
	freeCallWasmContract sdk.WasmAddress
	freeCallWasmCodeId   uint64
	freeCallEvmContract  ethcmn.Address

	timeYear int
}

func NewChain(env *Env) *Chain {
	chain := new(Chain)
	chain.acc = make([]apptypes.EthAccount, 10)
	chain.priv = make([]ethsecp256k1.PrivKey, 10)
	chain.addr = make([]sdk.AccAddress, 10)
	chain.seq = make([]uint64, 10)
	chain.num = make([]uint64, 10)
	chain.chainIdStr = "ethermint-3"
	chain.chainIdInt = big.NewInt(3)
	chain.timeYear = 2022
	// initialize account
	genAccs := make([]authexported.GenesisAccount, 0)
	for i := 0; i < 10; i++ {
		chain.acc[i] = apptypes.EthAccount{
			BaseAccount: &auth.BaseAccount{
				Address: env.addr[i],
				Coins:   sdk.Coins{sdk.NewInt64Coin("okt", 1000000)},
			},
			CodeHash: ethcrypto.Keccak256(nil),
		}
		genAccs = append(genAccs, chain.acc[i])
		chain.priv[i] = env.priv[i]
		chain.addr[i] = env.addr[i]
		chain.seq[i] = 0
		chain.num[i] = uint64(i)
	}

	chain.app = app.SetupWithGenesisAccounts(false, genAccs, app.WithChainId(chain.chainIdStr))
	chain.codec = chain.app.Codec()

	chain.app.WasmKeeper.SetParams(chain.Ctx(), wasmtypes.TestParams())
	params := evmtypes.DefaultParams()
	params.EnableCreate = true
	params.EnableCall = true
	chain.app.EvmKeeper.SetParams(chain.Ctx(), params)

	chain.app.BaseApp.Commit(abci.RequestCommit{})
	return chain
}

func (chain *Chain) Ctx() sdk.Context {
	return chain.app.BaseApp.GetDeliverStateCtx()
}

func DeployContractAndGetContractAddress(t *testing.T, chain *Chain) {
	var rawTxs [][]byte
	rawTxs = append(rawTxs, deployContract(t, chain, 0))
	r := runTxs(chain, rawTxs, false)

	log := r[0].Log[1 : len(r[0].Log)-1]
	logMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(log), &logMap)
	require.NoError(t, err)

	logs := strings.Split(logMap["log"].(string), ";")
	require.True(t, len(logs) == 3)
	contractLog := strings.Split(logs[2], " ")
	require.True(t, len(contractLog) == 4)
	chain.ContractAddr = []byte(contractLog[3])
}

func createEthTx(t *testing.T, chain *Chain, addressIdx int) []byte {
	amount, gasPrice, gasLimit := int64(1024), int64(2048), uint64(100000)
	addrTo := ethcmn.BytesToAddress(chain.priv[addressIdx+1].PubKey().Address().Bytes())
	msg := evmtypes.NewMsgEthereumTx(chain.seq[addressIdx], &addrTo, big.NewInt(amount), gasLimit, big.NewInt(gasPrice), []byte{})
	chain.seq[addressIdx]++
	err := msg.Sign(chain.chainIdInt, chain.priv[addressIdx].ToECDSA())
	require.NoError(t, err)
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)

	return rawTx
}

func createAnteErrEthTx(t *testing.T, chain *Chain, addressIdx int) []byte {
	amount, gasPrice, gasLimit := int64(1024), int64(2048), uint64(100000)
	addrTo := ethcmn.BytesToAddress(chain.priv[addressIdx+1].PubKey().Address().Bytes())
	//Note: anteErr occur (invalid nonce)
	msg := evmtypes.NewMsgEthereumTx(chain.seq[addressIdx]+1, &addrTo, big.NewInt(amount), gasLimit, big.NewInt(gasPrice), []byte{})
	err := msg.Sign(chain.chainIdInt, chain.priv[addressIdx].ToECDSA())
	require.NoError(t, err)
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)

	return rawTx
}

func createFailedEthTx(t *testing.T, chain *Chain, addressIdx int) []byte {
	amount, gasPrice, gasLimit := int64(1024), int64(2048), uint64(1)
	addrTo := ethcmn.BytesToAddress(chain.priv[addressIdx+1].PubKey().Address().Bytes())
	msg := evmtypes.NewMsgEthereumTx(chain.seq[addressIdx], &addrTo, big.NewInt(amount), gasLimit, big.NewInt(gasPrice), []byte{})
	chain.seq[addressIdx]++
	err := msg.Sign(chain.chainIdInt, chain.priv[addressIdx].ToECDSA())
	require.NoError(t, err)
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)

	return rawTx
}

func createTokenSendTx(t *testing.T, chain *Chain, i int) []byte {
	msg := tokentypes.NewMsgTokenSend(chain.addr[i], chain.addr[i+1], sdk.Coins{sdk.NewInt64Coin("okt", 1)})

	tx := helpers.GenTx(
		[]sdk.Msg{msg},
		sdk.Coins{sdk.NewInt64Coin(sdk.DefaultBondDenom, 1)},
		helpers.DefaultGenTxGas,
		chain.chainIdStr,
		[]uint64{chain.num[i]},
		[]uint64{chain.seq[i]},
		chain.priv[i],
	)
	chain.seq[i]++

	txBytes, err := chain.app.Codec().MarshalBinaryLengthPrefixed(tx)
	require.Nil(t, err)
	return txBytes
}

func createFailedTokenSendTx(t *testing.T, chain *Chain, i int) []byte {
	msg := tokentypes.NewMsgTokenSend(chain.addr[i], chain.addr[i+1], sdk.Coins{sdk.NewInt64Coin("okt", 100000000000)})

	tx := helpers.GenTx(
		[]sdk.Msg{msg},
		sdk.Coins{sdk.NewInt64Coin(sdk.DefaultBondDenom, 1)},
		helpers.DefaultGenTxGas,
		chain.chainIdStr,
		[]uint64{chain.num[i]},
		[]uint64{chain.seq[i]},
		chain.priv[i],
	)
	chain.seq[i]++

	txBytes, err := chain.app.Codec().MarshalBinaryLengthPrefixed(tx)
	require.Nil(t, err)
	return txBytes
}

func createAnteErrTokenSendTx(t *testing.T, chain *Chain, i int) []byte {
	msg := tokentypes.NewMsgTokenSend(chain.addr[i], chain.addr[i+1], sdk.Coins{sdk.NewInt64Coin("okt", 1)})

	tx := helpers.GenTx(
		[]sdk.Msg{msg},
		sdk.Coins{sdk.NewInt64Coin(sdk.DefaultBondDenom, 1000000000000)},
		helpers.DefaultGenTxGas,
		chain.chainIdStr,
		[]uint64{chain.num[i]},
		[]uint64{chain.seq[i]},
		chain.priv[i],
	)

	txBytes, err := chain.app.Codec().MarshalBinaryLengthPrefixed(tx)
	require.Nil(t, err)
	return txBytes
}

func runTxs(chain *Chain, rawTxs [][]byte, isParallel bool) []*abci.ResponseDeliverTx {
	timeValue := fmt.Sprintf("%d-04-11 13:33:37", chain.timeYear+1)
	testTime, _ := time.Parse("2006-01-02 15:04:05", timeValue)
	header := abci.Header{Height: chain.app.LastBlockHeight() + 1, ChainID: chain.chainIdStr, Time: testTime}
	chain.app.BaseApp.BeginBlock(abci.RequestBeginBlock{Header: header})
	var ret []*abci.ResponseDeliverTx
	if isParallel {
		ret = chain.app.BaseApp.ParallelTxs(rawTxs, false)
	} else {
		for _, tx := range rawTxs {
			r := chain.app.BaseApp.DeliverTx(abci.RequestDeliverTx{Tx: tx})
			ret = append(ret, &r)
		}
	}
	chain.app.BaseApp.EndBlock(abci.RequestEndBlock{})
	chain.app.BaseApp.Commit(abci.RequestCommit{})

	return ret
}

func TestParallelTxs(t *testing.T) {

	tmtypes.UnittestOnlySetMilestoneVenusHeight(-1)
	tmtypes.UnittestOnlySetMilestoneVenus1Height(1)
	tmtypes.UnittestOnlySetMilestoneVenus2Height(1)
	tmtypes.UnittestOnlySetMilestoneEarthHeight(1)
	tmtypes.UnittestOnlySetMilestoneVenus6Height(1)

	env := new(Env)
	env.priv = make([]ethsecp256k1.PrivKey, 10)
	env.addr = make([]sdk.AccAddress, 10)
	for i := 0; i < 10; i++ {
		priv, _ := ethsecp256k1.GenerateKey()
		addr := sdk.AccAddress(priv.PubKey().Address())
		env.priv[i] = priv
		env.addr[i] = addr
	}
	chainA, chainB := NewChain(env), NewChain(env)

	VMBPrecompileSetup(t, chainA)
	VMBPrecompileSetup(t, chainB)

	DeployContractAndGetContractAddress(t, chainA)
	DeployContractAndGetContractAddress(t, chainB)

	testCases := []struct {
		title         string
		executeTxs    func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte)
		expectedCodes []uint32
	}{
		// #####################
		// ### only evm txs ####
		// #####################
		{
			"5 evm txs, 1 group: a->b b->c c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				for i := 0; i < 5; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"4 evm txs and 1 AnteErr evm tx, 1 group: a->b anteErr(a->b) b->c c->d d->e",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				for i := 2; i < 5; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 0, 0, 0},
		},
		{
			"4 evm txs and 1 AnteErr evm tx, 2 group: a->b anteErr(a->b) / c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				for i := 3; i < 6; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 0, 0, 0},
		},
		{
			"5 failed evm txs, 1 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				for i := 0; i < 5; i++ {
					rawTxs = append(rawTxs, createFailedEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{11, 11, 11, 11, 11},
		},
		{
			"5 evm txs, 2 group:a->b b->c / d->e e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				//one group 3txs
				for i := 0; i < 3; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				//one group 2txs
				for i := 8; i > 6; i-- {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"5 failed evm txs, 2 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				//one group 3txs
				for i := 0; i < 3; i++ {
					rawTxs = append(rawTxs, createFailedEthTx(t, chain, i))
				}
				//one group 2txs
				for i := 8; i > 6; i-- {
					rawTxs = append(rawTxs, createFailedEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{11, 11, 11, 11, 11},
		},
		{
			"2 evm txs and 3 failed evm txs, 2 group:a->b b->c / failed(d->e e->f f->g)",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				rawTxs := [][]byte{}
				//one group 3txs
				for i := 0; i < 3; i++ {
					rawTxs = append(rawTxs, createFailedEthTx(t, chain, i))
				}
				//one group 2txs
				for i := 8; i > 6; i-- {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{11, 11, 11, 0, 0},
		},
		{
			"3 evm txs and 2 failed evm txs, 2 group:a->b failed(b->c) / d->e e->f failed(f->g)",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				rawTxs := [][]byte{}
				//one group 3txs
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 2))
				//one group 2txs
				for i := 8; i > 7; i-- {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 7))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 11, 0, 11},
		},
		{
			"3 contract txs and 2 normal evm txs, 2 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte

				for i := 0; i < 3; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				for i := 8; i > 6; i-- {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		// #####################
		// ## only cosmos txs ##
		// #####################
		{
			"5 cosmos txs, 1 group: a->b b->c c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				for i := 0; i < 5; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"4 cosmos txs, 1 Failed cosmos tx, 1 group: a->b failed(b->c) b->c c->d d->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 1))
				for i := 2; i < 5; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 61034, 0, 0, 0},
		},
		{
			"4 cosmos txs, 1 Failed cosmos tx, 2 group: a->b failed(b->c) / d->e e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 1))
				for i := 3; i < 6; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 61034, 0, 0, 0},
		},
		{
			"4 cosmos txs, 1 AnteErr cosmos tx, 1 group: a->b AnteErr(b->c) c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 1))
				for i := 2; i < 5; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 5, 0, 0, 0},
		},
		{
			"4 failed cosmos txs, 1 AnteErr cosmos tx, 1 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 1))
				for i := 2; i < 5; i++ {
					rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{61034, 5, 61034, 61034, 61034},
		},
		{
			"3 cosmos txs, 1 failed cosmos tx, 1 AnteErr cosmos tx, 1 group: a->b b->c c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 61034, 5, 0, 0},
		},
		{
			"5 failed cosmos txs, 1 group: a->b b->c c->d d->e e->f",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				for i := 0; i < 5; i++ {
					rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{61034, 61034, 61034, 61034, 61034},
		},
		{
			"5 cosmos txs, 2 group: a->b b->c / d->e e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				for i := 3; i < 6; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"4 cosmos txs, 1 AnteErr cosmos tx, 2 group: a->b AnteErr(b->c) / d->e e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 1))
				for i := 3; i < 6; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 5, 0, 0, 0},
		},
		{
			"3 cosmos txs, 1 failed cosmos tx, 1 AnteErr cosmos tx, 2 group: a->b failed(b->c) / AnteErr(d->e) e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 3))
				for i := 4; i < 6; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 61034, 5, 0, 0},
		},
		{
			"3 cosmos txs, 1 failed cosmos tx, 1 AnteErr cosmos tx, 2 group: a->b failed(b->c) AnteErr(d->e) / e->f f->g",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {

				var rawTxs [][]byte
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				for i := 4; i < 6; i++ {
					rawTxs = append(rawTxs, createTokenSendTx(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)
				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 61034, 5, 0, 0},
		},
		// #####################
		// ###### mix txs ######
		// #####################
		{
			"2 evm txs with 1 cosmos tx and 2 evm contract txs, 2 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				//one group 3txs
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				//cosmos tx
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 4; i < 6; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"2 evm txs, 1 cosmos tx, and 2 evm contract txs, 2 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 3))
				//one group 2txs
				for i := 5; i < 7; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 0, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm tx, 1 cosmos tx, and 2 evm contract txs, 2 group",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 3))
				//one group 2txs
				for i := 5; i < 7; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 0, 0, 0},
		},
		{
			"1 evm tx, 1 failed evm tx, 1 cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 0, 0, 0},
		},
		{
			"1 evm tx, 1 failed evm tx, 1 cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 0, 0, 0},
		},
		{
			"2 evm tx, 1 AnteErr cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 5, 0, 0},
		},
		{
			"2 evm tx, 1 failed cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 61034, 0, 0},
		},
		{
			"2 evm tx, 1 cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 3, 0},
		},
		{
			"2 evm tx, 1 cosmos tx, 1 failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 0, 11, 0},
		},
		{
			"2 evm tx, 1 AnteErr cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 5, 0, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 AnteErr cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 5, 0, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 failed cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 61034, 0, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 0, 3, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 cosmos tx, 1 failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 0, 11, 0},
		},
		{
			"1 evm tx, 1 failed evm, 1 AnteErr cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 5, 0, 0},
		},
		{
			"1 evm tx, 1 failed evm, 1 failed cosmos tx, and 2 evm contract txs",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				for i := 3; i < 5; i++ {
					rawTxs = append(rawTxs, callContract(t, chain, i))
				}
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 61034, 0, 0},
		},
		{
			"1 evm tx, 1 failed evm, 1 cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 0, 3, 0},
		},
		{
			"1 evm tx, 1 failed evm, 1 cosmos tx, 1 failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 0, 11, 0},
		},
		{
			"2 evm tx, 1 AnteErr cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 5, 3, 0},
		},
		{
			"2 evm tx, 1 AnteErr cosmos tx, 1 failed evm contract tx, and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 5, 11, 0},
		},
		{
			"2 evm tx, 1 failed cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 61034, 3, 0},
		},
		{
			"2 evm tx, 1 failed cosmos tx, 1 failed evm contract tx, and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				for i := 0; i < 2; i++ {
					rawTxs = append(rawTxs, createEthTx(t, chain, i))
				}
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 61034, 11, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 AnteErr cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 5, 3, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 AnteErr cosmos tx, 1 Failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 5, 11, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 Failed cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 61034, 3, 0},
		},
		{
			"1 evm tx, 1 Failed evm, 1 AnteErr cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 5, 3, 0},
		},
		{
			"1 evm tx, 1 Failed evm, 1 Failed cosmos tx, 1 Failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 61034, 11, 0},
		},
		{
			"1 evm tx, 1 Failed evm, 1 Failed cosmos tx, 1 AnteErr evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractAnteErr(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 61034, 3, 0},
		},
		{
			"1 evm tx, 1 Failed evm, 1 AnteErr cosmos tx, 1 Failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createFailedEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createAnteErrTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 11, 5, 11, 0},
		},
		{
			"1 evm tx, 1 AnteErr evm, 1 Failed cosmos tx, 1 Failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, createAnteErrEthTx(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 3, 61034, 11, 0},
		},
		{
			"1 evm tx, 1 callWasm vmb tx, 1 Failed cosmos tx, 1 Failed evm contract txs，and 1 evm contract tx",
			func(t *testing.T, chain *Chain, isParallel bool) ([]*abci.ResponseDeliverTx, []byte, []byte) {
				var rawTxs [][]byte
				rawTxs = append(rawTxs, createEthTx(t, chain, 0))
				rawTxs = append(rawTxs, callWasmAtContractA(t, chain, 1))
				rawTxs = append(rawTxs, createFailedTokenSendTx(t, chain, 2))
				//one group 2txs
				rawTxs = append(rawTxs, callContractFailed(t, chain, 3))
				rawTxs = append(rawTxs, callContract(t, chain, 4))
				ret := runTxs(chain, rawTxs, isParallel)

				return ret, resultHash(ret), chain.app.BaseApp.LastCommitID().Hash
			},
			[]uint32{0, 0, 61034, 11, 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			retA, resultHashA, appHashA := tc.executeTxs(t, chainA, true)
			retB, resultHashB, appHashB := tc.executeTxs(t, chainB, false)
			checkCodes(t, tc.title, retA, tc.expectedCodes)
			checkCodes(t, tc.title, retB, tc.expectedCodes)
			require.True(t, reflect.DeepEqual(resultHashA, resultHashB))
			require.True(t, reflect.DeepEqual(appHashA, appHashB))
		})
	}
}

func resultHash(txs []*abci.ResponseDeliverTx) []byte {
	results := tmtypes.NewResults(txs)
	return results.Hash()
}

// contract Storage {
// uint256 number;
// /**
// * @dev Store value in variable
// * @param num value to store
// */
// function store(uint256 num) public {
// number = num;
// }
// function add() public {
// number += 1;
// }
// /**
// * @dev Return value
// * @return value of 'number'
// */
// function retrieve() public view returns (uint256){
// return number;
// }
// }
var abiStr = `[{"inputs":[{"internalType":"uint256","name":"num","type":"uint256"}],"name":"add","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"retrieve","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"num","type":"uint256"}],"name":"store","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

func deployContract(t *testing.T, chain *Chain, i int) []byte {
	// Deploy contract - Owner.sol
	gasLimit := uint64(30000000)
	gasPrice := big.NewInt(100000000)

	//sender := ethcmn.HexToAddress(chain.priv[i].PubKey().Address().String())

	bytecode := ethcmn.FromHex("608060405234801561001057600080fd5b50610217806100206000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80631003e2d2146100465780632e64cec1146100625780636057361d14610080575b600080fd5b610060600480360381019061005b9190610105565b61009c565b005b61006a6100b7565b6040516100779190610141565b60405180910390f35b61009a60048036038101906100959190610105565b6100c0565b005b806000808282546100ad919061018b565b9250508190555050565b60008054905090565b8060008190555050565b600080fd5b6000819050919050565b6100e2816100cf565b81146100ed57600080fd5b50565b6000813590506100ff816100d9565b92915050565b60006020828403121561011b5761011a6100ca565b5b6000610129848285016100f0565b91505092915050565b61013b816100cf565b82525050565b60006020820190506101566000830184610132565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b6000610196826100cf565b91506101a1836100cf565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156101d6576101d561015c565b5b82820190509291505056fea2646970667358221220318e29d6b4806f219eedd0cc861e82c13e28eb7f42161f2c780dc539b0e32b4e64736f6c634300080a0033")
	msg := evmtypes.NewMsgEthereumTx(chain.seq[i], nil, big.NewInt(0), gasLimit, gasPrice, bytecode)
	err := msg.Sign(big.NewInt(3), chain.priv[i].ToECDSA())
	require.NoError(t, err)
	chain.seq[i]++
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	return rawTx
}

type CompiledContract struct {
	ABI abi.ABI
	Bin string
}

func UnmarshalContract(t *testing.T, cJson string) *CompiledContract {
	cc := new(CompiledContract)
	err := json.Unmarshal([]byte(cJson), cc)
	require.NoError(t, err)
	return cc
}

func callContract(t *testing.T, chain *Chain, i int) []byte {
	gasLimit := uint64(30000000)
	gasPrice := big.NewInt(100000000)
	//to := ethcmn.HexToAddress(chain.priv[i].PubKey().Address().String())
	to := ethcmn.BytesToAddress(chain.ContractAddr)
	cc := UnmarshalContract(t, contractJson)
	data, err := cc.ABI.Pack("add")
	require.NoError(t, err)
	msg := evmtypes.NewMsgEthereumTx(chain.seq[i], &to, big.NewInt(0), gasLimit, gasPrice, data)
	err = msg.Sign(big.NewInt(3), chain.priv[i].ToECDSA())
	require.NoError(t, err)
	chain.seq[i]++
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	return rawTx
}

func callWasmAtContractA(t *testing.T, chain *Chain, i int) []byte {
	gasLimit := uint64(30000000)
	gasPrice := big.NewInt(100000000)

	to := ethcmn.BytesToAddress(chain.VMBWasmContract.Bytes())
	cc := UnmarshalContract(t, testPrecompileABIAJson)
	wasmCallData := fmt.Sprintf(callWasmMsgFormat, 10, chain.addr[i].String())
	data, err := cc.ABI.Pack("callWasm", chain.VMBWasmContract.String(), hex.EncodeToString([]byte(wasmCallData)), true)
	require.NoError(t, err)
	msg := evmtypes.NewMsgEthereumTx(chain.seq[i], &to, big.NewInt(0), gasLimit, gasPrice, data)
	err = msg.Sign(big.NewInt(3), chain.priv[i].ToECDSA())
	require.NoError(t, err)
	chain.seq[i]++
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	return rawTx
}

func callContractFailed(t *testing.T, chain *Chain, i int) []byte {
	gasLimit := uint64(1)
	gasPrice := big.NewInt(100000000)
	//to := ethcmn.HexToAddress(chain.priv[i].PubKey().Address().String())
	to := ethcmn.BytesToAddress(chain.ContractAddr)
	cc := UnmarshalContract(t, contractJson)
	data, err := cc.ABI.Pack("add")
	require.NoError(t, err)
	msg := evmtypes.NewMsgEthereumTx(chain.seq[i], &to, big.NewInt(0), gasLimit, gasPrice, data)
	err = msg.Sign(big.NewInt(3), chain.priv[i].ToECDSA())
	require.NoError(t, err)
	chain.seq[i]++
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	return rawTx
}

func callContractAnteErr(t *testing.T, chain *Chain, i int) []byte {
	gasLimit := uint64(30000000)
	gasPrice := big.NewInt(100000000)
	//to := ethcmn.HexToAddress(chain.priv[i].PubKey().Address().String())
	to := ethcmn.BytesToAddress(chain.ContractAddr)
	cc := UnmarshalContract(t, contractJson)
	data, err := cc.ABI.Pack("add")
	require.NoError(t, err)
	msg := evmtypes.NewMsgEthereumTx(chain.seq[i]+1, &to, big.NewInt(0), gasLimit, gasPrice, data)
	err = msg.Sign(big.NewInt(3), chain.priv[i].ToECDSA())
	require.NoError(t, err)
	rawTx, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	return rawTx
}

func checkCodes(t *testing.T, title string, resp []*abci.ResponseDeliverTx, codes []uint32) {
	for i, code := range codes {
		require.True(t, resp[i].Code == code, "title: %s, expect code: %d, but %d! tx index: %d", title, code, resp[i].Code, i)
	}
}

func VMBPrecompileSetup(t *testing.T, chain *Chain) {
	timeValue := fmt.Sprintf("%d-04-11 13:33:37", chain.timeYear+1)
	testTime, _ := time.Parse("2006-01-02 15:04:05", timeValue)
	header := abci.Header{Height: chain.app.LastBlockHeight() + 1, ChainID: chain.chainIdStr, Time: testTime}
	chain.app.BaseApp.BeginBlock(abci.RequestBeginBlock{Header: header})

	chain.VMBContractA = vmbDeployEvmContract(t, chain, testPrecompileCodeA)
	initMsg := []byte(fmt.Sprintf("{\"decimals\":10,\"initial_balances\":[{\"address\":\"%s\",\"amount\":\"100000000\"}],\"name\":\"my test token\", \"symbol\":\"MTT\"}", chain.addr[0].String()))
	chain.VMBWasmContract = vmbDeployWasmContract(t, chain, "precompile.wasm", initMsg)

	chain.app.BaseApp.EndBlock(abci.RequestEndBlock{})
	chain.app.BaseApp.Commit(abci.RequestCommit{})
}

func vmbDeployEvmContract(t *testing.T, chain *Chain, code string) ethcmn.Address {
	freeCallBytecode := ethcmn.Hex2Bytes(code)
	_, contract, err := chain.app.VMBridgeKeeper.CallEvm(chain.Ctx(), ethcmn.BytesToAddress(chain.addr[0]), nil, big.NewInt(0), freeCallBytecode)
	require.NoError(t, err)
	chain.seq[0]++
	return contract.ContractAddress
}

func vmbDeployWasmContract(t *testing.T, chain *Chain, filename string, initMsg []byte) sdk.WasmAddress {
	wasmcode, err := ioutil.ReadFile(fmt.Sprintf("./testdata/%s", filename))
	require.NoError(t, err)
	codeid, err := chain.app.WasmPermissionKeeper.Create(chain.Ctx(), sdk.AccToAWasmddress(chain.addr[0]), wasmcode, nil)
	require.NoError(t, err)
	//initMsg := []byte(fmt.Sprintf("{\"decimals\":10,\"initial_balances\":[{\"address\":\"%s\",\"amount\":\"100000000\"}],\"name\":\"my test token\", \"symbol\":\"MTT\"}", suite.addr.String()))
	contract, _, err := chain.app.WasmPermissionKeeper.Instantiate(chain.Ctx(), codeid, sdk.AccToAWasmddress(chain.addr[0]), sdk.AccToAWasmddress(chain.addr[0]), initMsg, "label", sdk.Coins{sdk.NewInt64Coin(sdk.DefaultBondDenom, 0)})
	require.NoError(t, err)
	return contract
}
