package app

import (
	"math/big"
	"os"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	cosmossdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	authclient "github.com/okex/exchain/libs/cosmos-sdk/x/auth/client/utils"
	authtypes "github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/upgrade"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	abcitypes "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tendertypes "github.com/okex/exchain/libs/tendermint/types"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/okex/exchain/x/dex"
	distr "github.com/okex/exchain/x/distribution"
	"github.com/okex/exchain/x/distribution/keeper"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/okex/exchain/x/farm"
	"github.com/okex/exchain/x/gov"
	"github.com/okex/exchain/x/params"
)

var (
	txCoin10    = cosmossdk.NewInt64Coin(cosmossdk.DefaultBondDenom, 10)
	txCoin1000  = cosmossdk.NewInt64Coin(cosmossdk.DefaultBondDenom, 1000)
	txFees      = auth.NewStdFee(21000, cosmossdk.NewCoins(txCoin10))
	txFeesError = auth.NewStdFee(100000000000000, cosmossdk.NewCoins(cosmossdk.NewInt64Coin(cosmossdk.DefaultBondDenom, 1000000000000000000)))

	cosmosChainId = "ethermint-3"
	checkTx       = false
	blockHeight   = int64(2)

	evmAmountZero = big.NewInt(0)
	evmGasLimit   = uint64(1000000)
	evmGasPrice   = big.NewInt(10000)
	evmChainID    = big.NewInt(3)

	nonce0 = uint64(0)
	nonce1 = uint64(1)
	nonce2 = uint64(2)
	nonce3 = uint64(3)

	accountNum = uint64(0)

	sysCoins10 = keeper.NewTestSysCoins(10, 0)
	sysCoins90 = keeper.NewTestSysCoins(90, 0)
	memo       = "hello, memo"

	govProposalID1 = uint64(1)
	govProposalID2 = uint64(2)
)

func TestOKExChainAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewOKExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	genesisState := ModuleBasics.DefaultGenesis()
	stateBytes, err := codec.MarshalJSONIndent(app.Codec(), genesisState)
	require.NoError(t, err)

	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app.Commit(abci.RequestCommit{})

	// Making a new app object with the db, so that initchain hasn't been called
	app2 := NewOKExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)
	_, _, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

func TestModuleManager(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewOKExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	for moduleName, _ := range ModuleBasics {
		if moduleName == upgrade.ModuleName {
			continue
		}
		_, found := app.mm.Modules[moduleName]
		require.True(t, found)
	}
}

