package keeper

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/tendermint/crypto"

	"github.com/okex/exchain/x/distribution/types"
	stakingtypes "github.com/okex/exchain/x/staking/types"
)

// Hooks is struct of keepers from other module
type Hooks struct {
	k Keeper
}

var _ stakingtypes.StakingHooks = Hooks{}

// Hooks creates new distribution hooks
func (k Keeper) Hooks() Hooks { return Hooks{k} }

// AfterValidatorCreated initializes validator distribution record
func (h Hooks) AfterValidatorCreated(ctx sdk.Context, valAddr sdk.ValAddress) {
	val := h.k.stakingKeeper.Validator(ctx, valAddr)
	h.k.initializeValidator(ctx, val)
}

func (h Hooks) AfterValidatorPubkeyChanged(ctx sdk.Context, oldAddress sdk.ConsAddress, newAddress sdk.ConsAddress, newPubkey crypto.PubKey) {
}

// AfterValidatorRemoved cleans up for after validator is removed
func (h Hooks) AfterValidatorRemoved(ctx sdk.Context, _ sdk.ConsAddress, valAddr sdk.ValAddress) {
	if h.k.CheckDistributionProposalValid(ctx) {
		h.afterValidatorRemovedForDistributionProposal(ctx, nil, valAddr)
		return
	}

	// force-withdraw commission
	commission := h.k.GetValidatorAccumulatedCommission(ctx, valAddr)
	if !commission.IsZero() {
		// split into integral & remainder
		coins, remainder := commission.TruncateDecimal()
		// remainder to community pool
		if !remainder.IsZero() {
			feePool := h.k.GetFeePool(ctx)
			feePool.CommunityPool = feePool.CommunityPool.Add(remainder...)
			h.k.SetFeePool(ctx, feePool)
		}
		// add to validator account
		if !coins.IsZero() {
			accAddr := sdk.AccAddress(valAddr)
			withdrawAddr := h.k.GetDelegatorWithdrawAddr(ctx, accAddr)
			err := h.k.supplyKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, withdrawAddr, coins)
			if err != nil {
				panic(err)
			}
		}
	}

	// remove commission record
	h.k.deleteValidatorAccumulatedCommission(ctx, valAddr)
}

// AfterValidatorDestroyed nothing to do
func (h Hooks) AfterValidatorDestroyed(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress) {

}

// nolint - unused hooks
func (h Hooks) BeforeValidatorModified(_ sdk.Context, _ sdk.ValAddress)                         {}
func (h Hooks) AfterValidatorBonded(_ sdk.Context, _ sdk.ConsAddress, _ sdk.ValAddress)         {}
func (h Hooks) AfterValidatorBeginUnbonding(_ sdk.Context, _ sdk.ConsAddress, _ sdk.ValAddress) {}
