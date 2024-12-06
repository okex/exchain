package sanity

import (
	"github.com/spf13/viper"

	"github.com/okex/exchain/app/config"
	apptype "github.com/okex/exchain/app/types"
	"github.com/okex/exchain/libs/cosmos-sdk/server"
	cosmost "github.com/okex/exchain/libs/cosmos-sdk/store/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/tendermint/consensus"
	"github.com/okex/exchain/libs/tendermint/state"
	"github.com/okex/exchain/libs/tendermint/types"
	"github.com/okex/exchain/x/evm/watcher"
	"github.com/okex/exchain/x/infura"
)

// CheckStart check start command's flags. if user set conflict flags return error.
// the conflicts flags are:
// --fast-query      conflict with --pruning=nothing
// --enable-preruntx conflict with --download-delta
//
// based the conflicts above and node-mode below
// --node-mode=rpc manage the following flags:
//     --disable-checktx-mutex=true
//     --disable-query-mutex=true
//     --enable-bloom-filter=true
//     --fast-lru=10000
//     --fast-query=true
//     --iavl-enable-async-commit=true
//     --max-open=20000
//     --mempool.enable_pending_pool=true
//     --cors=*
//
// --node-mode=validator manage the following flags:
//     --disable-checktx-mutex=true
//     --disable-query-mutex=true
//     --dynamic-gp-mode=2
//     --iavl-enable-async-commit=true
//     --iavl-cache-size=10000000
//     --pruning=everything
//
// --node-mode=archive manage the following flags:
//    --pruning=nothing
//    --disable-checktx-mutex=true
//    --disable-query-mutex=true
//    --enable-bloom-filter=true
//    --iavl-enable-async-commit=true
//    --max-open=20000
//    --cors=*
//
// then
// --node-mode=archive(--pruning=nothing) conflicts with --fast-query

var (
	startDependentElems = []dependentPair{
		{ // if infura.FlagEnable=true , watcher.FlagFastQuery must be set to true
			config:       boolItem{name: infura.FlagEnable, expect: true},
			reliedConfig: boolItem{name: watcher.FlagFastQuery, expect: true},
		},
	}
	// conflicts flags
	startConflictElems = []conflictPair{
		// --fast-query      conflict with --pruning=nothing
		{
			configA: boolItem{name: watcher.FlagFastQuery, expect: true},
			configB: stringItem{name: server.FlagPruning, expect: cosmost.PruningOptionNothing},
		},
		// --enable-preruntx conflict with --download-delta
		{
			configA: boolItem{name: consensus.EnablePrerunTx, expect: true},
			configB: boolItem{name: types.FlagDownloadDDS, expect: true},
		},
		// --enable-preruntx conflict with --delta-mode down-redis|down-persist
		{
			configA: boolItem{name: consensus.EnablePrerunTx, expect: true},
			configB: funcItem{
				name:   types.FlagDeltaMode,
				expect: true,
				actual: false,
				f: func() bool {
					return types.IsDeltaModeDownload(viper.GetString(types.FlagDeltaMode))
				},
			},
		},
		// --multi-cache conflict with --download-delta
		{
			configA: boolItem{name: sdk.FlagMultiCache, expect: true},
			configB: boolItem{name: types.FlagDownloadDDS, expect: true},
		},
		// --multi-cache conflict with --delta-mode down-redis|down-persist
		{
			configA: boolItem{name: sdk.FlagMultiCache, expect: true},
			configB: funcItem{
				name:   types.FlagDeltaMode,
				expect: true,
				actual: false,
				f: func() bool {
					return types.IsDeltaModeDownload(viper.GetString(types.FlagDeltaMode))
				},
			},
		},
		{
			configA: stringItem{name: apptype.FlagNodeMode, expect: string(apptype.RpcNode)},
			configB: stringItem{name: server.FlagPruning, expect: cosmost.PruningOptionNothing},
		},
		// --node-mode=archive(--pruning=nothing) conflicts with --fast-query
		{
			configA: stringItem{name: apptype.FlagNodeMode, expect: string(apptype.ArchiveNode)},
			configB: boolItem{name: watcher.FlagFastQuery, expect: true},
		},
		{
			configA: stringItem{name: apptype.FlagNodeMode, expect: string(apptype.RpcNode)},
			configB: boolItem{name: config.FlagEnablePGU, expect: true},
		},
		{
			configA: stringItem{name: apptype.FlagNodeMode, expect: string(apptype.ArchiveNode)},
			configB: boolItem{name: config.FlagEnablePGU, expect: true},
		},
		{
			configA: stringItem{name: apptype.FlagNodeMode, expect: string(apptype.InnertxNode)},
			configB: boolItem{name: config.FlagEnablePGU, expect: true},
		},
	}

	checkRangeItems = []rangeItem{
		{
			enumRange: []int{int(state.DeliverTxsExecModeSerial), state.DeliverTxsExecModeParallel},
			name:      state.FlagDeliverTxsExecMode,
		},
	}
)

// CheckStart check start command.If it has conflict pair above. then return the conflict error
func CheckStart() error {
	if viper.GetBool(FlagDisableSanity) {
		return nil
	}
	for _, v := range startDependentElems {
		if err := v.check(); err != nil {
			return err
		}
	}
	for _, v := range startConflictElems {
		if err := v.check(); err != nil {
			return err
		}
	}

	for _, v := range checkRangeItems {
		if err := v.checkRange(); err != nil {
			return err
		}
	}

	return nil
}
