package keeper

import (
	"bytes"
	"fmt"
	"github.com/okex/exchain/libs/tendermint/crypto"
	cryptoAmino "github.com/okex/exchain/libs/tendermint/crypto/encoding/amino"
	"time"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/staking/types"
)

// GetValidator gets a single validator
func (k Keeper) GetValidator(ctx sdk.Context, addr sdk.ValAddress) (validator types.Validator, found bool) {
	store := ctx.KVStore(k.storeKey)
	value := store.Get(types.GetValidatorKey(addr))
	if value == nil {
		return validator, false
	}
	validator = types.MustUnmarshalValidator(k.cdcMarshl.GetCdc(), value)
	return validator, true
}

func (k Keeper) mustGetValidator(ctx sdk.Context, addr sdk.ValAddress) types.Validator {
	validator, found := k.GetValidator(ctx, addr)
	if !found {
		panic(fmt.Sprintf("validator record not found for address: %X\n", addr))
	}
	return validator
}

// GetValidatorByConsAddr gets a single validator by consensus address
func (k Keeper) GetValidatorByConsAddr(ctx sdk.Context, consAddr sdk.ConsAddress) (validator types.Validator,
	found bool) {
	store := ctx.KVStore(k.storeKey)
	opAddr := store.Get(types.GetValidatorByConsAddrKey(consAddr))
	if opAddr == nil {
		return validator, false
	}
	return k.GetValidator(ctx, opAddr)
}

func (k Keeper) mustGetValidatorByConsAddr(ctx sdk.Context, consAddr sdk.ConsAddress) types.Validator {
	validator, found := k.GetValidatorByConsAddr(ctx, consAddr)
	if !found {
		panic(fmt.Errorf("validator with consensus-Address %s not found", consAddr))
	}
	return validator
}

// SetValidator sets the main record holding validator details
func (k Keeper) SetValidator(ctx sdk.Context, validator types.Validator) {
	store := ctx.KVStore(k.storeKey)
	bz := types.MustMarshalValidator(k.cdcMarshl.GetCdc(), validator)
	store.Set(types.GetValidatorKey(validator.OperatorAddress), bz)
}

func (k Keeper) SetChangePubkey(ctx sdk.Context, addr sdk.ValAddress, pubkey crypto.PubKey) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetValidatorChangePubkeyKey(addr), pubkey.Bytes())
}

func (k Keeper) DeleteChangePubkey(ctx sdk.Context, addr sdk.ValAddress) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetValidatorChangePubkeyKey(addr))
}

func (k Keeper) GetChangePubkey(ctx sdk.Context, addr sdk.ValAddress) (pubkey crypto.PubKey, found bool) {
	store := ctx.KVStore(k.storeKey)
	value := store.Get(types.GetValidatorChangePubkeyKey(addr))
	if value == nil {
		return nil, false
	}
	pk, err := cryptoAmino.PubKeyFromBytes(value)
	if err != nil {
		panic(err)
	}
	return pk, true
}

// SetValidatorByConsAddr sets the operator address with the key of validator consensus pubkey
func (k Keeper) SetValidatorByConsAddr(ctx sdk.Context, validator types.Validator) {
	store := ctx.KVStore(k.storeKey)
	consAddr := sdk.ConsAddress(validator.ConsPubKey.Address())
	store.Set(types.GetValidatorByConsAddrKey(consAddr), validator.OperatorAddress)
}

func (k Keeper) DeleteValidatorByConsAddr(ctx sdk.Context, consAddr sdk.ConsAddress) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetValidatorByConsAddrKey(consAddr))
}

// SetValidatorByPowerIndex sets the power index key of an unjailed validator
func (k Keeper) SetValidatorByPowerIndex(ctx sdk.Context, validator types.Validator) {
	// jailed validators are not kept in the power index
	if validator.Jailed {
		return
	}
	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetValidatorsByPowerIndexKey(validator), validator.OperatorAddress)
}

// DeleteValidatorByPowerIndex deletes the power index key
func (k Keeper) DeleteValidatorByPowerIndex(ctx sdk.Context, validator types.Validator) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetValidatorsByPowerIndexKey(validator))
}

// SetNewValidatorByPowerIndex sets the power index key of a validator
func (k Keeper) SetNewValidatorByPowerIndex(ctx sdk.Context, validator types.Validator) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetValidatorsByPowerIndexKey(validator), validator.OperatorAddress)
}

