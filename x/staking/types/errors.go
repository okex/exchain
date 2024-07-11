// nolint
package types

import (
	"fmt"
	"strings"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
)

const (
	DefaultCodespace string = ModuleName

	CodeNoValidatorFound                uint32 = 67000
	CodeInvalidDelegation               uint32 = 67001
	CodeNilValidatorAddr                uint32 = 67002
	CodeBadValidatorAddr                uint32 = 67003
	CodeMoreMinSelfDelegation           uint32 = 67004
	CodeProxyNotFound                   uint32 = 67005
	CodeEmptyValidators                 uint32 = 67006
	CodeProxyAlreadyExist               uint32 = 67007
	CodeAddressNotEqual                 uint32 = 67009
	CodeDescriptionIsEmpty              uint32 = 67010
	CodeGetConsPubKeyBech32Failed       uint32 = 67011
	CodeUnknownStakingQueryType         uint32 = 67012
	CodeValidatorOwnerExists            uint32 = 67013
	CodeValidatorPubKeyExists           uint32 = 67014
	CodeValidatorPubKeyTypeNotSupported uint32 = 67015
	CodeBondedPoolOrNotBondedIsNotExist uint32 = 67016
	CodeInvalidDescriptionLength        uint32 = 67017
	CodeCommissionNegative              uint32 = 67018
	CodeCommissionHuge                  uint32 = 67019
	CodeCommissionGTMaxRate             uint32 = 67020
	CodeCommissionUpdateTime            uint32 = 67021
	CodeCommissionChangeRateNegative    uint32 = 67022
	CodeCommissionChangeRateGTMaxRate   uint32 = 67023
	CodeCommissionGTMaxChangeRate       uint32 = 67024
	CodeMinSelfDelegationInvalid        uint32 = 67025
	CodeNilDelegatorAddr                uint32 = 67026
	CodeDelegatorEqualToProxyAddr       uint32 = 67027
	CodeBadDenom                        uint32 = 67028
	CodeBadDelegationAmount             uint32 = 67029
	CodeNoUnbondingDelegation           uint32 = 67030
	CodeAddSharesToDismission           uint32 = 67031
	CodeAddSharesDuringProxy            uint32 = 67032
	CodeDoubleProxy                     uint32 = 67033
	CodeExceedValidatorAddrs            uint32 = 67034
	CodeNoDelegationToAddShares         uint32 = 67035
	CodeNotInDelegating                 uint32 = 67036
	CodeInsufficientDelegation          uint32 = 67037
	CodeInsufficientQuantity            uint32 = 67038
	CodeInvalidMinSelfDelegation        uint32 = 67039
	CodeBadUnDelegationAmount           uint32 = 67040
	CodeInvalidProxyUpdating            uint32 = 67041
	CodeInvalidProxyWithdrawTotal       uint32 = 67042
	CodeAlreadyAddedShares              uint32 = 67043
	CodeNoDelegatorExisted              uint32 = 67044
	CodeTargetValsDuplicate             uint32 = 67045
	CodeAlreadyBound                    uint32 = 67046

	CodePubkeyEqual uint32 = 67047
)

var (
	ErrInvalidHistoricalInfo = sdkerrors.Register(ModuleName, 144, "invalid historical info")
	ErrNoHistoricalInfo      = sdkerrors.Register(ModuleName, 145, "no historical info found")
)

// ErrNoValidatorFound returns an error when a validator doesn't exist
func ErrNoValidatorFound(valAddr string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeNoValidatorFound, fmt.Sprintf("validator %s does not exist", valAddr))}
}

func ErrPubkeyEqual(pubkey string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodePubkeyEqual, fmt.Sprintf("validator pubkey %s does exist", pubkey))}
}

// ErrInvalidDelegation returns an error when the delegation is invalid
func ErrInvalidDelegation(delegator string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeInvalidDelegation,
		fmt.Sprintf("failed. invalid delegation on %s", delegator))}
}

// ErrNilValidatorAddr returns an error when an empty validator address appears
func ErrNilValidatorAddr() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeNilValidatorAddr, "empty validator address")
}

// ErrBadValidatorAddr returns an error when an invalid validator address appears
func ErrBadValidatorAddr() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeBadValidatorAddr, "validator address is invalid")
}

// ErrMoreMinSelfDelegation returns an error when the msd doesn't match the rest of shares on a validator
func ErrMoreMinSelfDelegation(valAddr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeMoreMinSelfDelegation,
		fmt.Sprintf("failed. min self delegation of %s is more than its shares", valAddr))
}

// ErrProxyNotFound returns an error when a delegator who's not a proxy tries to unreg
func ErrProxyNotFound(delegator string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeProxyNotFound,
		fmt.Sprintf("failed. proxy %s not found", delegator))}
}

