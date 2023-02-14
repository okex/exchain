package token

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/common/perf"
	"github.com/okex/exchain/x/token/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
)

// BeginBlocker is called when dapp handles with abci::BeginBlock
func beginBlocker(ctx sdk.Context, keeper Keeper) {
	seq := perf.GetPerf().OnBeginBlockEnter(ctx, types.ModuleName)
	defer perf.GetPerf().OnBeginBlockExit(ctx, types.ModuleName, seq)

	keeper.ResetCache(ctx)

	exportAccountHeight := cfg.DynamicConfig.GetExportAccountHeight()
	if ctx.BlockHeight() == int64(exportAccountHeight) {
		exportAccounts(ctx, keeper)
	}
}
