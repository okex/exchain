// nolint
package types

import (
	"strings"

	ethcmm "github.com/ethereum/go-ethereum/common"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/common"
)

const (
	DescLenLimit   = 256
	MultiSendLimit = 1000

	// 90 billion
	TotalSupplyUpperbound = int64(9 * 1e10)
)

type MsgTokenIssue struct {
	Description    string         `json:"description"`
	Symbol         string         `json:"symbol"`
	OriginalSymbol string         `json:"original_symbol"`
	WholeName      string         `json:"whole_name"`
	TotalSupply    string         `json:"total_supply"`
	Owner          sdk.AccAddress `json:"owner"`
	Mintable       bool           `json:"mintable"`
}

func NewMsgTokenIssue(tokenDescription, symbol, originalSymbol, wholeName, totalSupply string, owner sdk.AccAddress, mintable bool) MsgTokenIssue {
	return MsgTokenIssue{
		Description:    tokenDescription,
		Symbol:         symbol,
		OriginalSymbol: originalSymbol,
		WholeName:      wholeName,
		TotalSupply:    totalSupply,
		Owner:          owner,
		Mintable:       mintable,
	}
}

func (msg MsgTokenIssue) Route() string { return RouterKey }

func (msg MsgTokenIssue) Type() string { return "issue" }

func (msg MsgTokenIssue) ValidateBasic() sdk.Error {
	// check owner
	if msg.Owner.Empty() {
		return ErrAddressIsRequired()
	}

	// check original symbol
	if len(msg.OriginalSymbol) == 0 {
		return ErrUserInputSymbolIsEmpty()
	}
	if !ValidOriginalSymbol(msg.OriginalSymbol) {
		return ErrNotAllowedOriginalSymbol(msg.OriginalSymbol)
	}

	// check wholeName
	isValid := wholeNameValid(msg.WholeName)
	if !isValid {
		return ErrWholeNameIsNotValidl()
	}
	// check desc
	if len(msg.Description) > DescLenLimit {
		return ErrDescLenBiggerThanLimit()
	}
	// check totalSupply
	totalSupply, err := sdk.NewDecFromStr(msg.TotalSupply)
	if err != nil {
		return err
	}
	if totalSupply.GT(sdk.NewDec(TotalSupplyUpperbound)) || totalSupply.LTE(sdk.ZeroDec()) {
		return ErrTotalSupplyOutOfRange()
	}
	return nil
}

func (msg MsgTokenIssue) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgTokenIssue) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

type MsgTokenBurn struct {
	Amount sdk.SysCoin    `json:"amount"`
	Owner  sdk.AccAddress `json:"owner"`
}

func NewMsgTokenBurn(amount sdk.SysCoin, owner sdk.AccAddress) MsgTokenBurn {
	return MsgTokenBurn{
		Amount: amount,
		Owner:  owner,
	}
}

func (msg MsgTokenBurn) Route() string { return RouterKey }

func (msg MsgTokenBurn) Type() string { return "burn" }

func (msg MsgTokenBurn) ValidateBasic() sdk.Error {
	// check owner
	if msg.Owner.Empty() {
		return ErrAddressIsRequired()
	}
	if !msg.Amount.IsValid() {
		return common.ErrInsufficientCoins(DefaultParamspace, msg.Amount.String())
	}

	return nil
}

func (msg MsgTokenBurn) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgTokenBurn) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

type MsgTokenMint struct {
	Amount sdk.SysCoin    `json:"amount"`
	Owner  sdk.AccAddress `json:"owner"`
}

func NewMsgTokenMint(amount sdk.SysCoin, owner sdk.AccAddress) MsgTokenMint {
	return MsgTokenMint{
		Amount: amount,
		Owner:  owner,
	}
}

func (msg MsgTokenMint) Route() string { return RouterKey }

func (msg MsgTokenMint) Type() string { return "mint" }

func (msg MsgTokenMint) ValidateBasic() sdk.Error {
	if msg.Owner.Empty() {
		return ErrAddressIsRequired()
	}

	amount := msg.Amount.Amount
	if amount.GT(sdk.NewDec(TotalSupplyUpperbound)) {
		return ErrAmountBiggerThanTotalSupplyUpperbound()
	}
	if !msg.Amount.IsValid() {
		return ErrAmountIsNotValid(msg.Amount.String())
	}
	return nil
}

func (msg MsgTokenMint) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)

	return sdk.MustSortJSON(bz)
}

func (msg MsgTokenMint) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

type MsgMultiSend struct {
	From      sdk.AccAddress `json:"from"`
	Transfers []TransferUnit `json:"transfers"`
}

func NewMsgMultiSend(from sdk.AccAddress, transfers []TransferUnit) MsgMultiSend {
	return MsgMultiSend{
		From:      from,
		Transfers: transfers,
	}
}

func (msg MsgMultiSend) Route() string { return RouterKey }

func (msg MsgMultiSend) Type() string { return "multi-send" }

func (msg MsgMultiSend) ValidateBasic() sdk.Error {
	if msg.From.Empty() {
		return ErrAddressIsRequired()
	}

	// check transfers
	if len(msg.Transfers) > MultiSendLimit {
		return ErrMsgTransfersAmountBiggerThanSendLimit()
	}
	for _, transfer := range msg.Transfers {
		if !transfer.Coins.IsAllPositive() || !transfer.Coins.IsValid() {
			return ErrInvalidCoins(transfer.Coins.String())
		}

		if transfer.To.Empty() {
			return ErrAddressIsRequired()
		}
	}
	return nil
}

func (msg MsgMultiSend) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgMultiSend) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.From}
}