// RemoveValidator removes the validator record and associated indexes
// except for the bonded validator index which is only handled in ApplyAndReturnTendermintUpdates
func (k Keeper) RemoveValidator(ctx sdk.Context, address sdk.ValAddress) {
	k.Logger(ctx).Debug("Remove Validator", "ValAddr", address.String())

	// first retrieve the old validator record
	validator, found := k.GetValidator(ctx, address)
	if !found {
		return
	}

	if !validator.IsUnbonded() {
		panic("cannot call RemoveValidator on bonded or unbonding validators")
	}
	if validator.Tokens.IsPositive() {
		panic("attempting to remove a validator which still contains tokens")
	}
	if validator.Tokens.GT(sdk.ZeroInt()) {
		panic("validator being removed should never have positive tokens")
	}

	// delete the old validator record
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetValidatorKey(address))
	store.Delete(types.GetValidatorByConsAddrKey(sdk.ConsAddress(validator.ConsPubKey.Address())))
	store.Delete(types.GetValidatorsByPowerIndexKey(validator))

	// call hooks
	k.AfterValidatorRemoved(ctx, validator.ConsAddress(), validator.OperatorAddress)
}

// get groups of validators

// GetAllValidators gets the set of all validators with no limits, used during genesis dump
func (k Keeper) GetAllValidators(ctx sdk.Context) (validators types.Validators) {
	store := ctx.KVStore(k.storeKey)
	iterator := sdk.KVStorePrefixIterator(store, types.ValidatorsKey)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		validator := types.MustUnmarshalValidator(k.cdcMarshl.GetCdc(), iterator.Value())
		validators = append(validators, validator)
	}
	return validators
}

// ValidatorsPowerStoreIterator returns an iterator for the current validator power store
func (k Keeper) ValidatorsPowerStoreIterator(ctx sdk.Context) (iterator sdk.Iterator) {
	store := ctx.KVStore(k.storeKey)
	iterator = sdk.KVStoreReversePrefixIterator(store, types.ValidatorsByPowerIndexKey)
	return iterator
}

//_______________________________________________________________________
// Last Validator Index

// GetLastValidatorPower loads the last validator power and returns zero if the operator was not a validator last block
func (k Keeper) GetLastValidatorPower(ctx sdk.Context, operator sdk.ValAddress) (power int64) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.GetLastValidatorPowerKey(operator))
	if bz == nil {
		return 0
	}
	k.cdcMarshl.GetCdc().MustUnmarshalBinaryLengthPrefixed(bz, &power)
	return
}

// SetLastValidatorPower sets the last validator power
func (k Keeper) SetLastValidatorPower(ctx sdk.Context, operator sdk.ValAddress, power int64) {
	store := ctx.KVStore(k.storeKey)
	bz := k.cdcMarshl.GetCdc().MustMarshalBinaryLengthPrefixed(power)
	store.Set(types.GetLastValidatorPowerKey(operator), bz)
}

// DeleteLastValidatorPower deletes the last validator power
func (k Keeper) DeleteLastValidatorPower(ctx sdk.Context, operator sdk.ValAddress) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetLastValidatorPowerKey(operator))
}

// LastValidatorsIterator returns an iterator for the consensus validators in the last block
func (k Keeper) LastValidatorsIterator(ctx sdk.Context) (iterator sdk.Iterator) {
	store := ctx.KVStore(k.storeKey)
	iterator = sdk.KVStorePrefixIterator(store, types.LastValidatorPowerKey)
	return iterator
}

// IterateLastValidatorPowers iterates over last validator powers
func (k Keeper) IterateLastValidatorPowers(ctx sdk.Context,
	handler func(operator sdk.ValAddress, power int64) (stop bool)) {
	store := ctx.KVStore(k.storeKey)
	iter := sdk.KVStorePrefixIterator(store, types.LastValidatorPowerKey)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		addr := sdk.ValAddress(iter.Key()[len(types.LastValidatorPowerKey):])
		var power int64
		k.cdcMarshl.GetCdc().MustUnmarshalBinaryLengthPrefixed(iter.Value(), &power)
		if handler(addr, power) {
			break
		}
	}
}

//_______________________________________________________________________
// Validator Queue

