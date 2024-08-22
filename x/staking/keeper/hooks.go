package keeper

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/x/staking/types"
)

// Implements StakingHooks interface
var _ types.StakingHooks = Keeper{}

// AfterValidatorCreated - call hook if registered
func (k Keeper) AfterValidatorCreated(ctx sdk.Context, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.AfterValidatorCreated(ctx, valAddr)
	}
}

func (k Keeper) AfterValidatorPubkeyChanged(ctx sdk.Context, oldAddress sdk.ConsAddress, newAddress sdk.ConsAddress, newPubkey crypto.PubKey) {
	if k.hooks != nil {
		k.hooks.AfterValidatorPubkeyChanged(ctx, oldAddress, newAddress, newPubkey)
	}
}

// BeforeValidatorModified - call hook if registered
func (k Keeper) BeforeValidatorModified(ctx sdk.Context, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.BeforeValidatorModified(ctx, valAddr)
	}
}

// AfterValidatorRemoved - call hook if registered
func (k Keeper) AfterValidatorRemoved(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.AfterValidatorRemoved(ctx, consAddr, valAddr)
	}
}

// AfterValidatorBonded - call hook if registered
func (k Keeper) AfterValidatorBonded(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.AfterValidatorBonded(ctx, consAddr, valAddr)
	}
}

// AfterValidatorBeginUnbonding - call hook if registered
func (k Keeper) AfterValidatorBeginUnbonding(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.AfterValidatorBeginUnbonding(ctx, consAddr, valAddr)
	}
}

// AfterValidatorDestroyed - call hook if registered
func (k Keeper) AfterValidatorDestroyed(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {
	if k.hooks != nil {
		k.hooks.AfterValidatorDestroyed(ctx, consAddr, valAddr)
	}
}
