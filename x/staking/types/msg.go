package types

import (
	"encoding/json"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/x/common"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
)

// ensure Msg interface compliance at compile time
var (
	_ sdk.Msg = &MsgCreateValidator{}
	_ sdk.Msg = &MsgEditValidator{}
)

//______________________________________________________________________

// MsgCreateValidator - struct for bonding transactions
type MsgCreateValidator struct {
	Description Description `json:"description" yaml:"description"`
	//Commission        CommissionRates `json:"commission" yaml:"commission"`
	MinSelfDelegation sdk.SysCoin    `json:"min_self_delegation" yaml:"min_self_delegation"`
	DelegatorAddress  sdk.AccAddress `json:"delegator_address" yaml:"delegator_address"`
	ValidatorAddress  sdk.ValAddress `json:"validator_address" yaml:"validator_address"`
	PubKey            crypto.PubKey  `json:"pubkey" yaml:"pubkey"`
}

type msgCreateValidatorJSON struct {
	Description Description `json:"description" yaml:"description"`
	//Commission        CommissionRates `json:"commission" yaml:"commission"`
	MinSelfDelegation sdk.SysCoin    `json:"min_self_delegation" yaml:"min_self_delegation"`
	DelegatorAddress  sdk.AccAddress `json:"delegator_address" yaml:"delegator_address"`
	ValidatorAddress  sdk.ValAddress `json:"validator_address" yaml:"validator_address"`
	PubKey            string         `json:"pubkey" yaml:"pubkey"`
}

// NewMsgCreateValidator creates a msg of create-validator
// Delegator address and validator address are the same
func NewMsgCreateValidator(
	valAddr sdk.ValAddress, pubKey crypto.PubKey,
	description Description, minSelfDelegation sdk.SysCoin,
) MsgCreateValidator {

	return MsgCreateValidator{
		Description:       description,
		DelegatorAddress:  sdk.AccAddress(valAddr),
		ValidatorAddress:  valAddr,
		PubKey:            pubKey,
		MinSelfDelegation: minSelfDelegation,
	}
}

// nolint
func (msg MsgCreateValidator) Route() string { return RouterKey }
func (msg MsgCreateValidator) Type() string  { return "create_validator" }

// GetSigners returns address(es) that must sign over msg.GetSignBytes()
func (msg MsgCreateValidator) GetSigners() []sdk.AccAddress {
	// delegator is first signer so delegator pays fees
	addrs := []sdk.AccAddress{msg.DelegatorAddress}

	// TODO: the following will never be execute becoz ValidateBasic() raise error if DlgAddress != ValAddress
	//if !bytes.Equal(msg.DelegatorAddress.Bytes(), msg.ValidatorAddress.Bytes()) {
	//	// if validator addr is not same as delegator addr, validator must sign
	//	// msg as well
	//	addrs = append(addrs, sdk.AccAddress(msg.ValidatorAddress))
	//}
	return addrs
}

// MarshalJSON implements the json.Marshaler interface to provide custom JSON serialization
func (msg MsgCreateValidator) MarshalJSON() ([]byte, error) {
	return json.Marshal(msgCreateValidatorJSON{
		Description:       msg.Description,
		DelegatorAddress:  msg.DelegatorAddress,
		ValidatorAddress:  msg.ValidatorAddress,
		PubKey:            MustBech32ifyConsPub(msg.PubKey),
		MinSelfDelegation: msg.MinSelfDelegation,
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface to provide custom JSON deserialization
func (msg *MsgCreateValidator) UnmarshalJSON(bz []byte) error {
	var msgCreateValJSON msgCreateValidatorJSON
	if err := json.Unmarshal(bz, &msgCreateValJSON); err != nil {
		return common.ErrUnMarshalJSONFailed(err.Error())
	}

	msg.Description = msgCreateValJSON.Description
	msg.DelegatorAddress = msgCreateValJSON.DelegatorAddress
	msg.ValidatorAddress = msgCreateValJSON.ValidatorAddress
	var err error
	msg.PubKey, err = GetConsPubKeyBech32(msgCreateValJSON.PubKey)
	if err != nil {
		return ErrGetConsPubKeyBech32()
	}
	msg.MinSelfDelegation = msgCreateValJSON.MinSelfDelegation

	return nil
}

// GetSignBytes returns the message bytes to sign over
func (msg MsgCreateValidator) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

// ValidateBasic gives a quick validity check
func (msg MsgCreateValidator) ValidateBasic() error {
	// note that unmarshaling from bech32 ensures either empty or valid
	if msg.DelegatorAddress.Empty() {
		return ErrNilDelegatorAddr()
	}
	if msg.ValidatorAddress.Empty() {
		return ErrNilValidatorAddr()
	}
	if !sdk.AccAddress(msg.ValidatorAddress).Equals(msg.DelegatorAddress) {
		return ErrBadValidatorAddr()
	}
	if msg.MinSelfDelegation.Amount.LTE(sdk.ZeroDec()) || !msg.MinSelfDelegation.IsValid() {
		return ErrMinSelfDelegationInvalid()
	}
	if msg.Description == (Description{}) {
		return ErrDescriptionIsEmpty()
	}

	return nil
}

// MsgEditValidator - struct for editing a validator
type MsgEditValidator struct {
	Description
	ValidatorAddress sdk.ValAddress `json:"address" yaml:"address"`
	PubKey           crypto.PubKey  `json:"pubkey" yaml:"pubkey"`
}

// NewMsgEditValidator creates a msg of edit-validator
func NewMsgEditValidator(valAddr sdk.ValAddress, description Description, pubKey crypto.PubKey) MsgEditValidator {
	return MsgEditValidator{
		Description:      description,
		ValidatorAddress: valAddr,
		PubKey:           pubKey,
	}
}

// nolint
func (msg MsgEditValidator) Route() string { return RouterKey }
func (msg MsgEditValidator) Type() string  { return "edit_validator" }
func (msg MsgEditValidator) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{sdk.AccAddress(msg.ValidatorAddress)}
}

// GetSignBytes gets the bytes for the message signer to sign on
func (msg MsgEditValidator) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

// ValidateBasic gives a quick validity check
func (msg MsgEditValidator) ValidateBasic() error {
	if msg.ValidatorAddress.Empty() {
		return ErrNilValidatorAddr()
	}

	if msg.Description == (Description{}) && (msg.PubKey == nil || len(msg.PubKey.Bytes()) == 0) {
		return ErrDescriptionAndPubkeyIsEmpty()
	}

	return nil
}