func TestProposalManager(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewOKExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	require.True(t, app.GovKeeper.Router().HasRoute(params.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(dex.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(distr.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(farm.RouterKey))

	require.True(t, app.GovKeeper.ProposalHandlerRouter().HasRoute(params.RouterKey))
	require.True(t, app.GovKeeper.ProposalHandlerRouter().HasRoute(dex.RouterKey))
	require.True(t, app.GovKeeper.ProposalHandlerRouter().HasRoute(farm.RouterKey))
}

func TestFakeBlockTxSuite(t *testing.T) {
	suite.Run(t, new(FakeBlockTxTestSuite))
}

type FakeBlockTxTestSuite struct {
	suite.Suite
	app   *OKExChainApp
	codec *codec.Codec

	evmSenderPrivKey   ethsecp256k1.PrivKey
	evmContractAddress ethcommon.Address

	stdSenderPrivKey    ethsecp256k1.PrivKey
	stdSenderAccAddress cosmossdk.AccAddress
}

func (suite *FakeBlockTxTestSuite) SetupTest() {
	suite.app = Setup(checkTx, WithChainId(cosmosChainId))
	suite.codec = suite.app.Codec()
	params := evmtypes.DefaultParams()
	params.EnableCreate = true
	params.EnableCall = true
	suite.app.EvmKeeper.SetParams(suite.Ctx(), params)
}

func (suite *FakeBlockTxTestSuite) Ctx() cosmossdk.Context {
	return suite.app.BaseApp.GetDeliverStateCtx()
}

func (suite *FakeBlockTxTestSuite) beginFakeBlock() {
	suite.evmSenderPrivKey, _ = ethsecp256k1.GenerateKey()
	suite.evmContractAddress = ethcrypto.CreateAddress(ethcommon.HexToAddress(suite.evmSenderPrivKey.PubKey().Address().String()), 0)
	accountEvm := suite.app.AccountKeeper.NewAccountWithAddress(suite.Ctx(), suite.evmSenderPrivKey.PubKey().Address().Bytes())
	accountEvm.SetAccountNumber(accountNum)
	accountEvm.SetCoins(cosmossdk.NewCoins(txCoin1000))
	suite.app.AccountKeeper.SetAccount(suite.Ctx(), accountEvm)

	suite.stdSenderPrivKey, _ = ethsecp256k1.GenerateKey()
	suite.stdSenderAccAddress = cosmossdk.AccAddress(suite.stdSenderPrivKey.PubKey().Address())
	accountStd := suite.app.AccountKeeper.NewAccountWithAddress(suite.Ctx(), suite.stdSenderAccAddress.Bytes())
	accountStd.SetAccountNumber(accountNum)
	accountStd.SetCoins(cosmossdk.NewCoins(txCoin1000))
	suite.app.AccountKeeper.SetAccount(suite.Ctx(), accountStd)
	err := suite.app.BankKeeper.SetCoins(suite.Ctx(), suite.stdSenderAccAddress, cosmossdk.NewCoins(txCoin1000))
	suite.Require().NoError(err)

	tendertypes.UnittestOnlySetMilestoneVenusHeight(blockHeight - 1)
	global.SetGlobalHeight(blockHeight - 1)
	suite.app.BeginBlocker(suite.Ctx(), abcitypes.RequestBeginBlock{Header: abcitypes.Header{Height: blockHeight}})
}

func (suite *FakeBlockTxTestSuite) endFakeBlock(totalGas int64) {
	suite.app.EndBlocker(suite.Ctx(), abcitypes.RequestEndBlock{})
	ctx := suite.Ctx()
	blockActualGas := ctx.BlockGasMeter().GasConsumed()
	suite.Require().True(cosmossdk.Gas(totalGas) == blockActualGas, "block gas expect %d, but %d ", totalGas, blockActualGas)
	suite.Require().False(ctx.BlockGasMeter().IsPastLimit())
	suite.Require().False(ctx.BlockGasMeter().IsOutOfGas())
}

func (suite *FakeBlockTxTestSuite) TestFakeBlockTx() {
	testCases := []struct {
		title      string
		buildTx    func() []byte
		expectCode uint32
		expectGas  int64
	}{
		{
			"create evm contract, success",
			func() []byte {
				//Create evm contract - Owner.sol
				bytecode := ethcommon.FromHex("0x608060405234801561001057600080fd5b50336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a36102c4806100dc6000396000f3fe608060405234801561001057600080fd5b5060043610610053576000357c010000000000000000000000000000000000000000000000000000000090048063893d20e814610058578063a6f9dae1146100a2575b600080fd5b6100606100e6565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6100e4600480360360208110156100b857600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061010f565b005b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146101d1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f43616c6c6572206973206e6f74206f776e65720000000000000000000000000081525060200191505060405180910390fd5b8073ffffffffffffffffffffffffffffffffffffffff166000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167f342827c97908e5e2f71151c08502a66d44b6f758e3ac2f1de95f02eb95f0a73560405160405180910390a3806000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055505056fea265627a7a72315820f397f2733a89198bc7fed0764083694c5b828791f39ebcbc9e414bccef14b48064736f6c63430005100032")
				tx := evmtypes.NewMsgEthereumTx(nonce0, nil, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				txBytes, err := authclient.GetTxEncoder(nil, authclient.WithEthereumTx())(tx)
				suite.Require().NoError(err)
				return txBytes
			},
			0,
			231649,
		},
		{
			"create evm contract, failed",
			func() []byte {
				//Create evm contract - Owner.sol
				bytecode := ethcommon.FromHex("0xa6f9dae10000000000000000000000006a82e4a67715c8412a9114fbd2cbaefbc8181424")
				tx := evmtypes.NewMsgEthereumTx(nonce1, nil, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				txBytes, err := authclient.GetTxEncoder(nil, authclient.WithEthereumTx())(tx)
				suite.Require().NoError(err)
				return txBytes
			},
			7, //invalid opcode: opcode 0xa6 not defined: failed to execute message; message index: 0
			1000000,
		},
		{
			"call evm contract with function changeOwner, success",
			func() []byte {
				// Call evm contract with function changeOwner, for saving data.
				storeAddr := "0xa6f9dae10000000000000000000000006a82e4a67715c8412a9114fbd2cbaefbc8181424"
				bytecode := ethcommon.FromHex(storeAddr)

				tx := evmtypes.NewMsgEthereumTx(nonce2, &suite.evmContractAddress, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())

				txEncoder := authclient.GetTxEncoder(nil, authclient.WithEthereumTx())
				txBytes, _ := txEncoder(tx)
				return txBytes
			},
			0,
			30789,
		},
		{
			"call evm contract with function changeOwner, failed",
			func() []byte {
				// call evm contract with function changeOwner, error with function bytecode
				storeAddr := "0x11111111"
				bytecode := ethcommon.FromHex(storeAddr)
				tx := evmtypes.NewMsgEthereumTx(nonce3, &suite.evmContractAddress, evmAmountZero, evmGasLimit, evmGasPrice, bytecode)
				tx.Sign(evmChainID, suite.evmSenderPrivKey.ToECDSA())
				txBytes, _ := authclient.GetTxEncoder(nil, authclient.WithEthereumTx())(tx)
				return txBytes
			},
			7, //execution reverted: failed to execute message; message index: 0
			21195,
		},
		{
			"send std tx for gov, success",
			func() []byte {
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID1, sysCoins90)
				msgs := []cosmossdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce0}, txFees, memo)

				txEncoder := authclient.GetTxEncoder(suite.codec)
				txBytes, _ := txEncoder(tx)
				return txBytes
			},
			0,
			159732,
		},
		{
			"send tx for gov with error fee, failed, do not write to block",
			func() []byte {
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID1, sysCoins90)
				msgs := []cosmossdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce1}, txFeesError, memo)

				txEncoder := authclient.GetTxEncoder(suite.codec)
				txBytes, _ := txEncoder(tx)
				return txBytes
			},
			5, //insufficient funds: insufficient funds to pay for fees; 890.000000000000000000okt < 1000000000000000000.000000000000000000okt
			0,
		},
		{
			"send tx for gov with repeat proposal id, failed",
			func() []byte {
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID1, sysCoins90)
				msgs := []cosmossdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce1}, txFees, memo)

				txEncoder := authclient.GetTxEncoder(suite.codec)
				txBytes, _ := txEncoder(tx)
				return txBytes
			},
			68007, //the status of proposal is not for this operation: failed to execute message; message index: 1
			121641,
		},
		{
			"send std tx for gov again with proposal id 2, success",
			func() []byte {
				content := gov.NewTextProposal("Test", "description")
				newProposalMsg := gov.NewMsgSubmitProposal(content, sysCoins10, suite.stdSenderAccAddress)
				depositMsg := gov.NewMsgDeposit(suite.stdSenderAccAddress, govProposalID2, sysCoins90)
				msgs := []cosmossdk.Msg{newProposalMsg, depositMsg}
				tx := newTestStdTx(msgs, []crypto.PrivKey{suite.stdSenderPrivKey}, []uint64{accountNum}, []uint64{nonce2}, txFees, memo)

				txEncoder := authclient.GetTxEncoder(suite.codec)
				txBytes, _ := txEncoder(tx)
				return txBytes
			},
			0,
			154279,
		},
	}

	suite.SetupTest()
	suite.beginFakeBlock()
	totalGas := int64(0)
	for _, tc := range testCases {
		suite.Run(tc.title, func() {
			txReal := suite.app.PreDeliverRealTx(tc.buildTx())
			suite.Require().NotNil(txReal)
			resp := suite.app.DeliverRealTx(txReal)
			totalGas += resp.GasUsed
			suite.Require().True(tc.expectCode == resp.Code, "%s, expect code:%d, but %d ", tc.title, tc.expectCode, resp.Code)
			suite.Require().True(tc.expectGas == resp.GasUsed, "%s, expect gas:%d, but %d ", tc.title, tc.expectGas, resp.GasUsed)
		})
	}
	suite.endFakeBlock(totalGas)
}

func newTestStdTx(msgs []cosmossdk.Msg, privs []crypto.PrivKey, accNums []uint64, seqs []uint64, fee auth.StdFee, memo string) cosmossdk.Tx {
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
