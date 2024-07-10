package types

import (
	"testing"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/stretchr/testify/require"
)

var (
	coinPos  = sdk.NewInt64Coin(sdk.DefaultBondDenom, 1000)
	coinZero = sdk.NewInt64Coin(sdk.DefaultBondDenom, 0)
)

// test ValidateBasic for MsgCreateValidator
func TestMsgCreateValidator(t *testing.T) {
	addr1 := valAddr1
	tests := []struct {
		name, moniker, identity, website, details string
		minSelfDelegation                         sdk.Int
		validatorAddr                             sdk.ValAddress
		delegatorAddr                             sdk.AccAddress
		pubkey                                    crypto.PubKey
		bond                                      sdk.Coin
		expectPass                                bool
	}{
		{"empty bond", "a", "b", "c", "d", sdk.OneInt(), addr1, dlgAddr1, pk1, coinZero, true},
		{"zero min self delegation", "a", "b", "c", "d", sdk.ZeroInt(), addr1, dlgAddr1, pk1, coinPos, false},
		{"basic good", "a", "b", "c", "d", sdk.OneInt(), addr1, dlgAddr1, pk1, coinPos, true},
		{"partial description", "", "", "c", "", sdk.OneInt(), addr1, dlgAddr1, pk1, coinPos, true},
		{"empty description", "", "", "", "", sdk.OneInt(), addr1, dlgAddr1, pk1, coinPos, false},
		{"empty address1", "a", "b", "c", "d", sdk.OneInt(), emptyAddr, dlgAddr1, pk1, coinPos, false},
		{"empty address2", "a", "b", "c", "d", sdk.OneInt(), nil, nil, pk1, coinPos, false},
		{"valAddr dlgAddr not equals", "a", "b", "c", "d", sdk.OneInt(), addr1, dlgAddr2, pk1, coinPos, false},
		{"empty pubkey", "a", "b", "c", "d", sdk.OneInt(), addr1, dlgAddr1, emptyPubkey, coinPos, true},
		//{"negative min self delegation", "a", "b", "c", "d", commission1, sdk.NewInt(-1), addr1, pk1, coinPos, false},
		//{"delegation less than min self delegation", "a", "b", "c", "d", commission1, coinPos.Amount.Add(sdk.OneInt()), addr1, pk1, coinPos, false},
	}

	for _, tc := range tests {
		description := NewDescription(tc.moniker, tc.identity, tc.website, tc.details)
		coin := sdk.NewDecCoin(sdk.DefaultBondDenom, tc.minSelfDelegation)

		msg := MsgCreateValidator{
			Description:       description,
			DelegatorAddress:  tc.delegatorAddr,
			ValidatorAddress:  tc.validatorAddr,
			PubKey:            tc.pubkey,
			MinSelfDelegation: coin,
		}

		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

// test ValidateBasic for MsgDestroyValidator
func TestMsgDestroyValidator(t *testing.T) {

	tests := []struct {
		name       string
		valAddr    sdk.AccAddress
		expectPass bool
	}{
		{"basic good", dlgAddr1, true},
		{"empty validator", sdk.AccAddress(emptyAddr), false},
	}

	for _, tc := range tests {
		msg := NewMsgDestroyValidator(tc.valAddr)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "destroy_validator")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

func TestMsgCreateValidator_Smoke(t *testing.T) {

	msd := sdk.NewDecCoinFromDec(sdk.DefaultBondDenom, sdk.NewDec(2000))

	msg := NewMsgCreateValidator(valAddr1, pk1,
		NewDescription("my moniker", "my identity", "my website", "my details"), msd,
	)
	require.Contains(t, msg.Route(), RouterKey)
	require.Contains(t, msg.Type(), "create_validator")
	require.True(t, len(msg.GetSigners()) == 1, msg)
	require.True(t, len(msg.GetSignBytes()) > 0, msg)

	bz, err := msg.MarshalJSON()
	require.Nil(t, err)
	newMsg := MsgCreateValidator{}
	err2 := newMsg.UnmarshalJSON(bz)
	require.Nil(t, err2)
	require.Equal(t, msg, newMsg)

	err3 := newMsg.UnmarshalJSON(nil)
	require.NotNil(t, err3)
}

// test ValidateBasic for MsgEditValidator
func TestMsgEditValidator(t *testing.T) {
	tests := []struct {
		name, moniker, identity, website, details string
		validatorAddr                             sdk.ValAddress
		expectPass                                bool
	}{
		{"basic good", "a", "b", "c", "d", valAddr1, true},
		{"partial description", "", "", "c", "", valAddr1, true},
		{"empty description", "", "", "", "", valAddr1, false},
		{"empty address", "a", "b", "c", "d", emptyAddr, false},
	}

	for _, tc := range tests {
		description := NewDescription(tc.moniker, tc.identity, tc.website, tc.details)
		msg := NewMsgEditValidator(tc.validatorAddr, description)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "edit_validator")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

func checkMsg(t *testing.T, msg sdk.Msg, expType string) {
	require.Contains(t, msg.Route(), RouterKey)
	require.Contains(t, msg.Type(), expType)
	require.True(t, len(msg.GetSigners()) == 1, msg)
	require.True(t, len(msg.GetSignBytes()) > 0, msg)
}

// test ValidateBasic for MsgDeposit
func TestMsgDeposit(t *testing.T) {

	coinPos := sdk.NewDecCoinFromDec(sdk.DefaultBondDenom, sdk.NewDec(1000))
	coinZero := sdk.NewDecCoinFromDec(sdk.DefaultBondDenom, sdk.ZeroDec())

	tests := []struct {
		name          string
		delegatorAddr sdk.AccAddress
		amount        sdk.SysCoin
		expectPass    bool
	}{
		{"basic good", dlgAddr1, coinPos, true},
		{"empty delegator", sdk.AccAddress(emptyAddr), coinPos, false},
		{"empty bond", sdk.AccAddress(addr1), coinZero, false},
	}

	for _, tc := range tests {
		msg := NewMsgDeposit(tc.delegatorAddr, tc.amount)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "deposit")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

// test ValidateBasic for MsgWithdraw
func TestMsgWithdraw(t *testing.T) {

	coinPos := sdk.NewDecCoinFromDec(sdk.DefaultBondDenom, sdk.NewDec(1000))
	coinNeg := sdk.SysCoin{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDec(-1)}

	tests := []struct {
		name          string
		delegatorAddr sdk.AccAddress
		amount        sdk.SysCoin
		expectPass    bool
	}{
		{"basic good", dlgAddr1, coinPos, true},
		{"empty delegator", sdk.AccAddress(emptyAddr), coinPos, false},
		{"negative bond", sdk.AccAddress(addr1), coinNeg, false},
	}

	for _, tc := range tests {
		msg := NewMsgWithdraw(tc.delegatorAddr, tc.amount)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "withdraw")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

// test ValidateBasic for MsgDestroyValidator
func TestMsgBindProxy(t *testing.T) {

	tests := []struct {
		name       string
		dlgAddr    sdk.AccAddress
		valAddr    sdk.AccAddress
		expectPass bool
	}{
		{"basic good", dlgAddr1, dlgAddr2, true},
		{"empty delegator", emptyAddr.Bytes(), dlgAddr2, false},
		{"bind to self", dlgAddr1, dlgAddr1, false},
	}

	for _, tc := range tests {
		msg := NewMsgBindProxy(tc.dlgAddr, tc.valAddr)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "bind_proxy")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

// test TestMsgUnbindProxy for MsgDestroyValidator
func TestMsgUnbindProxy(t *testing.T) {

	tests := []struct {
		name       string
		valAddr    sdk.AccAddress
		expectPass bool
	}{
		{"basic good", dlgAddr1, true},
		{"empty validator", sdk.AccAddress(emptyAddr), false},
	}

	for _, tc := range tests {
		msg := NewMsgUnbindProxy(tc.valAddr)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "unbind_proxy")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}
}

func TestMsgRegProxy(t *testing.T) {

	tests := []struct {
		name       string
		dlgAddr    sdk.AccAddress
		doReg      bool
		expectPass bool
	}{
		{"success register", dlgAddr1, true, true},
		{"success unregister", dlgAddr1, false, true},
		{"empty delegator", sdk.AccAddress(emptyAddr), true, false},
	}

	for _, tc := range tests {
		msg := NewMsgRegProxy(tc.dlgAddr, tc.doReg)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "reg_or_unreg_proxy")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}

}

func TestMsgAddShares(t *testing.T) {

	tests := []struct {
		name       string
		dlgAddr    sdk.AccAddress
		valAddrs   []sdk.ValAddress
		expectPass bool
	}{
		{"basic good", dlgAddr1, []sdk.ValAddress{valAddr1}, true},
		{"basic good2", dlgAddr1, []sdk.ValAddress{valAddr1, valAddr2}, true},
		{"duplicate", dlgAddr1, []sdk.ValAddress{valAddr1, valAddr2, valAddr1}, false},
		{"empty validator", dlgAddr1, nil, false},
		{"empty delegator", nil, []sdk.ValAddress{valAddr1}, false},
	}

	for _, tc := range tests {
		msg := NewMsgAddShares(tc.dlgAddr, tc.valAddrs)
		if tc.expectPass {
			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
			checkMsg(t, msg, "add_shares_to_validators")
		} else {
			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
		}
	}

}

//// test ValidateBasic for MsgUnbond
//func TestMsgBeginRedelegate(t *testing.T) {
//	tests := []struct {
//		name             string
//		delegatorAddr    sdk.AccAddress
//		validatorSrcAddr sdk.ValAddress
//		validatorDstAddr sdk.ValAddress
//		amount           sdk.Coin
//		expectPass       bool
//	}{
//		{"regular", sdk.AccAddress(addr1), addr2, addr3, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), true},
//		{"zero amount", sdk.AccAddress(addr1), addr2, addr3, sdk.NewInt64Coin(sdk.DefaultBondDenom, 0), false},
//		{"empty delegator", sdk.AccAddress(emptyAddr), addr1, addr3, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), false},
//		{"empty source validator", sdk.AccAddress(addr1), emptyAddr, addr3, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), false},
//		{"empty destination validator", sdk.AccAddress(addr1), addr2, emptyAddr, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), false},
//	}
//
//	for _, tc := range tests {
//		msg := NewMsgBeginRedelegate(tc.delegatorAddr, tc.validatorSrcAddr, tc.validatorDstAddr, tc.amount)
//		if tc.expectPass {
//			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
//		} else {
//			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
//		}
//	}
//}
//
//// test ValidateBasic for MsgUnbond
//func TestMsgUndelegate(t *testing.T) {
//	tests := []struct {
//		name          string
//		delegatorAddr sdk.AccAddress
//		validatorAddr sdk.ValAddress
//		amount        sdk.Coin
//		expectPass    bool
//	}{
//		{"regular", sdk.AccAddress(addr1), addr2, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), true},
//		{"zero amount", sdk.AccAddress(addr1), addr2, sdk.NewInt64Coin(sdk.DefaultBondDenom, 0), false},
//		{"empty delegator", sdk.AccAddress(emptyAddr), addr1, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), false},
//		{"empty validator", sdk.AccAddress(addr1), emptyAddr, sdk.NewInt64Coin(sdk.DefaultBondDenom, 1), false},
//	}
//
//	for _, tc := range tests {
//		msg := NewMsgUndelegate(tc.delegatorAddr, tc.validatorAddr, tc.amount)
//		if tc.expectPass {
//			require.Nil(t, msg.ValidateBasic(), "test: %v", tc.name)
//		} else {
//			require.NotNil(t, msg.ValidateBasic(), "test: %v", tc.name)
//		}
//	}
//}
