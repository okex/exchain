package staking

import (
	"testing"

	"github.com/okex/exchain/libs/tendermint/crypto/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	keep "github.com/okex/exchain/x/staking/keeper"
	"github.com/okex/exchain/x/staking/types"
)

//______________________________________________________________________

// retrieve params which are instant
func setInstantUnbondPeriod(keeper keep.Keeper, ctx sdk.Context) types.Params {
	params := keeper.GetParams(ctx)
	params.UnbondingTime = 1
	keeper.SetParams(ctx, params)
	return params
}

//______________________________________________________________________

func TestValidatorByPowerIndex(t *testing.T) {
	validatorAddr, validatorAddr2 := sdk.ValAddress(keep.Addrs[0]), sdk.ValAddress(keep.Addrs[1])

	initPower := int64(1000000)
	ctx, _, mKeeper := CreateTestInput(t, false, initPower)
	keeper := mKeeper.Keeper
	_ = setInstantUnbondPeriod(keeper, ctx)

	// create validator
	msgCreateValidator := NewTestMsgCreateValidator(validatorAddr, keep.PKs[0], DefaultMSD)
	got, err := handleMsgCreateValidator(ctx, msgCreateValidator, keeper)
	require.Nil(t, err, "expected create-validator to be ok, got %v", got)

	// must end-block
	updates := keeper.ApplyAndReturnValidatorSetUpdates(ctx)
	require.Equal(t, 1, len(updates))

	// verify that the by power index exists
	validator, found := keeper.GetValidator(ctx, validatorAddr)
	require.True(t, found)
	power := GetValidatorsByPowerIndexKey(validator)
	require.True(t, ValidatorByPowerIndexExists(ctx, mKeeper, power))

	// create a second validator keep it bonded
	msgCreateValidator = NewTestMsgCreateValidator(validatorAddr2, keep.PKs[2], DefaultMSD)
	got, err = handleMsgCreateValidator(ctx, msgCreateValidator, keeper)
	require.Nil(t, err, "expected create-validator to be ok, got %v", got)

	// must end-block
	updates = keeper.ApplyAndReturnValidatorSetUpdates(ctx)
	require.Equal(t, 1, len(updates))

	EndBlocker(ctx, keeper)
	_, found = keeper.GetValidator(ctx, validatorAddr)
	require.True(t, found)

	powerIndex := GetValidatorsByPowerIndexKey(validator)
	require.True(t, ValidatorByPowerIndexExists(ctx, mKeeper, powerIndex))

}

func TestDuplicatesMsgCreateValidator(t *testing.T) {

	initPower := int64(1000000)

	ctx, _, mKeeper := CreateTestInput(t, false, initPower)
	keeper := mKeeper.Keeper

	addr1, addr2 := sdk.ValAddress(keep.Addrs[0]), sdk.ValAddress(keep.Addrs[1])
	pk1, pk2 := keep.PKs[0], keep.PKs[1]

	msgCreateValidator1 := NewTestMsgCreateValidator(addr1, pk1, DefaultMSD)
	got, err := handleMsgCreateValidator(ctx, msgCreateValidator1, keeper)
	require.Nil(t, err, "%v", got)

	keeper.ApplyAndReturnValidatorSetUpdates(ctx)

	validator, found := keeper.GetValidator(ctx, addr1)
	require.True(t, found)
	assert.Equal(t, sdk.Bonded, validator.Status)
	assert.Equal(t, addr1, validator.OperatorAddress)
	assert.Equal(t, pk1, validator.ConsPubKey)
	assert.Equal(t, DefaultMSD, validator.MinSelfDelegation)
	require.True(t, keeper.IsValidator(ctx, validator.OperatorAddress.Bytes()))

	assert.Equal(t, SharesFromDefaultMSD, validator.DelegatorShares)
	assert.Equal(t, defaultDescriptionForTest(), validator.Description)

	// two validators can't have the same operator address
	msgCreateValidator2 := NewTestMsgCreateValidator(addr1, pk2, DefaultMSD)
	got, err = handleMsgCreateValidator(ctx, msgCreateValidator2, keeper)
	require.NotNil(t, err, "%v", got)

	// two validators can't have the same pubkey
	msgCreateValidator3 := NewTestMsgCreateValidator(addr2, pk1, DefaultMSD)
	got, err = handleMsgCreateValidator(ctx, msgCreateValidator3, keeper)
	require.NotNil(t, err, "%v", got)

	// must have different pubkey and operator
	msgCreateValidator4 := NewTestMsgCreateValidator(addr2, pk2, DefaultMSD)
	got, err = handleMsgCreateValidator(ctx, msgCreateValidator4, keeper)
	require.Nil(t, err, "%v", got)

	// must end-block
	updates := keeper.ApplyAndReturnValidatorSetUpdates(ctx)
	require.Equal(t, 1, len(updates))

	validator, found = keeper.GetValidator(ctx, addr2)

	require.True(t, found)
	assert.Equal(t, sdk.Bonded, validator.Status)
	assert.Equal(t, addr2, validator.OperatorAddress)
	assert.Equal(t, pk2, validator.ConsPubKey)
	assert.True(sdk.DecEq(t, DefaultMSD, validator.MinSelfDelegation))

	assert.True(sdk.DecEq(t, SharesFromDefaultMSD, validator.DelegatorShares))
	assert.Equal(t, defaultDescriptionForTest(), validator.Description)
}

