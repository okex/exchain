package staking

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/okex/okchain/x/staking/keeper"
	"github.com/okex/okchain/x/staking/types"
	abci "github.com/tendermint/tendermint/abci/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

// NewHandler manages all tx treatment
func NewHandler(k keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		ctx = ctx.WithEventManager(sdk.NewEventManager())

		switch msg := msg.(type) {
		case types.MsgCreateValidator:
			return handleMsgCreateValidator(ctx, msg, k)
		case types.MsgEditValidator:
			return handleMsgEditValidator(ctx, msg, k)
		case types.MsgDeposit:
			return handleMsgDeposit(ctx, msg, k)
		case types.MsgWithdraw:
			return handleMsgWithdraw(ctx, msg, k)
		case types.MsgAddShares:
			return handleMsgAddShares(ctx, msg, k)
		case types.MsgBindProxy:
			return handleMsgBindProxy(ctx, msg, k)
		case types.MsgUnbindProxy:
			return handleMsgUnbindProxy(ctx, msg, k)
		case types.MsgRegProxy:
			return handleRegProxy(ctx, msg, k)
		case types.MsgDestroyValidator:
			return handleMsgDestroyValidator(ctx, msg, k)
		default:
			errMsg := fmt.Sprintf("unrecognized staking message type: %T", msg)
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

// EndBlocker is called every block, update validator set
func EndBlocker(ctx sdk.Context, k keeper.Keeper) []abci.ValidatorUpdate {
	// calculate validator set changes
	validatorUpdates := make([]abci.ValidatorUpdate, 0)
	if k.IsEndOfEpoch(ctx) {
		oldEpoch, newEpoch := k.GetEpoch(ctx), k.ParamsEpoch(ctx)
		if oldEpoch != newEpoch {
			k.SetEpoch(ctx, newEpoch)
		}
		k.SetTheEndOfLastEpoch(ctx)
		//ctx.Logger().Debug("validatorUpdates epoch", "old", oldEpoch, "new", newEpoch)
		//ctx.Logger().Debug(fmt.Sprintf("old epoch end blockHeight: %d", lastEpochEndHeight))

		validatorUpdates = k.ApplyAndReturnValidatorSetUpdates(ctx)
		// dont forget to delete in case that some validator need to kick out when an epoch ends
		k.DeleteAbandonedValidatorAddrs(ctx)
	} else if k.IsKickedOut(ctx) {
		// if there are some validators to kick out in an epoch
		validatorUpdates = k.KickOutAndReturnValidatorSetUpdates(ctx)
		k.DeleteAbandonedValidatorAddrs(ctx)
	}

	// Unbond all mature validators from the unbonding queue.
	k.UnbondAllMatureValidatorQueue(ctx)

	k.IterateKeysBeforeCurrentTime(ctx, ctx.BlockHeader().Time,
		func(index int64, key []byte) (stop bool) {
			oldTime, delAddr := types.SplitCompleteTimeWithAddrKey(key)
			k.DeleteAddrByTimeKey(ctx, oldTime, delAddr)

			quantity, err := k.CompleteUndelegation(ctx, delAddr)
			if err != nil {
				ctx.Logger().Error(fmt.Sprintf("complete withdraw failed: %s", err.Result().Data))
			} else {
				ctx.EventManager().EmitEvent(
					sdk.NewEvent(
						types.EventTypeCompleteUnbonding,
						sdk.NewAttribute(types.AttributeKeyDelegator, delAddr.String()),
						sdk.NewAttribute(sdk.AttributeKeyAmount, quantity.String()),
					),
				)
			}
			return false
		})

	return validatorUpdates
}

// These functions assumes everything has been authenticated, now we just perform action and save
func handleMsgCreateValidator(ctx sdk.Context, msg types.MsgCreateValidator, k keeper.Keeper) sdk.Result {
	if _, found := k.GetValidator(ctx, msg.ValidatorAddress); found {
		return ErrValidatorOwnerExists(k.Codespace()).Result()
	}
	if _, found := k.GetValidatorByConsAddr(ctx, sdk.GetConsAddress(msg.PubKey)); found {
		return ErrValidatorPubKeyExists(k.Codespace()).Result()
	}
	if msg.MinSelfDelegation.Denom != k.BondDenom(ctx) {
		return ErrBadDenom(k.Codespace()).Result()
	}

	if _, err := msg.Description.EnsureLength(); err != nil {
		return err.Result()
	}
	if ctx.ConsensusParams() != nil {
		tmPubKey := tmtypes.TM2PB.PubKey(msg.PubKey)
		if !common.StringInSlice(tmPubKey.Type, ctx.ConsensusParams().Validator.PubKeyTypes) {
			return ErrValidatorPubKeyTypeNotSupported(k.Codespace(), tmPubKey.Type,
				ctx.ConsensusParams().Validator.PubKeyTypes).Result()
		}
	}

	minSelfDelegation := k.ParamsMinSelfDelegation(ctx)
	validator := NewValidator(msg.ValidatorAddress, msg.PubKey, msg.Description, minSelfDelegation)
	commission := NewCommission(sdk.NewDec(1), sdk.NewDec(1), sdk.NewDec(0))
	validator, err := validator.SetInitialCommission(commission)
	if err != nil {
		return err.Result()
	}
	k.SetValidator(ctx, validator)
	k.SetValidatorByConsAddr(ctx, validator)
	k.SetNewValidatorByPowerIndex(ctx, validator)
	// add shares of equal value of msd for validator itself
	defaultMinSelfDelegationToken := sdk.NewDecCoinFromDec(k.BondDenom(ctx), validator.MinSelfDelegation)
	if err = k.AddSharesAsMinSelfDelegation(ctx, msg.DelegatorAddress, &validator, defaultMinSelfDelegationToken); err != nil {
		return err.Result()
	}
	k.AfterValidatorCreated(ctx, validator.OperatorAddress)
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeCreateValidator,
			sdk.NewAttribute(types.AttributeKeyValidator, msg.ValidatorAddress.String()),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.MinSelfDelegation.Amount.String())),
		sdk.NewEvent(sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.DelegatorAddress.String())),
	})
	return sdk.Result{Events: ctx.EventManager().Events()}
}

func handleMsgEditValidator(ctx sdk.Context, msg types.MsgEditValidator, k keeper.Keeper) sdk.Result {
	// validator must already be registered
	validator, found := k.GetValidator(ctx, msg.ValidatorAddress)
	if !found {
		return ErrNoValidatorFound(k.Codespace(), msg.ValidatorAddress.String()).Result()
	}

	// replace all editable fields (clients should autofill existing values)
	description, err := validator.Description.UpdateDescription(msg.Description)
	if err != nil {
		return err.Result()
	}

	validator.Description = description

	k.SetValidator(ctx, validator)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeEditValidator,
			sdk.NewAttribute(types.AttributeKeyMinSelfDelegation, validator.MinSelfDelegation.String()),
		),
		sdk.NewEvent(sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.ValidatorAddress.String()),
		),
	})

	return sdk.Result{Events: ctx.EventManager().Events()}
}
