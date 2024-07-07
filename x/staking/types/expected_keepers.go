package types

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	authexported "github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	supplyexported "github.com/okex/exchain/libs/cosmos-sdk/x/supply/exported"
	"github.com/okex/exchain/libs/tendermint/crypto"
	stakingexported "github.com/okex/exchain/x/staking/exported"
)

// AccountKeeper defines the expected account keeper (noalias)
type AccountKeeper interface {
	IterateAccounts(ctx sdk.Context, process func(authexported.Account) (stop bool))
}

// SupplyKeeper defines the expected supply Keeper (noalias)
type SupplyKeeper interface {
	GetSupplyByDenom(ctx sdk.Context, denom string) sdk.Dec

	GetModuleAddress(name string) sdk.AccAddress
	GetModuleAccount(ctx sdk.Context, moduleName string) supplyexported.ModuleAccountI

	// TODO remove with genesis 2-phases refactor https://github.com/cosmos/cosmos-sdk/issues/2862
	SetModuleAccount(sdk.Context, supplyexported.ModuleAccountI)

	SendCoinsFromModuleToModule(ctx sdk.Context, senderPool, recipientPool string, amt sdk.Coins) error
	UndelegateCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress,
		amt sdk.SysCoins) error
	DelegateCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string,
		amt sdk.SysCoins) error

	BurnCoins(ctx sdk.Context, name string, amt sdk.Coins) error
}

// ValidatorSet expected properties for the set of all validators (noalias)
type ValidatorSet interface {
	// iterate through validators by operator address, execute func for each validator
	IterateValidators(sdk.Context,
		func(index int64, validator stakingexported.ValidatorI) (stop bool))

	// iterate through bonded validators by operator address, execute func for each validator
	IterateBondedValidatorsByPower(sdk.Context,
		func(index int64, validator stakingexported.ValidatorI) (stop bool))

	// iterate through the consensus validator set of the last block by operator address, execute func for each validator
	IterateLastValidators(sdk.Context,
		func(index int64, validator stakingexported.ValidatorI) (stop bool))
	// get a particular validator by operator address
	Validator(sdk.Context, sdk.ValAddress) stakingexported.ValidatorI
	// get a particular validator by consensus address
	ValidatorByConsAddr(sdk.Context, sdk.ConsAddress) stakingexported.ValidatorI
	// total bonded tokens within the validator set
	TotalBondedTokens(sdk.Context) sdk.Dec
	// total staking token supply
	StakingTokenSupply(sdk.Context) sdk.Dec

	// slash the validator and delegators of the validator, specifying offence height, offence power, and slash fraction
	// jail a validator
	Jail(sdk.Context, sdk.ConsAddress)
	// unjail a validator
	Unjail(sdk.Context, sdk.ConsAddress)

	// MaxValidators returns the maximum amount of bonded validators
	MaxValidators(sdk.Context) uint16
}

//_______________________________________________________________________________
// Event Hooks
// These can be utilized to communicate between a staking keeper and another
// keeper which must take particular actions when validators/delegators change
// state. The second keeper must implement this interface, which then the
// staking keeper can call.

// StakingHooks event hooks for staking validator object (noalias)
type StakingHooks interface {
	// Must be called when a validator is created
	AfterValidatorCreated(ctx sdk.Context, valAddr sdk.ValAddress)
	AfterValidatorPubkeyChanged(ctx sdk.Context, oldAddress sdk.ConsAddress, newAddress sdk.ConsAddress, newPubkey crypto.PubKey)
	// Must be called when a validator's state changes
	BeforeValidatorModified(ctx sdk.Context, valAddr sdk.ValAddress)
	// Must be called when a validator is deleted
	AfterValidatorRemoved(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress)
	// Must be called when a validator is bonded
	AfterValidatorBonded(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress)
	// Must be called when a validator begins unbonding
	AfterValidatorBeginUnbonding(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress)

	// required by okexchain
	// Must be called when a validator is destroyed by tx
	AfterValidatorDestroyed(ctx sdk.Context, consAddr sdk.ConsAddress, valAddr sdk.ValAddress)

	// Must be called when a delegation is created
	BeforeDelegationCreated(ctx sdk.Context, delAddr sdk.AccAddress, valAddrs []sdk.ValAddress)
	// Must be called when a delegation's shares are modified
	BeforeDelegationSharesModified(ctx sdk.Context, delAddr sdk.AccAddress, valAddrs []sdk.ValAddress)
	// Must be called when a delegation is removed
	BeforeDelegationRemoved(ctx sdk.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress)
	// Must be called when a delegation is modified
	AfterDelegationModified(ctx sdk.Context, delAddr sdk.AccAddress, valAddrs []sdk.ValAddress)
	//BeforeValidatorSlashed(ctx sdk.Context, valAddr sdk.ValAddress, fraction sdk.Dec)
	// Check modules enabled
	CheckEnabled(ctx sdk.Context) bool
}
