package types

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/tendermint/global"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
)

var (
	_ sdk.Msg = &MsgRegisterFeeSplit{}
	_ sdk.Msg = &MsgCancelFeeSplit{}
	_ sdk.Msg = &MsgUpdateFeeSplit{}
)

const (
	TypeMsgRegisterFeeSplit = "register_fee_split"
	TypeMsgCancelFeeSplit   = "cancel_fee_split"
	TypeMsgUpdateFeeSplit   = "update_fee_split"
)

// MsgRegisterFeeSplit defines a message that registers a FeeSplit
type MsgRegisterFeeSplit struct {
	// contract hex address
	ContractAddress string `json:"contract_address,omitempty"`
	// bech32 address of message sender, must be the same as the origin EOA
	// sending the transaction which deploys the contract
	DeployerAddress string `json:"deployer_address,omitempty"`
	// bech32 address of account receiving the transaction fees
	WithdrawerAddress string `json:"withdrawer_address,omitempty"`
	// array of nonces from the address path, where the last nonce is the nonce
	// that determines the contract's address - it can be an EOA nonce or a
	// factory contract nonce
	Nonces []uint64 `json:"nonces,omitempty"`
}

// NewMsgRegisterFeeSplit creates new instance of MsgRegisterFeeSplit
func NewMsgRegisterFeeSplit(
	contract common.Address,
	deployer,
	withdrawer sdk.AccAddress,
	nonces []uint64,
) MsgRegisterFeeSplit {
	withdrawerAddress := ""
	if withdrawer != nil {
		withdrawerAddress = withdrawer.String()
	}

	return MsgRegisterFeeSplit{
		ContractAddress:   contract.String(),
		DeployerAddress:   deployer.String(),
		WithdrawerAddress: withdrawerAddress,
		Nonces:            nonces,
	}
}

// Route returns the name of the module
func (msg MsgRegisterFeeSplit) Route() string { return RouterKey }

// Type returns the action
func (msg MsgRegisterFeeSplit) Type() string { return TypeMsgRegisterFeeSplit }

// ValidateBasic runs stateless checks on the message
func (msg MsgRegisterFeeSplit) ValidateBasic() error {
	if global.GetGlobalHeight() > 0 && !tmtypes.HigherThanVenus3(global.GetGlobalHeight()) {
		return ErrNotFeesplitHeight
	}

	if _, err := sdk.AccAddressFromBech32(msg.DeployerAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid deployer address %s", msg.DeployerAddress)
	}

	if err := ValidateNonZeroAddress(msg.ContractAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid contract address %s", msg.ContractAddress)
	}

	if msg.WithdrawerAddress != "" {
		if _, err := sdk.AccAddressFromBech32(msg.WithdrawerAddress); err != nil {
			return sdkerrors.Wrapf(err, "invalid withdraw address %s", msg.WithdrawerAddress)
		}
	}

	if len(msg.Nonces) < 1 {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "invalid nonces - empty array")
	}

	if len(msg.Nonces) > 20 {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "invalid nonces - array length must be less than 20")
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgRegisterFeeSplit) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgRegisterFeeSplit) GetSigners() []sdk.AccAddress {
	from := sdk.MustAccAddressFromBech32(msg.DeployerAddress)
	return []sdk.AccAddress{from}
}

// MsgUpdateFeeSplit defines a message that updates the withdrawer address for a
// registered FeeSplit
type MsgUpdateFeeSplit struct {
	// contract hex address
	ContractAddress string `json:"contract_address,omitempty"`
	// deployer bech32 address
	DeployerAddress string `json:"deployer_address,omitempty"`
	// new withdrawer bech32 address for receiving the transaction fees
	WithdrawerAddress string `json:"withdrawer_address,omitempty"`
}

// NewMsgUpdateFeeSplit creates new instance of MsgUpdateFeeSplit
func NewMsgUpdateFeeSplit(
	contract common.Address,
	deployer,
	withdraw sdk.AccAddress,
) MsgUpdateFeeSplit {
	return MsgUpdateFeeSplit{
		ContractAddress:   contract.String(),
		DeployerAddress:   deployer.String(),
		WithdrawerAddress: withdraw.String(),
	}
}