// ErrEmptyValidators returns an error when none of the validators in shares adding list is available
func ErrEmptyValidators() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeEmptyValidators,
		"failed. empty validators")
}

// ErrProxyAlreadyExist returns an error when a proxy tries to reg the second time
func ErrProxyAlreadyExist(delegator string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeProxyAlreadyExist,
		fmt.Sprintf("failed. delegator %s has already registered as a proxy", delegator))}
}

// ErrDescriptionIsEmpty returns an error when description is empty.
func ErrDescriptionIsEmpty() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeDescriptionIsEmpty, "empty description")
}

// ErrGetConsPubKeyBech32 returns an error when get bech32 consensus public key failed.
func ErrGetConsPubKeyBech32() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeGetConsPubKeyBech32Failed, "get bech32 consensus public key failed")
}

// ErrUnknownStakingQueryType returns an error when encounter unknown staking query type.
func ErrUnknownStakingQueryType() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeUnknownStakingQueryType, "unknown staking query type")
}

// ErrValidatorOwnerExists returns an error when the validator address has been registered
func ErrValidatorOwnerExists() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeValidatorOwnerExists,
		"validator already exist for this operator address, must use new validator operator address")
}

// ErrValidatorPubKeyExists returns an error when the validator consensus pubkey has been registered
func ErrValidatorPubKeyExists() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeValidatorPubKeyExists,
		"validator already exist for this pubkey, must use new validator pubkey")
}

// ErrValidatorPubKeyTypeNotSupported returns an error when the type of  pubkey was not supported
func ErrValidatorPubKeyTypeNotSupported(keyType string,
	supportedTypes []string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeValidatorPubKeyTypeNotSupported,
		fmt.Sprintf("validator pubkey type %s is not supported, must use %s", keyType, strings.Join(supportedTypes, ",")))
}

// ErrBondedPoolOrNotBondedIsNotExist returns an error when bonded pool or not bonded pool is empty.
func ErrBondedPoolOrNotBondedIsNotExist() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeBondedPoolOrNotBondedIsNotExist, "bonded pool or not bonded pool is empty")
}

// ErrDescriptionLength returns an error when the description of validator has a wrong length
func ErrDescriptionLength(descriptor string, got, max int) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInvalidDescriptionLength,
		fmt.Sprintf("bad description length for %v, got length %v, max is %v", descriptor, got, max))
}

// ErrCommissionNegative returns an error when the commission is not positive
func ErrCommissionNegative() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionNegative, "commission must be positive")
}

// ErrCommissionHuge returns an error when the commission is greater than 100%
func ErrCommissionHuge() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionHuge, "commission cannot be more than 100%")
}

// ErrCommissionGTMaxRate returns an error when the commission rate is greater than the max rate
func ErrCommissionGTMaxRate() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionGTMaxRate, "commission cannot be more than the max rate")
}

// ErrCommissionUpdateTime returns an error when the commission is remodified within 24 hours
func ErrCommissionUpdateTime() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionUpdateTime, "commission cannot be changed more than once in 24h")
}

// ErrCommissionChangeRateNegative returns an error when the commission change rate is not positive
func ErrCommissionChangeRateNegative() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionChangeRateNegative, "commission change rate must be positive")
}

// ErrCommissionChangeRateGTMaxRate returns an error when the commission change rate is greater than the max rate
func ErrCommissionChangeRateGTMaxRate() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionChangeRateGTMaxRate, "commission change rate cannot be more than the max rate")
}

// ErrCommissionGTMaxChangeRate returns an error when the new rate % points change is greater than the max change rate
func ErrCommissionGTMaxChangeRate() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeCommissionGTMaxChangeRate, "commission cannot be changed more than max change rate")
}

// ErrMinSelfDelegationInvalid returns an error when the msd isn't positive
func ErrMinSelfDelegationInvalid() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeMinSelfDelegationInvalid, "minimum self delegation must be a positive integer")
}

// ErrNilDelegatorAddr returns an error when the delegator address is nil
func ErrNilDelegatorAddr() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeNilDelegatorAddr, "delegator address is nil")
}

// ErrDelegatorEqualToProxyAddr returns an error when the address is not expected
func ErrDelegatorEqualToProxyAddr() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeDelegatorEqualToProxyAddr, "delegator address can not equals to proxy address")
}

// ErrBadDenom returns an error when the coin denomination is invalid
func ErrBadDenom() sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeBadDenom, "invalid coin denomination")}
}

// ErrBadDelegationAmount returns an error when the amount of delegation isn't positive
func ErrBadDelegationAmount() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeBadDelegationAmount, "amount must be more than 0")
}

