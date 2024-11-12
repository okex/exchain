// nolint
package keeper

import (
	"time"

	"github.com/okex/exchain/libs/tendermint/crypto"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/slashing/internal/types"
)

func (k Keeper) AfterValidatorBonded(ctx sdk.Context, address sdk.ConsAddress, _ sdk.ValAddress) {
	// Update the signing info start height or create a new signing info
	_, found := k.GetValidatorSigningInfo(ctx, address)
	if !found {
		signingInfo := types.NewValidatorSigningInfo(
			address,
			ctx.BlockHeight(),
			0,
			time.Unix(0, 0),
			false,
			0,
			types.Created,
		)
		k.SetValidatorSigningInfo(ctx, address, signingInfo)
	}
}

// When a validator is created, add the address-pubkey relation.
func (k Keeper) AfterValidatorCreated(ctx sdk.Context, valAddr sdk.ValAddress) {
	validator := k.sk.Validator(ctx, valAddr)
	k.AddPubkey(ctx, validator.GetConsPubKey())
	k.modifyValidatorStatus(ctx, validator.GetConsAddr(), types.Created)
}

// When a validator is removed, delete the address-pubkey relation.
func (k Keeper) AfterValidatorRemoved(ctx sdk.Context, address sdk.ConsAddress) {
	k.deleteAddrPubkeyRelation(ctx, crypto.Address(address))
	k.modifyValidatorStatus(ctx, address, types.Destroyed)
}

func (k Keeper) AfterValidatorPubkeyChanged(ctx sdk.Context, oldAddress sdk.ConsAddress, newAddress sdk.ConsAddress, newPubkey crypto.PubKey) {
	k.deleteAddrPubkeyRelation(ctx, crypto.Address(oldAddress))
	signingInfo, found := k.GetValidatorSigningInfo(ctx, oldAddress)
	k.modifyValidatorStatus(ctx, oldAddress, types.Destroyed)

	k.AddPubkey(ctx, newPubkey)
	if found {
		k.SetValidatorSigningInfo(ctx, newAddress, signingInfo)
	}
}

func (k Keeper) AfterValidatorDestroyed(ctx sdk.Context, valAddr sdk.ValAddress) {
	validator := k.sk.Validator(ctx, valAddr)
	if validator != nil {
		k.modifyValidatorStatus(ctx, validator.GetConsAddr(), types.Destroying)
	}
}

//_________________________________________________________________________________________

// Hooks wrapper struct for slashing keeper
type Hooks struct {
	k Keeper
}

var _ types.StakingHooks = Hooks{}

// Return the wrapper struct
func (k Keeper) Hooks() Hooks {
	return Hooks{k}
}

// Implements sdk.ValidatorHooks
func (h Hooks) AfterValidatorBonded(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {
	h.k.AfterValidatorBonded(ctx, consAddr, valAddr)
}

// Implements sdk.ValidatorHooks
func (h Hooks) AfterValidatorRemoved(ctx sdk.Context, consAddr sdk.ConsAddress, _ sdk.ValAddress) {
	h.k.AfterValidatorRemoved(ctx, consAddr)
}

// Implements sdk.ValidatorHooks
func (h Hooks) AfterValidatorCreated(ctx sdk.Context, valAddr sdk.ValAddress) {
	h.k.AfterValidatorCreated(ctx, valAddr)
}

func (h Hooks) AfterValidatorPubkeyChanged(ctx sdk.Context, oldAddress sdk.ConsAddress, newAddress sdk.ConsAddress, newPubkey crypto.PubKey) {
	h.k.AfterValidatorPubkeyChanged(ctx, oldAddress, newAddress, newPubkey)
}
func (h Hooks) AfterValidatorDestroyed(ctx sdk.Context, _ sdk.ConsAddress, valAddr sdk.ValAddress) {
	h.k.AfterValidatorDestroyed(ctx, valAddr)
}

// nolint - unused hooks
func (h Hooks) AfterValidatorBeginUnbonding(_ sdk.Context, _ sdk.ConsAddress, _ sdk.ValAddress)    {}
func (h Hooks) BeforeValidatorModified(_ sdk.Context, _ sdk.ValAddress)                            {}
func (h Hooks) BeforeDelegationCreated(_ sdk.Context, _ sdk.AccAddress, _ []sdk.ValAddress)        {}
func (h Hooks) BeforeDelegationSharesModified(_ sdk.Context, _ sdk.AccAddress, _ []sdk.ValAddress) {}
func (h Hooks) BeforeDelegationRemoved(_ sdk.Context, _ sdk.AccAddress, _ sdk.ValAddress)          {}
func (h Hooks) AfterDelegationModified(_ sdk.Context, _ sdk.AccAddress, _ []sdk.ValAddress)        {}
func (h Hooks) BeforeValidatorSlashed(_ sdk.Context, _ sdk.ValAddress, _ sdk.Dec)                  {}
func (h Hooks) CheckEnabled(ctx sdk.Context) bool                                                  { return true }