// MsgSend - high level transaction of the coin module
type MsgSend struct {
	FromAddress sdk.AccAddress `json:"from_address"`
	ToAddress   sdk.AccAddress `json:"to_address"`
	Amount      sdk.SysCoins   `json:"amount"`
}

func NewMsgTokenSend(from, to sdk.AccAddress, coins sdk.SysCoins) MsgSend {
	return MsgSend{
		FromAddress: from,
		ToAddress:   to,
		Amount:      coins,
	}
}

func (msg MsgSend) Route() string { return RouterKey }

func (msg MsgSend) Type() string { return "send" }

func (msg MsgSend) ValidateBasic() sdk.Error {
	if msg.FromAddress.Empty() {
		return ErrAddressIsRequired()
	}
	if msg.ToAddress.Empty() {
		return ErrAddressIsRequired()
	}
	if !msg.Amount.IsValid() {
		return ErrInvalidCoins(msg.Amount.String())
	}
	if !msg.Amount.IsAllPositive() {
		return common.ErrInsufficientCoins(DefaultParamspace, msg.Amount.String())
	}
	return nil
}

func (msg MsgSend) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgSend) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.FromAddress}
}

func (msg MsgSend) CalFromAndToForPara() (string, string) {
	return strings.ToLower(ethcmm.BytesToAddress(msg.FromAddress).String()[2:]), strings.ToLower(ethcmm.BytesToAddress(msg.ToAddress).String()[2:])
}

// MsgTransferOwnership - high level transaction of the coin module
type MsgTransferOwnership struct {
	FromAddress sdk.AccAddress `json:"from_address"`
	ToAddress   sdk.AccAddress `json:"to_address"`
	Symbol      string         `json:"symbol"`
}

func NewMsgTransferOwnership(from, to sdk.AccAddress, symbol string) MsgTransferOwnership {
	return MsgTransferOwnership{
		FromAddress: from,
		ToAddress:   to,
		Symbol:      symbol,
	}
}

func (msg MsgTransferOwnership) Route() string { return RouterKey }

func (msg MsgTransferOwnership) Type() string { return "transfer" }

func (msg MsgTransferOwnership) ValidateBasic() sdk.Error {
	if msg.FromAddress.Empty() {
		return ErrAddressIsRequired()
	}
	if msg.ToAddress.Empty() {
		return ErrAddressIsRequired()
	}
	if len(msg.Symbol) == 0 {
		return ErrMsgSymbolIsEmpty()
	}

	if sdk.ValidateDenom(msg.Symbol) != nil {
		return ErrConfirmOwnershipNotExistOrBlockTimeAfter()
	}
	return nil
}

func (msg MsgTransferOwnership) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgTransferOwnership) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.FromAddress}
}

type MsgTokenModify struct {
	Owner                 sdk.AccAddress `json:"owner"`
	Symbol                string         `json:"symbol"`
	Description           string         `json:"description"`
	WholeName             string         `json:"whole_name"`
	IsDescriptionModified bool           `json:"description_modified"`
	IsWholeNameModified   bool           `json:"whole_name_modified"`
}

func NewMsgTokenModify(symbol, desc, wholeName string, isDescEdit, isWholeNameEdit bool, owner sdk.AccAddress) MsgTokenModify {
	return MsgTokenModify{
		Symbol:                symbol,
		IsDescriptionModified: isDescEdit,
		Description:           desc,
		IsWholeNameModified:   isWholeNameEdit,
		WholeName:             wholeName,
		Owner:                 owner,
	}
}

func (msg MsgTokenModify) Route() string { return RouterKey }

func (msg MsgTokenModify) Type() string { return "edit" }

func (msg MsgTokenModify) ValidateBasic() sdk.Error {
	// check owner
	if msg.Owner.Empty() {
		return ErrAddressIsRequired()
	}
	// check symbol
	if len(msg.Symbol) == 0 {
		return ErrMsgSymbolIsEmpty()
	}
	if sdk.ValidateDenom(msg.Symbol) != nil {
		return ErrNotAllowedOriginalSymbol(msg.Symbol)
	}
	// check wholeName
	if msg.IsWholeNameModified {
		isValid := wholeNameValid(msg.WholeName)
		if !isValid {
			return ErrWholeNameIsNotValidl()
		}
	}
	// check desc
	if msg.IsDescriptionModified {
		if len(msg.Description) > DescLenLimit {
			return ErrDescLenBiggerThanLimit()
		}
	}
	return nil
}

func (msg MsgTokenModify) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgTokenModify) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

// MsgConfirmOwnership - high level transaction of the coin module
type MsgConfirmOwnership struct {
	Symbol  string         `json:"symbol"`
	Address sdk.AccAddress `json:"new_owner"`
}

func NewMsgConfirmOwnership(newOwner sdk.AccAddress, symbol string) MsgConfirmOwnership {
	return MsgConfirmOwnership{
		Symbol:  symbol,
		Address: newOwner,
	}
}

func (msg MsgConfirmOwnership) Route() string { return RouterKey }

func (msg MsgConfirmOwnership) Type() string { return "confirm" }

func (msg MsgConfirmOwnership) ValidateBasic() sdk.Error {
	if msg.Address.Empty() {
		return ErrAddressIsRequired()
	}

	if len(msg.Symbol) == 0 {
		return ErrMsgSymbolIsEmpty()
	}

	if sdk.ValidateDenom(msg.Symbol) != nil {
		return ErrNotAllowedOriginalSymbol(msg.Symbol)
	}
	return nil
}

func (msg MsgConfirmOwnership) GetSignBytes() []byte {
	bz := ModuleCdc.MustMarshalJSON(msg)
	return sdk.MustSortJSON(bz)
}

func (msg MsgConfirmOwnership) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Address}
}
