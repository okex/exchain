package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/okex/okexchain/x/farm/types"
)

// getEarnings gets the earnings info by a given user address and a specific pool name
func (k Keeper) getEarnings(
	ctx sdk.Context, poolName string, accAddr sdk.AccAddress,
) (types.Earnings, sdk.Error) {
	var earnings types.Earnings
	lockInfo, found := k.GetLockInfo(ctx, accAddr, poolName)
	if !found {
		return earnings, types.ErrNoLockInfoFound(types.DefaultCodespace, accAddr.String())
	}

	pool, found := k.GetFarmPool(ctx, poolName)
	if !found {
		return earnings, types.ErrNoFarmPoolFound(types.DefaultCodespace, poolName)
	}

	// 1.1 Calculate how many provided token & native token have been yielded
	// between start block height and current height
	updatedPool, yieldedTokens := k.CalculateAmountYieldedBetween(ctx, pool)

	endingPeriod := k.IncrementPoolPeriod(ctx, poolName, updatedPool.TotalValueLocked, yieldedTokens)
	rewards := k.calculateRewards(ctx, poolName, accAddr, endingPeriod, lockInfo)

	earnings = types.NewEarnings(ctx.BlockHeight(), lockInfo.Amount, rewards)
	return earnings, nil
}
