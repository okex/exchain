package watcher_test

import (
	"math/big"
	"os"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"

	"github.com/okex/exchain/app"
	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	authclient "github.com/okex/exchain/libs/cosmos-sdk/x/auth/client/utils"
	authtypes "github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	tm "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/okex/exchain/x/distribution/keeper"
	etypes "github.com/okex/exchain/x/evm/types"
	"github.com/okex/exchain/x/evm/watcher"
	"github.com/okex/exchain/x/gov"
)

var (
	evmAmountZero = big.NewInt(0)
	evmGasLimit   = uint64(1000000)
	evmGasPrice   = big.NewInt(10000)
	evmChainID    = big.NewInt(3)

	cosmosChainId = "ethermint-3"

	nonce0 = uint64(0)
	nonce1 = uint64(1)
	//generate fees for stdTx
	Coin1000 = sdk.NewInt64Coin(sdk.DefaultBondDenom, 1000)

	txCoin10   = sdk.NewInt64Coin(sdk.DefaultBondDenom, 10)
	txFees     = auth.NewStdFee(21000, sdk.NewCoins(txCoin10))
	sysCoins10 = keeper.NewTestSysCoins(10, 0)
	sysCoins90 = keeper.NewTestSysCoins(90, 0)

	govProposalID1 = uint64(1)
	memo           = "hello, memo"

	blockHeight = int64(2)

	accountNum = uint64(0)
)

type TxTestSuite struct {
	suite.Suite
	app                *app.OKExChainApp
	codec              *codec.Codec
	Watcher            watcher.Watcher
	evmSenderPrivKey   ethsecp256k1.PrivKey
	evmContractAddress ethcommon.Address

	stdSenderPrivKey    ethsecp256k1.PrivKey
	stdSenderAccAddress sdk.AccAddress
}

// For generating DeliverTxResponse with DeliverTx
func (suite *TxTestSuite) SetupTest() {
	// Initialize OKExChainApp with already existing function
	// to avoid the watcher being disabled.
	w := setupTest()
	suite.app = w.app
	// Re-Initialize the chain with none-nil chain-id
	// to set the correct chain-id
	genesisState := app.NewDefaultGenesisState()
	stateBytes, err := codec.MarshalJSONIndent(suite.app.Codec(), genesisState)
	suite.Require().Nil(err, "Re-Initialize chain error")
	suite.app.InitChain(
		tm.RequestInitChain{
			Validators:    []tm.ValidatorUpdate{},
			AppStateBytes: stateBytes,
			ChainId:       cosmosChainId,
		},
	)

	params := etypes.DefaultParams()
	params.EnableCreate = true
	params.EnableCall = true
	suite.app.EvmKeeper.SetParams(suite.Ctx(), params)
}

func (suite *TxTestSuite) Ctx() sdk.Context {
	return suite.app.BaseApp.GetDeliverStateCtx()
}

func (suite *TxTestSuite) beginFakeBlock() {
	suite.evmSenderPrivKey, _ = ethsecp256k1.GenerateKey()
	suite.evmContractAddress = ethcrypto.CreateAddress(ethcommon.HexToAddress(suite.evmSenderPrivKey.PubKey().Address().String()), 0)
	accountEvm := suite.app.AccountKeeper.NewAccountWithAddress(suite.Ctx(), suite.evmSenderPrivKey.PubKey().Address().Bytes())
	accountEvm.SetAccountNumber(accountNum)
	accountEvm.SetCoins(sdk.NewCoins(Coin1000))
	suite.app.AccountKeeper.SetAccount(suite.Ctx(), accountEvm)

	suite.stdSenderPrivKey, _ = ethsecp256k1.GenerateKey()
	suite.stdSenderAccAddress = sdk.AccAddress(suite.stdSenderPrivKey.PubKey().Address())
	accountStd := suite.app.AccountKeeper.NewAccountWithAddress(suite.Ctx(), suite.stdSenderAccAddress.Bytes())
	accountStd.SetAccountNumber(accountNum)
	accountStd.SetCoins(sdk.NewCoins(Coin1000))
	suite.app.AccountKeeper.SetAccount(suite.Ctx(), accountStd)
	err := suite.app.BankKeeper.SetCoins(suite.Ctx(), suite.stdSenderAccAddress, sdk.NewCoins(Coin1000))
	suite.Require().NoError(err)

	tmtypes.UnittestOnlySetMilestoneVenusHeight(blockHeight - 1)
	global.SetGlobalHeight(blockHeight - 1)
	suite.app.BeginBlocker(suite.Ctx(), tm.RequestBeginBlock{Header: tm.Header{Height: blockHeight - 1}})
}

func (suite *TxTestSuite) endFakeBlock() {
	suite.app.EndBlocker(suite.Ctx(), tm.RequestEndBlock{})
}

func TestWatcherTx(t *testing.T) {
	suite.Run(t, new(TxTestSuite))
}

