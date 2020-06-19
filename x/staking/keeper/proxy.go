package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/okex/okchain/x/staking/types"
)

// ClearProxy clears the ProxyAddress on binded delegators
func (k Keeper) ClearProxy(ctx sdk.Context, proxyAddr sdk.AccAddress) {
	k.IterateProxy(ctx, proxyAddr, true, func(_ int64, delAddr, _ sdk.AccAddress) (stop bool) {
		delegator, found := k.GetDelegator(ctx, delAddr)
		if found {
			delegator.UnbindProxy()
			k.SetDelegator(ctx, delegator)
		}
		return false
	})
}

// SetProxyBinding sets or deletes the key of proxy relationship
func (k Keeper) SetProxyBinding(ctx sdk.Context, proxyAddress, delegatorAddress sdk.AccAddress, isRemove bool) {
	store := ctx.KVStore(k.storeKey)
	key := types.GetProxyDelegatorKey(proxyAddress, delegatorAddress)

	if isRemove {
		store.Delete(key)
	} else {
		store.Set(key, []byte(""))
	}
}

// IterateProxy iterates all the info between delegator and its proxy
func (k Keeper) IterateProxy(ctx sdk.Context, proxyAddr sdk.AccAddress, isClear bool,
	fn func(index int64, delAddr, proxyAddr sdk.AccAddress) (stop bool)) {
	store := ctx.KVStore(k.storeKey)
	iterator := sdk.KVStorePrefixIterator(store, types.GetProxyDelegatorKey(proxyAddr, []byte{}))
	defer iterator.Close()

	index := sdk.AddrLen + 1
	for i := int64(0); iterator.Valid(); iterator.Next() {
		key := iterator.Key()
		if stop := fn(i, key[index:], key[1:index]); stop {
			break
		}
		if isClear {
			store.Delete(key)
		}
		i++
	}
}

// UpdateVotes withdraws and votes continuously on the same validator set with different amount of votes
func (k Keeper) UpdateVotes(ctx sdk.Context, delAddr sdk.AccAddress, tokens sdk.Dec) sdk.Error {
	// get last validators voted existed in the store
	vals, lastVotes := k.GetLastValsVotedExisted(ctx, delAddr)
	if vals == nil {
		// if the delegator never votes, just pass
		return nil
	}

	lenVals := len(vals)
	votes, sdkErr := calculateWeight(ctx.BlockTime().Unix(), tokens)
	if sdkErr != nil {
		return sdkErr
	}

	delegator, found := k.GetDelegator(ctx, delAddr)
	if !found {
		return types.ErrNoDelegatorExisted(types.DefaultCodespace, delAddr.String())
	}

	logger := k.Logger(ctx)
	for i := 0; i < lenVals; i++ {
		if vals[i].MinSelfDelegation.IsZero() {
			return types.ErrVoteDismission(types.DefaultCodespace, vals[i].OperatorAddress.String())
		}

		// 1.delete related store
		k.DeleteValidatorByPowerIndex(ctx, vals[i])

		// 2.update vote
		k.SetVote(ctx, delAddr, vals[i].OperatorAddress, votes)

		// 3.update validator
		vals[i].DelegatorShares = vals[i].DelegatorShares.Sub(lastVotes).Add(votes)
		logger.Debug("update vote", vals[i].OperatorAddress.String(), vals[i].DelegatorShares.String())
		k.SetValidator(ctx, vals[i])
		k.SetValidatorByPowerIndex(ctx, vals[i])
	}

	// update the delegator struct
	delegator.Shares = votes
	k.SetDelegator(ctx, delegator)

	return nil
}

// VoteValidators votes to validators and return the votes
func (k Keeper) VoteValidators(ctx sdk.Context, delAddr sdk.AccAddress, vals types.Validators, tokens sdk.Dec) (sdk.Dec,
	sdk.Error) {
	lenVals := len(vals)
	votes, sdkErr := calculateWeight(ctx.BlockTime().Unix(), tokens)
	if sdkErr != nil {
		return sdk.Dec{}, sdkErr
	}
	for i := 0; i < lenVals; i++ {
		k.vote(ctx, delAddr, vals[i], votes)
	}
	return votes, nil
}

// WithdrawLastVotes withdraws the vote last time from the validators
func (k Keeper) WithdrawLastVotes(ctx sdk.Context, delAddr sdk.AccAddress, lastValsVoted types.Validators,
	lastVotes sdk.Dec) {
	lenLastVals := len(lastValsVoted)
	for i := 0; i < lenLastVals; i++ {
		k.withdrawVote(ctx, delAddr, lastValsVoted[i], lastVotes)
	}
}

func (k Keeper) withdrawVote(ctx sdk.Context, voterAddr sdk.AccAddress, val types.Validator, votes sdk.Dec) {
	// 1.delete vote entity
	k.DeleteVote(ctx, val.OperatorAddress, voterAddr)

	// 2.update validator entity
	k.DeleteValidatorByPowerIndex(ctx, val)

	// 3.update validator's votes
	val.DelegatorShares = val.GetDelegatorShares().Sub(votes)

	// 3.check whether the validator should be removed
	if val.IsUnbonded() && val.GetMinSelfDelegation().IsZero() && val.GetDelegatorShares().IsZero() {
		k.RemoveValidator(ctx, val.OperatorAddress)
		return
	}

	k.SetValidator(ctx, val)
	k.SetValidatorByPowerIndex(ctx, val)
}

func (k Keeper) vote(ctx sdk.Context, voterAddr sdk.AccAddress, val types.Validator, votes types.Votes) {
	// 1.update vote entity
	k.SetVote(ctx, voterAddr, val.OperatorAddress, votes)

	// 2.update validator entity
	k.DeleteValidatorByPowerIndex(ctx, val)
	val.DelegatorShares = val.GetDelegatorShares().Add(votes)
	k.SetValidator(ctx, val)
	k.SetValidatorByPowerIndex(ctx, val)
}

// GetLastValsVotedExisted gets last validators that the voter voted last time
func (k Keeper) GetLastValsVotedExisted(ctx sdk.Context, voterAddr sdk.AccAddress) (types.Validators, sdk.Dec) {
	// 1.get delegator entity
	delegator, found := k.GetDelegator(ctx, voterAddr)

	// if not found
	if !found {
		return nil, sdk.ZeroDec()
	}

	// 2.get validators voted existed in the store
	lenVals := len(delegator.ValidatorAddresses)
	var vals types.Validators
	for i := 0; i < lenVals; i++ {
		val, found := k.GetValidator(ctx, delegator.ValidatorAddresses[i])
		if found {
			// the validator voted hasn't been removed
			vals = append(vals, val)
		}
	}

	return vals, delegator.Shares
}

// GetValidatorsToVote gets the validators from their validator addresses
func (k Keeper) GetValidatorsToVote(ctx sdk.Context, valAddrs []sdk.ValAddress) (types.Validators, sdk.Error) {
	lenVals := len(valAddrs)
	vals := make(types.Validators, lenVals)
	for i := 0; i < lenVals; i++ {
		val, found := k.GetValidator(ctx, valAddrs[i])
		if found {
			// get the validator hasn't been removed
			vals[i] = val
		} else {
			return nil, types.ErrNoValidatorFound(types.DefaultCodespace, valAddrs[i].String())
		}
	}

	return vals, nil
}