// ErrNoUnbondingDelegation returns an error when the unbonding delegation doesn't exist
func ErrNoUnbondingDelegation() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeNoUnbondingDelegation, "no unbonding delegation found")
}

// ErrAddSharesToDismission returns an error when a zero-msd validator becomes the shares adding target
func ErrAddSharesToDismission(valAddr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeAddSharesToDismission,
		fmt.Sprintf("failed. destroyed validator %s isn't allowed to add shares to. please get rid of it from the "+
			"shares adding list by adding shares to other validators again or unbond all delegated tokens", valAddr))
}

// ErrAddSharesDuringProxy returns an error when a delegator who has bound tries to add shares to validators by itself
func ErrAddSharesDuringProxy(delegator string, proxy string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeAddSharesDuringProxy,
		fmt.Sprintf("failed. banned to add shares to validators before unbinding proxy relationship between %s and %s", delegator, proxy))}
}

// ErrDoubleProxy returns an error when a delegator trys to bind more than one proxy
func ErrDoubleProxy(delegator string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeDoubleProxy,
		fmt.Sprintf("failed. proxy isn't allowed to bind with other proxy %s", delegator))}
}

// ErrExceedValidatorAddrs returns an error when the number of target validators exceeds the max limit
func ErrExceedValidatorAddrs(num int) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeExceedValidatorAddrs,
		fmt.Sprintf("failed. the number of validator addresses is over the limit %d", num))}
}

// ErrNoDelegationToAddShares returns an error when there's no delegation to support adding shares to validators
func ErrNoDelegationToAddShares(delegator string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeNoDelegationToAddShares,
		fmt.Sprintf("failed. there's no delegation of %s", delegator))}
}

// ErrNotInDelegating returns an error when the UndelegationInfo doesn't exist during it's unbonding period
func ErrNotInDelegating(addr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeNotInDelegating,
		fmt.Sprintf("failed. the addr %s is not in the status of undelegating", addr))
}

// ErrInsufficientDelegation returns an error when the delegation left is not enough for unbonding
func ErrInsufficientDelegation(quantity, delLeft string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInsufficientDelegation,
		fmt.Sprintf("failed. insufficient delegation. [delegation left]:%s, [quantity to unbond]:%s", delLeft, quantity))
}

// ErrInsufficientQuantity returns an error when the quantity is less than the min delegation limit
func ErrInsufficientQuantity(quantity, minLimit string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInsufficientQuantity,
		fmt.Sprintf("failed. insufficient quantity. [min limit]:%s, [quantity]:%s", minLimit, quantity))
}

// ErrNoMinSelfDelegation returns an error when the msd has already been unbonded
func ErrNoMinSelfDelegation(valAddr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInvalidMinSelfDelegation,
		fmt.Sprintf("failed. there's no min self delegation on %s", valAddr))
}

// ErrBadUnDelegationAmount returns an error when the amount of delegation is not positive
func ErrBadUnDelegationAmount() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeBadUnDelegationAmount,
		"failed. undelegated amount must be greater than 0")
}

// ErrInvalidProxyUpdating returns an error when the total delegated tokens on a proxy are going to be negative
func ErrInvalidProxyUpdating() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInvalidProxyUpdating,
		"failed. the total delegated tokens on the proxy will be negative after this update")
}

// ErrInvalidProxyWithdrawTotal returns an error when proxy withdraws total tokens
func ErrInvalidProxyWithdrawTotal(addr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeInvalidProxyWithdrawTotal,
		fmt.Sprintf("failed. proxy %s has to unreg before withdrawing total tokens", addr))
}

// ErrAlreadyAddedShares returns an error when a delegator tries to bind proxy after adding shares
func ErrAlreadyAddedShares(delAddr string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeAlreadyAddedShares,
		fmt.Sprintf("failed. delegator %s isn't allowed to bind proxy while it has added shares. please unbond the delegation first", delAddr))}
}

// ErrNoDelegatorExisted returns an error when the info if a certain delegator doesn't exist
func ErrNoDelegatorExisted(delAddr string) sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeNoDelegatorExisted,
		fmt.Sprintf("failed. delegator %s doesn't exist", delAddr))
}

// ErrTargetValsDuplicate returns an error when the target validators in voting list are duplicate
func ErrTargetValsDuplicate() sdk.Error {
	return sdkerrors.New(DefaultCodespace, CodeTargetValsDuplicate,
		"failed. duplicate target validators")
}

// ErrAlreadyBound returns an error when a delegator keeps binding a proxy before proxy register
func ErrAlreadyBound(delAddr string) sdk.EnvelopedErr {
	return sdk.EnvelopedErr{Err: sdkerrors.New(DefaultCodespace, CodeAlreadyBound,
		fmt.Sprintf("failed. %s has already bound a proxy. it's necessary to unbind before proxy register",
			delAddr))}
}
