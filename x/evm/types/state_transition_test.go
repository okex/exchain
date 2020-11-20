package types_test

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/okex/okexchain/app/crypto/ethsecp256k1"
	ethermint "github.com/okex/okexchain/app/types"
	"github.com/okex/okexchain/x/evm/types"

	ethcmn "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func (suite *StateDBTestSuite) TestTransitionDb() {
	suite.stateDB.SetNonce(suite.address, 123)

	addr := sdk.AccAddress(suite.address.Bytes())
	balance := ethermint.NewPhotonCoin(sdk.NewInt(5000))
	acc := suite.app.AccountKeeper.GetAccount(suite.ctx, addr)
	_ = acc.SetCoins(sdk.NewCoins(balance))
	suite.app.AccountKeeper.SetAccount(suite.ctx, acc)

	priv, err := ethsecp256k1.GenerateKey()
	suite.Require().NoError(err)
	recipient := ethcrypto.PubkeyToAddress(priv.ToECDSA().PublicKey)

	testCase := []struct {
		name     string
		malleate func()
		state    types.StateTransition
		expPass  bool
	}{
		{
			"passing state transition",
			func() {},
			types.StateTransition{
				AccountNonce: 123,
				Price:        sdk.NewDec(10).BigInt(),
				GasLimit:     11,
				Recipient:    &recipient,
				Amount:       sdk.NewDec(50).BigInt(),
				Payload:      []byte("data"),
				ChainID:      big.NewInt(1),
				Csdb:         suite.stateDB,
				TxHash:       &ethcmn.Hash{},
				Sender:       suite.address,
				Simulate:     suite.ctx.IsCheckTx(),
			},
			true,
		},
		{
			"contract creation",
			func() {},
			types.StateTransition{
				AccountNonce: 123,
				Price:        sdk.NewDec(10).BigInt(),
				GasLimit:     11,
				Recipient:    nil,
				Amount:       sdk.NewDec(10).BigInt(),
				Payload:      []byte("data"),
				ChainID:      big.NewInt(1),
				Csdb:         suite.stateDB,
				TxHash:       &ethcmn.Hash{},
				Sender:       suite.address,
				Simulate:     true,
			},
			true,
		},
		{
			"state transition simulation",
			func() {},
			types.StateTransition{
				AccountNonce: 123,
				Price:        sdk.NewDec(10).BigInt(),
				GasLimit:     11,
				Recipient:    &recipient,
				Amount:       sdk.NewDec(10).BigInt(),
				Payload:      []byte("data"),
				ChainID:      big.NewInt(1),
				Csdb:         suite.stateDB,
				TxHash:       &ethcmn.Hash{},
				Sender:       suite.address,
				Simulate:     true,
			},
			true,
		},
		{
			"fail by sending more than balance",
			func() {},
			types.StateTransition{
				AccountNonce: 123,
				Price:        sdk.NewDec(10).BigInt(),
				GasLimit:     11,
				Recipient:    &recipient,
				Amount:       sdk.NewDec(500000).BigInt(),
				Payload:      []byte("data"),
				ChainID:      big.NewInt(1),
				Csdb:         suite.stateDB,
				TxHash:       &ethcmn.Hash{},
				Sender:       suite.address,
				Simulate:     suite.ctx.IsCheckTx(),
			},
			false,
		},
		{
			"nil gas price",
			func() {
				invalidGas := sdk.DecCoins{
					{Denom: ethermint.NativeToken},
				}
				suite.ctx = suite.ctx.WithMinGasPrices(invalidGas)
			},
			types.StateTransition{
				AccountNonce: 123,
				Price:        sdk.NewDec(10).BigInt(),
				GasLimit:     11,
				Recipient:    &recipient,
				Amount:       sdk.NewDec(10).BigInt(),
				Payload:      []byte("data"),
				ChainID:      big.NewInt(1),
				Csdb:         suite.stateDB,
				TxHash:       &ethcmn.Hash{},
				Sender:       suite.address,
				Simulate:     suite.ctx.IsCheckTx(),
			},
			false,
		},
	}

	for _, tc := range testCase {
		tc.malleate()

		_, err = tc.state.TransitionDb(suite.ctx, types.DefaultChainConfig())

		if tc.expPass {
			suite.Require().NoError(err, tc.name)
			fromBalance := suite.app.EvmKeeper.GetBalance(suite.ctx, suite.address)
			toBalance := suite.app.EvmKeeper.GetBalance(suite.ctx, recipient)
			suite.Require().Equal(fromBalance, sdk.NewDec(4950).BigInt(), tc.name)
			suite.Require().Equal(toBalance, sdk.NewDec(50).BigInt(), tc.name)
		} else {
			suite.Require().Error(err, tc.name)
		}
	}
}