func (suite *TxTestSuite) TestRecordTxAndFailedReceipt() {
	testCases := []struct {
		title          string
		watcherEnabled bool
		buildInput     func() (tm.TxEssentials, *tm.ResponseDeliverTx) // build the input of the tested function
		numBatch       int
	}{
		{
			title:          "evmTx success with none-nil ResponseDeliverTx",
			watcherEnabled: true,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				//Create evm contract - Owner.sol
				bytecode := ethcommon.FromHex("0x608060405234801561001057600080fd5b50336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a36102c4806100dc6000396000f3fe608060405234801561001057600080fd5b5060043610610053576000357c010000000000000000000000000000000000000000000000000000000090048063893d20e814610058578063a6f9dae1146100a2575b600080fd5b6100606100e6565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6100e4600480360360208110156100b857600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061010f565b005b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146101d1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f43616c6c6572206973206e6f74206f776e65720000000000000000000000000081525060200191505060405180910390fd5b8073ffffffffffffffffffffffffffffffffffffffff166000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a3806000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055505056fea265627a7a72315820f397f2733a89198bc7fed0764083694c5b828791f39ebcbc9e414bccef14b48064736f6c63430005100032")
				tx := etypes.NewMsgEthereumTx(nonce0, nil, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				err := tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				suite.Require().NoError(err)
				txBytes, err := authclient.GetTxEncoder(nil, authclient.WithEthereumTx())(tx)
				suite.Require().NoError(err)
				res := suite.app.PreDeliverRealTx(txBytes)
				suite.Require().NotNil(res)
				resp := suite.app.DeliverRealTx(res)
				return tx, &resp
			},
			numBatch: 2,
		},
		{
			title:          "evmTx fail with none-nil ResponseDeliverTx",
			watcherEnabled: true,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				//Create evm contract - Owner.sol
				bytecode := ethcommon.FromHex("0xa6f9dae10000000000000000000000006a82e4a67715c8412a9114fbd2cbaefbc8181424")
				tx := etypes.NewMsgEthereumTx(nonce1, nil, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				resp := suite.app.DeliverRealTx(tx)
				return tx, &resp
			},
			numBatch: 3,
		},
		{
			title:          "StdTx success with none-nil ResponseDeliverTx",
			watcherEnabled: true,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				//send std tx for gov, success
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID1, sysCoins90)
				msgs := []sdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce0}, txFees, memo)
				resp := suite.app.DeliverRealTx(tx)
				return tx, &resp
			},
			numBatch: 1,
		},
		{
			title:          "Watcher Disabled",
			watcherEnabled: false,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				return nil, nil
			},
			numBatch: 0,
		},
		{
			title:          "ethTx with nil Response",
			watcherEnabled: true,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				//Create evm contract - Owner.sol
				bytecode := ethcommon.FromHex("0x608060405234801561001057600080fd5b50336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a36102c4806100dc6000396000f3fe608060405234801561001057600080fd5b5060043610610053576000357c010000000000000000000000000000000000000000000000000000000090048063893d20e814610058578063a6f9dae1146100a2575b600080fd5b6100606100e6565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6100e4600480360360208110156100b857600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061010f565b005b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146101d1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f43616c6c6572206973206e6f74206f776e65720000000000000000000000000081525060200191505060405180910390fd5b8073ffffffffffffffffffffffffffffffffffffffff166000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a3806000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055505056fea265627a7a72315820f397f2733a89198bc7fed0764083694c5b828791f39ebcbc9e414bccef14b48064736f6c63430005100032")
				tx := etypes.NewMsgEthereumTx(nonce0, nil, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				return tx, nil
			},
			numBatch: 1,
		},
		{
			title:          "StdTx with nil Response",
			watcherEnabled: true,
			buildInput: func() (tm.TxEssentials, *tm.ResponseDeliverTx) {
				//send std tx for gov, success
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID1, sysCoins90)
				msgs := []sdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce0}, txFees, memo)
				return tx, nil
			},
			numBatch: 0,
		},
	}

	suite.SetupTest()
	suite.beginFakeBlock()
	for _, tc := range testCases {
		suite.Run(tc.title, func() {
			//Reset Watcher when starting a new test case
			suite.Watcher = *(watcher.NewWatcher(log.NewTMLogger(os.Stdout)))
			suite.Watcher.Enable(tc.watcherEnabled)
			simBlockHash := ethcommon.BytesToHash([]byte("block_hash"))
			simHeader := tm.Header{}
			suite.Watcher.NewHeight(uint64(blockHeight-1), simBlockHash, simHeader)
			//Build input transaction and ResponseDeliverTx
			tx, resp := tc.buildInput()
			suite.Watcher.RecordTxAndFailedReceipt(tx, resp, etypes.TxDecoder(suite.codec))
			// Get batch from WatchData
			WDatagen := suite.Watcher.CreateWatchDataGenerator()
			WatchDataByte, err := WDatagen()
			if !tc.watcherEnabled {
				suite.Require().Nil(err, "Watcher Disabled error")
				suite.Require().Nil(WatchDataByte, "Watcher Disabled error")
				return
			}
			// 0 batch returns nil WatchDataByte
			if tc.numBatch == 0 {
				suite.Require().Nil(err, "Get WatchData Byte error")
				suite.Require().Nil(WatchDataByte, "Add Batch error : 0 batch should be added")
				return
			}
			suite.Require().Nil(err, "Get WatchData Byte error")
			WatchDataInterface, err := suite.app.EvmKeeper.Watcher.UnmarshalWatchData(WatchDataByte)
			suite.Require().Nil(err, "Get WatchData Decode Result error")
			WatchData, ok := WatchDataInterface.(watcher.WatchData)
			suite.Require().True(ok, "Convert New WatchData Result error")
			lenBatch := len(WatchData.Batches)
			//Only test if the number of batches increase as expected.
			suite.Require().True(lenBatch == tc.numBatch, "Add Batch Number Wrong")
		})
	}
	suite.endFakeBlock()
}

func newTestStdTx(msgs []sdk.Msg, privs []crypto.PrivKey, accNums []uint64, seqs []uint64, fee auth.StdFee, memo string) sdk.Tx {
	sigs := make([]authtypes.StdSignature, len(privs))
	for i, priv := range privs {
		sig, err := priv.Sign(authtypes.StdSignBytes(cosmosChainId, accNums[i], seqs[i], fee, msgs, memo))
		if err != nil {
			panic(err)
		}
		sigs[i] = authtypes.StdSignature{PubKey: priv.PubKey(), Signature: sig}
	}

	tx := auth.NewStdTx(msgs, fee, sigs, memo)
	return tx
}
