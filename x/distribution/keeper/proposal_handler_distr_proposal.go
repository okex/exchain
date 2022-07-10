package keeper

import (
	"fmt"
	stakingexported "github.com/okex/exchain/x/staking/exported"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/distribution/types"
)

// HandleChangeDistributionTypeProposal is a handler for executing a passed change distribution type proposal
func HandleChangeDistributionTypeProposal(ctx sdk.Context, k Keeper, p types.ChangeDistributionTypeProposal) error {
	logger := k.Logger(ctx)

	//1.check if it's the same
	if k.GetDistributionType(ctx) == p.Type {
		logger.Debug(fmt.Sprintf("do nothing, same distribution type, %d", p.Type))
		return nil
	}

	//2. if on chain, iteration validators and init val which has not outstanding
	if p.Type == types.DistributionTypeOnChain {
		if !k.CheckInitExistedValidatorFlag(ctx) {
			k.SetInitExistedValidatorFlag(ctx, true)
			k.stakingKeeper.IterateValidators(ctx, func(index int64, validator stakingexported.ValidatorI) (stop bool) {
				if validator != nil {
					k.initExistedValidatorForDistrProposal(ctx, validator)
				}
				return false
			})
		}
	}

	//3. set it
	k.SetDistributionType(ctx, p.Type)

	return nil
}
