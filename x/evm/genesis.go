package evm

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authexported "github.com/cosmos/cosmos-sdk/x/auth/exported"
	ethcmn "github.com/ethereum/go-ethereum/common"
	ethermint "github.com/okex/okexchain/app/types"
	"github.com/okex/okexchain/x/evm/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// InitGenesis initializes genesis state based on exported genesis
func InitGenesis(ctx sdk.Context, k Keeper, accountKeeper types.AccountKeeper, data GenesisState) []abci.ValidatorUpdate { // nolint: interfacer
	logger := ctx.Logger().With("module", types.ModuleName)
	initImportEnv()

	k.SetParams(ctx, data.Params)
	for _, account := range data.Accounts {
		address := ethcmn.HexToAddress(account.Address)
		accAddress := sdk.AccAddress(address.Bytes())

		// check that the EVM balance the matches the account balance
		acc := accountKeeper.GetAccount(ctx, accAddress)
		if acc == nil {
			panic(fmt.Errorf("account not found for address %s", account.Address))
		}

		_, ok := acc.(*ethermint.EthAccount)
		if !ok {
			panic(
				fmt.Errorf("account %s must be an %T type, got %T",
					account.Address, &ethermint.EthAccount{}, acc,
				),
			)
		}

		// read Code from file
		addGoroutine()
		go syncReadCodeFromFile(ctx, logger, k, address)

		// read Storage From file
		addGoroutine()
		go syncReadStorageFromFile(ctx, logger, k, address)
	}

	// wait for all data to be set into db
	globalWG.Wait()

	logger.Debug("Import finished:", "contract num", contractCounter.GetNum(), "state num", stateCounter.GetNum())

	k.SetChainConfig(ctx, data.ChainConfig)

	return []abci.ValidatorUpdate{}
}

// ExportGenesis exports genesis state of the EVM module
func ExportGenesis(ctx sdk.Context, k Keeper, ak types.AccountKeeper) GenesisState {
	logger := ctx.Logger().With("module", types.ModuleName)
	initExportEnv()

	// nolint: prealloc
	var ethGenAccounts []types.GenesisAccount
	ak.IterateAccounts(ctx, func(account authexported.Account) bool {
		ethAccount, ok := account.(*ethermint.EthAccount)
		if !ok {
			// ignore non EthAccounts
			return false
		}

		addr := ethAccount.EthAddress()

		// write Code
		addGoroutine()
		go syncWriteAccountCode(ctx, k, addr)
		// write Storage
		addGoroutine()
		go syncWriteAccountStorage(ctx, k, addr)

		genAccount := types.GenesisAccount{
			Address: addr.String(),
			Code:    nil,
			Storage: nil,
		}

		ethGenAccounts = append(ethGenAccounts, genAccount)
		return false
	})
	// wait for all data to be written into files
	globalWG.Wait()
	logger.Debug("Export finished:", "contract num", contractCounter.GetNum(), "state num", stateCounter.GetNum())

	config, _ := k.GetChainConfig(ctx)

	return GenesisState{
		Accounts:    ethGenAccounts,
		ChainConfig: config,
		Params:      k.GetParams(ctx),
	}
}
