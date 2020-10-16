package farm

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/okex/okexchain/x/common/perf"
	"github.com/okex/okexchain/x/farm/keeper"
	"github.com/okex/okexchain/x/farm/types"
)

// NewHandler creates an sdk.Handler for all the farm type messages
func NewHandler(k keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		ctx = ctx.WithEventManager(sdk.NewEventManager())
		logger := ctx.Logger().With("module", types.ModuleName)

		var handlerFun func() sdk.Result
		var name string
		switch msg := msg.(type) {
		case types.MsgCreatePool:
			name = "handleMsgCreatePool"
			handlerFun = func() sdk.Result {
				return handleMsgCreatePool(ctx, k, msg, logger)
			}
		case types.MsgDestroyPool:
			name = "handleMsgDestroyPool"
			handlerFun = func() sdk.Result {
				return handleMsgDestroyPool(ctx, k, msg, logger)
			}
		case types.MsgProvide:
			name = "handleMsgProvide"
			handlerFun = func() sdk.Result {
				return handleMsgProvide(ctx, k, msg, logger)
			}
		case types.MsgLock:
			name = "handleMsgLock"
			handlerFun = func() sdk.Result {
				return handleMsgLock(ctx, k, msg, logger)
			}
		case types.MsgUnlock:
			name = "handleMsgUnlock"
			handlerFun = func() sdk.Result {
				return handleMsgUnlock(ctx, k, msg, logger)
			}
		case types.MsgClaim:
			name = "handleMsgClaim"
			handlerFun = func() sdk.Result {
				return handleMsgClaim(ctx, k, msg, logger)
			}
		case types.MsgSetWhite:
			name = "handleMsgSetWhite"
			handlerFun = func() sdk.Result {
				return handleMsgSetWhite(ctx, k, msg, logger)
			}
		default:
			errMsg := fmt.Sprintf("unrecognized %s message type: %T", types.ModuleName, msg)
			return sdk.ErrUnknownRequest(errMsg).Result()
		}

		seq := perf.GetPerf().OnDeliverTxEnter(ctx, types.ModuleName, name)
		defer perf.GetPerf().OnDeliverTxExit(ctx, types.ModuleName, name, seq)
		return handlerFun()
	}
}

func handleMsgProvide(ctx sdk.Context, k keeper.Keeper, msg types.MsgProvide, logger log.Logger) sdk.Result {
	// 0.1 Check if the start_height_to_yield is more than current height
	if msg.StartHeightToYield <= ctx.BlockHeight() {
		return types.ErrInvalidStartHeight(DefaultCodespace).Result()
	}

	// 0.2 Get farm pool
	pool, found := k.GetFarmPool(ctx, msg.PoolName)
	if !found {
		return types.ErrNoFarmPoolFound(DefaultCodespace, msg.PoolName).Result()
	}

	// 0.3 Check if the provided coin denom is the same as the locked coin name
	if len(pool.YieldedTokenInfos) != 1 { // TODO: use the panic temporarily
		panic(fmt.Sprintf("The YieldedTokenInfos length is %d, which should be 1 in current code version",
			len(pool.YieldedTokenInfos)))
	}
	if pool.YieldedTokenInfos[0].RemainingAmount.Denom != msg.Amount.Denom {
		return types.ErrInvalidDenom(
			DefaultCodespace, pool.YieldedTokenInfos[0].RemainingAmount.Denom, msg.Amount.Denom).Result()
	}

	// 1.1 Calculate how many provided token has been yielded between start_block_height and current_height
	currentPeriod := k.GetPoolCurrentRewards(ctx, msg.PoolName)
	updatedPool, yieldedTokens := keeper.CalculateAmountYieldedBetween(ctx.BlockHeight(), currentPeriod.StartBlockHeight, pool)

	// 1.2 Check if remaining amount is already zero
	remainingAmount := updatedPool.YieldedTokenInfos[0].RemainingAmount
	if !remainingAmount.IsZero() {
		return types.ErrRemainingAmountNotZero(DefaultCodespace, remainingAmount.String()).Result()
	}

	// 2. Terminate pool current period
	k.IncrementPoolPeriod(ctx, pool.Name, pool.TotalValueLocked, yieldedTokens)

	// 3. Transfer coin to farm module account
	if err := k.SupplyKeeper().SendCoinsFromAccountToModule(ctx, msg.Address, YieldFarmingAccount, msg.Amount.ToCoins()); err != nil {
		return err.Result()
	}

	// 4. Update farm pool
	updatedPool.YieldedTokenInfos[0].StartBlockHeightToYield = msg.StartHeightToYield
	updatedPool.YieldedTokenInfos[0].AmountYieldedPerBlock = msg.AmountYieldedPerBlock
	updatedPool.YieldedTokenInfos[0].RemainingAmount = remainingAmount.Add(msg.Amount)
	k.SetFarmPool(ctx, updatedPool)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeProvide,
		sdk.NewAttribute(types.AttributeKeyAddress, msg.Address.String()),
		sdk.NewAttribute(types.AttributeKeyPool, msg.PoolName),
		sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.String()),
		sdk.NewAttribute(types.AttributeKeyStartHeightToYield, strconv.FormatInt(msg.StartHeightToYield, 10)),
		sdk.NewAttribute(types.AttributeKeyAmountYieldPerBlock, msg.AmountYieldedPerBlock.String()),
	))
	return sdk.Result{Events: ctx.EventManager().Events()}
}

func handleMsgClaim(ctx sdk.Context, k keeper.Keeper, msg types.MsgClaim, logger log.Logger) sdk.Result {
	// 1 Get the pool info
	pool, poolFound := k.GetFarmPool(ctx, msg.PoolName)
	if !poolFound {
		return types.ErrNoFarmPoolFound(DefaultCodespace, msg.PoolName).Result()
	}

	// 1.1 calcualte the yielded tokens at first
	currentPeriod := k.GetPoolCurrentRewards(ctx, msg.PoolName)
	updatedPool, yieldedTokens := keeper.CalculateAmountYieldedBetween(ctx.BlockHeight(), currentPeriod.StartBlockHeight, pool)

	// 2. Withdraw rewards
	_, err := k.WithdrawRewards(ctx, pool.Name, pool.TotalValueLocked, yieldedTokens, msg.Address)
	if err != nil {
		return err.Result()
	}

	// 3. Update the lock_info data
	k.UpdateLockInfo(ctx, msg.Address, pool.Name, sdk.ZeroDec())

	// 4. Update farm pool
	k.SetFarmPool(ctx, updatedPool)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeClaim,
		sdk.NewAttribute(types.AttributeKeyAddress, msg.Address.String()),
		sdk.NewAttribute(types.AttributeKeyPool, msg.PoolName),
	))
	return sdk.Result{Events: ctx.EventManager().Events()}
}

func handleMsgSetWhite(ctx sdk.Context, k keeper.Keeper, msg types.MsgSetWhite, logger log.Logger) sdk.Result {
	if _, found := k.GetFarmPool(ctx, msg.PoolName); !found {
		return types.ErrNoFarmPoolFound(DefaultCodespace, msg.PoolName).Result()
	}

	k.SetWhitelist(ctx, msg.PoolName)

	return sdk.Result{Events: sdk.Events{
		sdk.NewEvent(
			types.EventTypeCreatePool,
			sdk.NewAttribute(types.AttributeKeyPool, msg.PoolName),
		),
	}}
}