func defaultDescriptionForTest() Description {
	return Description{
		Moniker:  "my moniker",
		Identity: "my identity",
		Website:  "my website",
		Details:  "my details",
	}
}

func TestInvalidPubKeyTypeMsgCreateValidator(t *testing.T) {

	ctx, _, mKeeper := CreateTestInput(t, false, SufficientInitPower)
	keeper := mKeeper.Keeper

	addr := sdk.ValAddress(keep.Addrs[0])
	invalidPk := secp256k1.GenPrivKey().PubKey()

	// invalid pukKey type should not be allowed
	msgCreateValidator := NewTestMsgCreateValidator(addr, invalidPk, DefaultMSD)
	got, err := handleMsgCreateValidator(ctx, msgCreateValidator, keeper)
	require.NotNil(t, err, "%v", got)

	ctx.SetConsensusParams(&abci.ConsensusParams{
		Validator: &abci.ValidatorParams{PubKeyTypes: []string{tmtypes.ABCIPubKeyTypeSecp256k1}},
	})

	got, err = handleMsgCreateValidator(ctx, msgCreateValidator, keeper)
	require.Nil(t, err, "%v", got)
}

// TODO: msd is fixed now. nothing could change it!!!
func TestEditValidatorDecreaseMinSelfDelegation(t *testing.T) {
	validatorAddr := sdk.ValAddress(keep.Addrs[0])
	ctx, _, mKeeper := CreateTestInput(t, false, SufficientInitPower)
	keeper := mKeeper.Keeper
	_ = setInstantUnbondPeriod(keeper, ctx)

	// create validator
	msgCreateValidator := NewTestMsgCreateValidator(validatorAddr, keep.PKs[0], DefaultMSD)
	handler := NewHandler(keeper)
	got, err := handler(ctx, msgCreateValidator)
	require.Nil(t, err, "expected create-validator to be ok, got %v", got)

	// must end-block
	updates := keeper.ApplyAndReturnValidatorSetUpdates(ctx)
	require.Equal(t, 1, len(updates))
	SimpleCheckValidator(t, ctx, keeper, validatorAddr, DefaultMSD, sdk.Bonded,
		SharesFromDefaultMSD, false)

	// edit validator
	msgEditValidator := NewMsgEditValidator(validatorAddr, Description{Moniker: "moniker"})
	require.Nil(t, msgEditValidator.ValidateBasic())

	// no one could change msd
	got, err = handler(ctx, msgEditValidator)
	require.Nil(t, err, "should not be able to decrease minSelfDelegation")
	SimpleCheckValidator(t, ctx, keeper, validatorAddr, DefaultMSD, sdk.Bonded,
		SharesFromDefaultMSD, false)
}
