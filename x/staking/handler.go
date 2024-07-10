package staking

import (
	"fmt"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/okex/exchain/x/staking/keeper"
	"github.com/okex/exchain/x/staking/types"
)

// NewHandler manages all tx treatment
func NewHandler(k keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		ctx.SetEventManager(sdk.NewEventManager())
		errMsg := fmt.Sprintf("unrecognized staking message type: %T", msg)

		if tmtypes.HigherThanVenus2(ctx.BlockHeight()) && !k.CheckEnabled(ctx) {
			return nil, types.ErrCodeDisabledOperate()
		}

		switch msg := msg.(type) {
		case types.MsgCreateValidator:
			return handleMsgCreateValidator(ctx, msg, k)
		case types.MsgEditValidator:
			return handleMsgEditValidator(ctx, msg, k)
		case types.MsgEditValidatorCommissionRate:
			if tmtypes.HigherThanVenus2(ctx.BlockHeight()) {
				return handleMsgEditValidatorCommissionRate(ctx, msg, k)
			}
			return sdk.ErrUnknownRequest(errMsg).Result()
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
				ctx.Logger().Error(fmt.Sprintf("complete withdraw failed: %s", err))
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

// StringInSlice returns true if a is found the list.
func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// These functions assumes everything has been authenticated, now we just perform action and save
func handleMsgCreateValidator(ctx sdk.Context, msg types.MsgCreateValidator, k keeper.Keeper) (*sdk.Result, error) {
	if _, found := k.GetValidator(ctx, msg.ValidatorAddress); found {
		return nil, ErrValidatorOwnerExists()
	}
	if _, found := k.GetValidatorByConsAddr(ctx, sdk.GetConsAddress(msg.PubKey)); found {
		return nil, ErrValidatorPubKeyExists()
	}
	if msg.MinSelfDelegation.Denom != k.BondDenom(ctx) {
		return nil, ErrBadDenom()
	}

	if _, err := msg.Description.EnsureLength(); err != nil {
		return nil, err
	}
	if ctx.ConsensusParams() != nil {
		tmPubKey := tmtypes.TM2PB.PubKey(msg.PubKey)
		if !StringInSlice(tmPubKey.Type, ctx.ConsensusParams().Validator.PubKeyTypes) {
			return nil, ErrValidatorPubKeyTypeNotSupported(tmPubKey.Type,
				ctx.ConsensusParams().Validator.PubKeyTypes)
		}
	}

	minSelfDelegation := k.ParamsMinSelfDelegation(ctx)
	validator := NewValidator(msg.ValidatorAddress, msg.PubKey, msg.Description, minSelfDelegation)
	commission := NewCommission(sdk.NewDec(1), sdk.NewDec(1), sdk.NewDec(0))
	validator, err := validator.SetInitialCommission(commission)
	if err != nil {
		return nil, err
	}
	k.SetValidator(ctx, validator)
	k.SetValidatorByConsAddr(ctx, validator)
	k.SetNewValidatorByPowerIndex(ctx, validator)
	// add shares of equal value of msd for validator itself
	defaultMinSelfDelegationToken := sdk.NewDecCoinFromDec(k.BondDenom(ctx), validator.MinSelfDelegation)
	if err = k.AddSharesAsMinSelfDelegation(ctx, msg.DelegatorAddress, &validator, defaultMinSelfDelegationToken); err != nil {
		return nil, err
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
	return &sdk.Result{Events: ctx.EventManager().Events()}, nil
}

func handleMsgEditValidator(ctx sdk.Context, msg types.MsgEditValidator, k keeper.Keeper) (*sdk.Result, error) {
	// validator must already be registered
	validator, found := k.GetValidator(ctx, msg.ValidatorAddress)
	if !found {
		return nil, ErrNoValidatorFound(msg.ValidatorAddress.String())
	}

	pk, err := types.GetConsPubKeyBech32(msg.Details)
	if err == nil && tmtypes.HigherThanJupiter(ctx.BlockHeight()) {
		if validator.ConsPubKey.Equals(pk) {
			return nil, ErrPubkeyEqual(pk.Address().String())
		}
		if _, found := k.GetValidatorByConsAddr(ctx, sdk.GetConsAddress(pk)); found {
			return nil, ErrValidatorPubKeyExists()
		}
		if ctx.ConsensusParams() != nil {
			tmPubKey := tmtypes.TM2PB.PubKey(pk)
			if !StringInSlice(tmPubKey.Type, ctx.ConsensusParams().Validator.PubKeyTypes) {
				return nil, ErrValidatorPubKeyTypeNotSupported(tmPubKey.Type,
					ctx.ConsensusParams().Validator.PubKeyTypes)
			}
		}
		k.SetChangePubkey(ctx, validator.OperatorAddress, validator.GetConsPubKey())
		oldConsAddr := validator.GetConsAddr()

		validator.ConsPubKey = pk
		newConsAddr := validator.GetConsAddr()
		k.SetValidator(ctx, validator)
		k.SetValidatorByConsAddr(ctx, validator)
		k.DeleteValidatorByConsAddr(ctx, oldConsAddr)
		k.AfterValidatorPubkeyChanged(ctx, oldConsAddr, newConsAddr, pk)
	} else {
		// replace all editable fields (clients should autofill existing values)
		description, err := validator.Description.UpdateDescription(msg.Description)
		if err != nil {
			return nil, err
		}

		validator.Description = description
		k.SetValidator(ctx, validator)
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeEditValidator,
			sdk.NewAttribute(types.AttributeKeyMinSelfDelegation, validator.MinSelfDelegation.String()),
		),
		sdk.NewEvent(sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.ValidatorAddress.String()),
		),
	})

	return &sdk.Result{Events: ctx.EventManager().Events()}, nil
}