// Route returns the name of the module
func (msg MsgUpdateFeeSplit) Route() string { return RouterKey }

// Type returns the action
func (msg MsgUpdateFeeSplit) Type() string { return TypeMsgUpdateFeeSplit }

// ValidateBasic runs stateless checks on the message
func (msg MsgUpdateFeeSplit) ValidateBasic() error {
	if global.GetGlobalHeight() > 0 && !tmtypes.HigherThanVenus3(global.GetGlobalHeight()) {
		return ErrNotFeesplitHeight
	}

	if _, err := sdk.AccAddressFromBech32(msg.DeployerAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid deployer address %s", msg.DeployerAddress)
	}

	if err := ValidateNonZeroAddress(msg.ContractAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid contract address %s", msg.ContractAddress)
	}

	if _, err := sdk.AccAddressFromBech32(msg.WithdrawerAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid withdraw address %s", msg.WithdrawerAddress)
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdateFeeSplit) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdateFeeSplit) GetSigners() []sdk.AccAddress {
	from := sdk.MustAccAddressFromBech32(msg.DeployerAddress)
	return []sdk.AccAddress{from}
}

// MsgCancelFeeSplit defines a message that cancels a registered FeeSplit
type MsgCancelFeeSplit struct {
	// contract hex address
	ContractAddress string `json:"contract_address,omitempty"`
	// deployer bech32 address
	DeployerAddress string `json:"deployer_address,omitempty"`
}

// NewMsgCancelFeeSplit creates new instance of MsgCancelFeeSplit.
func NewMsgCancelFeeSplit(
	contract common.Address,
	deployer sdk.AccAddress,
) MsgCancelFeeSplit {
	return MsgCancelFeeSplit{
		ContractAddress: contract.String(),
		DeployerAddress: deployer.String(),
	}
}

// Route returns the message route for a MsgCancelFeeSplit.
func (msg MsgCancelFeeSplit) Route() string { return RouterKey }

// Type returns the message type for a MsgCancelFeeSplit.
func (msg MsgCancelFeeSplit) Type() string { return TypeMsgCancelFeeSplit }

// ValidateBasic runs stateless checks on the message
func (msg MsgCancelFeeSplit) ValidateBasic() error {
	if global.GetGlobalHeight() > 0 && !tmtypes.HigherThanVenus3(global.GetGlobalHeight()) {
		return ErrNotFeesplitHeight
	}

	if _, err := sdk.AccAddressFromBech32(msg.DeployerAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid deployer address %s", msg.DeployerAddress)
	}

	if err := ValidateNonZeroAddress(msg.ContractAddress); err != nil {
		return sdkerrors.Wrapf(err, "invalid contract address %s", msg.ContractAddress)
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCancelFeeSplit) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCancelFeeSplit) GetSigners() []sdk.AccAddress {
	funder := sdk.MustAccAddressFromBech32(msg.DeployerAddress)
	return []sdk.AccAddress{funder}
}

// isZeroAddress returns true if the address corresponds to an empty ethereum hex address.
func isZeroAddress(address string) bool {
	return bytes.Equal(common.HexToAddress(address).Bytes(), common.Address{}.Bytes())
}

// validateAddress returns an error if the provided string is either not a hex formatted string address
func validateAddress(address string) error {
	if !common.IsHexAddress(address) {
		return sdkerrors.Wrapf(
			sdkerrors.ErrInvalidAddress, "address '%s' is not a valid ethereum hex address",
			address,
		)
	}
	return nil
}

// ValidateNonZeroAddress returns an error if the provided string is not a hex
// formatted string address or is equal to zero
func ValidateNonZeroAddress(address string) error {
	if isZeroAddress(address) {
		return sdkerrors.Wrapf(
			sdkerrors.ErrInvalidAddress, "address '%s' must not be zero",
			address,
		)
	}
	return validateAddress(address)
}
