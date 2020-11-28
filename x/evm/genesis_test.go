package evm_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/okex/okexchain/app/crypto/ethsecp256k1"
	ethermint "github.com/okex/okexchain/app/types"
	"github.com/okex/okexchain/x/evm"
	"github.com/okex/okexchain/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	ethcmn "github.com/ethereum/go-ethereum/common"
)

func (suite *EvmTestSuite) TestExportImport() {
	var genState types.GenesisState
	suite.Require().NotPanics(func() {
		genState = evm.ExportGenesis(suite.ctx, suite.app.EvmKeeper, suite.app.AccountKeeper)
	})

	_ = evm.InitGenesis(suite.ctx, suite.app.EvmKeeper, suite.app.AccountKeeper, genState)
}

func (suite *EvmTestSuite) TestInitGenesis() {
	privkey, err := ethsecp256k1.GenerateKey()
	suite.Require().NoError(err)

	address := ethcmn.HexToAddress(privkey.PubKey().Address().String())

	testCases := []struct {
		name     string
		malleate func()
		genState types.GenesisState
		expPanic bool
	}{
		{
			"default",
			func() {},
			types.DefaultGenesisState(),
			false,
		},
		{
			"valid account",
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, address.Bytes())
				suite.Require().NotNil(acc)
				err := acc.SetCoins(sdk.NewCoins(ethermint.NewPhotonCoinInt64(1)))
				suite.Require().NoError(err)
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Balance: sdk.OneDec(),
						Storage: types.Storage{
							{Key: common.BytesToHash([]byte("key")), Value: common.BytesToHash([]byte("value"))},
						},
					},
				},
			},
			false,
		},
		{
			"account not found",
			func() {},
			types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
					},
				},
			},
			true,
		},
		{
			"invalid account type",
			func() {
				acc := authtypes.NewBaseAccountWithAddress(address.Bytes())
				suite.app.AccountKeeper.SetAccount(suite.ctx, &acc)
			},
			types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Balance: sdk.OneDec(),
					},
				},
			},
			true,
		},
		{
			"balance mismatch",
			func() {
				acc := suite.app.AccountKeeper.NewAccountWithAddress(suite.ctx, address.Bytes())
				suite.Require().NotNil(acc)
				suite.app.AccountKeeper.SetAccount(suite.ctx, acc)
			},
			types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Balance: sdk.OneDec(),
					},
				},
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset values

			tc.malleate()

			if tc.expPanic {
				suite.Require().Panics(
					func() {
						_ = evm.InitGenesis(suite.ctx, suite.app.EvmKeeper, suite.app.AccountKeeper, tc.genState)
					},
				)
			} else {
				suite.Require().NotPanics(
					func() {
						_ = evm.InitGenesis(suite.ctx, suite.app.EvmKeeper, suite.app.AccountKeeper, tc.genState)
					},
				)
			}
		})
	}
}
