package app

import (
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/x/upgrade"
	"github.com/okex/exchain/x/debug"
	"github.com/okex/exchain/x/dex"
	distr "github.com/okex/exchain/x/distribution"
	"github.com/okex/exchain/x/farm"
	"github.com/okex/exchain/x/params"

	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/codec"
)

func TestExChainAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	genesisState := ModuleBasics.DefaultGenesis()
	stateBytes, err := codec.MarshalJSONIndent(app.cdc, genesisState)
	require.NoError(t, err)

	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	app2 := NewExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)
	_, _, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

func TestModuleManager(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	for moduleName, _ := range ModuleBasics {
		if moduleName == upgrade.ModuleName || moduleName == debug.ModuleName {
			continue
		}
		_, found := app.mm.Modules[moduleName]
		require.True(t, found)
	}
}

func TestProposalManager(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewExChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

	require.True(t, app.GovKeeper.Router().HasRoute(params.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(dex.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(distr.RouterKey))
	require.True(t, app.GovKeeper.Router().HasRoute(farm.RouterKey))

	require.True(t, app.GovKeeper.ProposalHandleRouter().HasRoute(params.RouterKey))
	require.True(t, app.GovKeeper.ProposalHandleRouter().HasRoute(dex.RouterKey))
	require.True(t, app.GovKeeper.ProposalHandleRouter().HasRoute(farm.RouterKey))
}