// GetValidatorQueueTimeSlice gets a specific validator queue timeslice
// A timeslice is a slice of ValAddresses corresponding to unbonding validators that expire at a certain time
func (k Keeper) GetValidatorQueueTimeSlice(ctx sdk.Context, timestamp time.Time) (valAddrs []sdk.ValAddress) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.GetValidatorQueueTimeKey(timestamp))
	if bz == nil {
		return []sdk.ValAddress{}
	}
	k.cdcMarshl.GetCdc().MustUnmarshalBinaryLengthPrefixed(bz, &valAddrs)
	return valAddrs
}

// SetValidatorQueueTimeSlice sets a specific validator queue timeslice
func (k Keeper) SetValidatorQueueTimeSlice(ctx sdk.Context, timestamp time.Time, keys []sdk.ValAddress) {
	store := ctx.KVStore(k.storeKey)
	bz := k.cdcMarshl.GetCdc().MustMarshalBinaryLengthPrefixed(keys)
	store.Set(types.GetValidatorQueueTimeKey(timestamp), bz)
}

// DeleteValidatorQueueTimeSlice deletes a specific validator queue timeslice
func (k Keeper) DeleteValidatorQueueTimeSlice(ctx sdk.Context, timestamp time.Time) {
	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetValidatorQueueTimeKey(timestamp))
}

// InsertValidatorQueue inserts an validator address to the appropriate timeslice in the validator queue
func (k Keeper) InsertValidatorQueue(ctx sdk.Context, val types.Validator) {
	timeSlice := k.GetValidatorQueueTimeSlice(ctx, val.UnbondingCompletionTime)
	var keys []sdk.ValAddress
	if len(timeSlice) == 0 {
		keys = []sdk.ValAddress{val.OperatorAddress}
	} else {
		keys = append(timeSlice, val.OperatorAddress)
	}
	k.SetValidatorQueueTimeSlice(ctx, val.UnbondingCompletionTime, keys)
}

// DeleteValidatorQueue deletes a validator address from the validator queue
func (k Keeper) DeleteValidatorQueue(ctx sdk.Context, val types.Validator) {
	timeSlice := k.GetValidatorQueueTimeSlice(ctx, val.UnbondingCompletionTime)
	newTimeSlice := []sdk.ValAddress{}
	for _, addr := range timeSlice {
		if !bytes.Equal(addr, val.OperatorAddress) {
			newTimeSlice = append(newTimeSlice, addr)
		}
	}
	if len(newTimeSlice) == 0 {
		k.DeleteValidatorQueueTimeSlice(ctx, val.UnbondingCompletionTime)
	} else {
		k.SetValidatorQueueTimeSlice(ctx, val.UnbondingCompletionTime, newTimeSlice)
	}
}

// ValidatorQueueIterator returns all the validator queue timeslices from time 0 until endTime
func (k Keeper) ValidatorQueueIterator(ctx sdk.Context, endTime time.Time) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return store.Iterator(types.ValidatorQueueKey, sdk.InclusiveEndBytes(types.GetValidatorQueueTimeKey(endTime)))
}

// UnbondAllMatureValidatorQueue unbonds all the unbonding validators that have finished their unbonding period
func (k Keeper) UnbondAllMatureValidatorQueue(ctx sdk.Context) {
	store := ctx.KVStore(k.storeKey)
	validatorTimesliceIterator := k.ValidatorQueueIterator(ctx, ctx.BlockHeader().Time)
	defer validatorTimesliceIterator.Close()

	for ; validatorTimesliceIterator.Valid(); validatorTimesliceIterator.Next() {
		timeslice := []sdk.ValAddress{}
		k.cdcMarshl.GetCdc().MustUnmarshalBinaryLengthPrefixed(validatorTimesliceIterator.Value(), &timeslice)

		for _, valAddr := range timeslice {
			val, found := k.GetValidator(ctx, valAddr)
			if !found {
				panic("validator in the unbonding queue was not found")
			}

			if !val.IsUnbonding() {
				panic("unexpected validator in unbonding queue; status was not unbonding")
			}
			val = k.unbondingToUnbonded(ctx, val)
			// required by okexchain
			//if val.GetDelegatorShares().IsZero() {
			//	k.RemoveValidator(ctx, val.OperatorAddress)
			//}
			if val.GetDelegatorShares().IsZero() && val.GetMinSelfDelegation().IsZero() {
				k.RemoveValidator(ctx, val.OperatorAddress)
			}
		}

		store.Delete(validatorTimesliceIterator.Key())
	}
}
