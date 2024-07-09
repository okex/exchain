package types

import (
	"fmt"
	"math"
	"testing"

	"github.com/okex/exchain/libs/tendermint/crypto/multisig"

	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/crypto/ed25519"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
)

var (
	priv = ed25519.GenPrivKey()
	addr = sdk.AccAddress(priv.PubKey().Address())
)

func TestStdTx(t *testing.T) {
	msgs := []sdk.Msg{sdk.NewTestMsg(addr)}
	fee := NewTestStdFee()
	sigs := []StdSignature{}

	tx := NewStdTx(msgs, fee, sigs, "")
	require.Equal(t, msgs, tx.GetMsgs())
	require.Equal(t, sigs, tx.Signatures)

	feePayer := tx.GetSigners()[0]
	require.Equal(t, addr, feePayer)
}

func TestStdTxAmino(t *testing.T) {
	cdc := ModuleCdc
	sdk.RegisterCodec(cdc)
	cdc.RegisterConcrete(sdk.TestMsg2{}, "cosmos-sdk/Test2", nil)

	msgs := []sdk.Msg{sdk.NewTestMsg2(addr)}
	fee := NewTestStdFee()
	sigs := []StdSignature{}

	tx := NewStdTx(msgs, fee, sigs, "")

	testCases := []*StdTx{
		{},
		tx,
		{
			Msgs: []sdk.Msg{sdk.NewTestMsg2(addr), sdk.NewTestMsg2(addr), sdk.NewTestMsg2(addr)},
			Fee: StdFee{
				Amount: sdk.NewCoins(sdk.NewInt64Coin("foocoin", 10), sdk.NewInt64Coin("barcoin", 15)),
				Gas:    10000,
			},
			Signatures: []StdSignature{
				{
					PubKey:    priv.PubKey(),
					Signature: []byte{1, 2, 3},
				},
				{
					PubKey:    priv.PubKey(),
					Signature: []byte{2, 3, 4},
				},
				{
					PubKey:    priv.PubKey(),
					Signature: []byte{3, 4, 5},
				},
			},
			Memo: "TestMemo",
		},
		{
			Msgs:       []sdk.Msg{},
			Signatures: []StdSignature{},
			Memo:       "",
		},

		{
			Msgs:       []sdk.Msg{},
			Signatures: []StdSignature{},
			Memo:       "",
			BaseTx: sdk.BaseTx{
				Raw: []byte{1, 2, 3},
			},
		},
	}

	for _, tx := range testCases {
		txBytes, err := cdc.MarshalBinaryBare(tx)
		require.NoError(t, err)

		tx2 := StdTx{}
		err = cdc.UnmarshalBinaryBare(txBytes, &tx2)
		require.NoError(t, err)

		tx3 := StdTx{}
		v, err := cdc.UnmarshalBinaryBareWithRegisteredUnmarshaller(txBytes, &tx3)
		require.NoError(t, err)
		tx3 = *(v.(*StdTx))

		require.EqualValues(t, tx2, tx3)
	}
}

func TestStdSignBytes(t *testing.T) {
	type args struct {
		chainID  string
		accnum   uint64
		sequence uint64
		fee      StdFee
		msgs     []sdk.Msg
		memo     string
	}
	defaultFee := NewTestStdFee()
	tests := []struct {
		args args
		want string
	}{
		{
			args{"1234", 3, 6, defaultFee, []sdk.Msg{sdk.NewTestMsg(addr)}, "memo"},
			fmt.Sprintf("{\"account_number\":\"3\",\"chain_id\":\"1234\",\"fee\":{\"amount\":[{\"amount\":\"150.000000000000000000\",\"denom\":\"atom\"}],\"gas\":\"100000\"},\"memo\":\"memo\",\"msgs\":[[\"%s\"]],\"sequence\":\"6\"}", addr),
		},
	}
	for i, tc := range tests {
		got := string(StdSignBytes(tc.args.chainID, tc.args.accnum, tc.args.sequence, tc.args.fee, tc.args.msgs, tc.args.memo))
		require.Equal(t, tc.want, got, "Got unexpected result on test case i: %d", i)
	}
}

func TestTxValidateBasic(t *testing.T) {
	ctx := sdk.NewContext(nil, abci.Header{ChainID: "mychainid"}, false, log.NewNopLogger())

	// keys and addresses
	priv1, _, addr1 := KeyTestPubAddr()
	priv2, _, addr2 := KeyTestPubAddr()

	// msg and signatures
	msg1 := NewTestMsg(addr1, addr2)
	fee := NewTestStdFee()

	msgs := []sdk.Msg{msg1}

	// require to fail validation upon invalid fee
	badFee := NewTestStdFee()
	badFee.Amount[0].Amount = sdk.NewDec(-5)
	tx := NewTestTx(ctx, nil, nil, nil, nil, badFee)

	err := tx.ValidateBasic()
	require.Error(t, err)
	_, code, _ := sdkerrors.ABCIInfo(err, false)
	require.Equal(t, sdkerrors.ErrInsufficientFee.ABCICode(), code)

	// require to fail validation when no signatures exist
	privs, accNums, seqs := []crypto.PrivKey{}, []uint64{}, []uint64{}
	tx = NewTestTx(ctx, msgs, privs, accNums, seqs, fee)

	err = tx.ValidateBasic()
	require.Error(t, err)
	_, code, _ = sdkerrors.ABCIInfo(err, false)
	require.Equal(t, sdkerrors.ErrNoSignatures.ABCICode(), code)

	// require to fail validation when signatures do not match expected signers
	privs, accNums, seqs = []crypto.PrivKey{priv1}, []uint64{0, 1}, []uint64{0, 0}
	tx = NewTestTx(ctx, msgs, privs, accNums, seqs, fee)

	err = tx.ValidateBasic()
	require.Error(t, err)
	_, code, _ = sdkerrors.ABCIInfo(err, false)
	require.Equal(t, sdkerrors.ErrUnauthorized.ABCICode(), code)

	// require to fail with invalid gas supplied
	badFee = NewTestStdFee()
	badFee.Gas = 9223372036854775808
	tx = NewTestTx(ctx, nil, nil, nil, nil, badFee)

	err = tx.ValidateBasic()
	require.Error(t, err)
	_, code, _ = sdkerrors.ABCIInfo(err, false)
	require.Equal(t, sdkerrors.ErrInvalidRequest.ABCICode(), code)

	// require to pass when above criteria are matched
	privs, accNums, seqs = []crypto.PrivKey{priv1, priv2}, []uint64{0, 1}, []uint64{0, 0}
	tx = NewTestTx(ctx, msgs, privs, accNums, seqs, fee)

	err = tx.ValidateBasic()
	require.NoError(t, err)
}

func TestDefaultTxEncoder(t *testing.T) {
	cdc := codec.New()
	sdk.RegisterCodec(cdc)
	RegisterCodec(cdc)
	cdc.RegisterConcrete(sdk.TestMsg{}, "cosmos-sdk/Test", nil)
	encoder := DefaultTxEncoder(cdc)

	msgs := []sdk.Msg{sdk.NewTestMsg(addr)}
	fee := NewTestStdFee()
	sigs := []StdSignature{}

	tx := NewStdTx(msgs, fee, sigs, "")

	cdcBytes, err := cdc.MarshalBinaryLengthPrefixed(tx)

	require.NoError(t, err)
	encoderBytes, err := encoder(tx)

	require.NoError(t, err)
	require.Equal(t, cdcBytes, encoderBytes)
}

func TestStdSignatureMarshalYAML(t *testing.T) {
	_, pubKey, _ := KeyTestPubAddr()

	testCases := []struct {
		sig    StdSignature
		output string
	}{
		{
			StdSignature{},
			"|\n  pubkey: \"\"\n  signature: \"\"\n",
		},
		{
			StdSignature{PubKey: pubKey, Signature: []byte("dummySig")},
			fmt.Sprintf("|\n  pubkey: %s\n  signature: dummySig\n", sdk.MustBech32ifyPubKey(sdk.Bech32PubKeyTypeAccPub, pubKey)),
		},
		{
			StdSignature{PubKey: pubKey, Signature: nil},
			fmt.Sprintf("|\n  pubkey: %s\n  signature: \"\"\n", sdk.MustBech32ifyPubKey(sdk.Bech32PubKeyTypeAccPub, pubKey)),
		},
	}

	for i, tc := range testCases {
		bz, err := yaml.Marshal(tc.sig)
		require.NoError(t, err)
		require.Equal(t, tc.output, string(bz), "test case #%d", i)
	}
}

func TestStdSignatureAmino(t *testing.T) {
	_, pubKey, _ := KeyTestPubAddr()
	testCases := []StdSignature{
		{},
		{PubKey: pubKey, Signature: []byte("dummySig")},
		{PubKey: multisig.PubKeyMultisigThreshold{}, Signature: []byte{}},
	}

	cdc := ModuleCdc

	for _, stdSig := range testCases {
		expectData, err := cdc.MarshalBinaryBare(stdSig)
		require.NoError(t, err)

		var expectValue StdSignature
		err = cdc.UnmarshalBinaryBare(expectData, &expectValue)
		require.NoError(t, err)

		var actualValue StdSignature
		err = actualValue.UnmarshalFromAmino(cdc, expectData)
		require.NoError(t, err)

		require.EqualValues(t, expectValue, actualValue)
	}
}

func TestStdFeeAmino(t *testing.T) {
	testCases := []StdFee{
		{},
		{
			Amount: sdk.Coins{
				sdk.Coin{
					Denom:  "dummy",
					Amount: sdk.NewDec(5),
				},
				sdk.Coin{
					Denom:  "summy",
					Amount: sdk.NewDec(math.MaxInt64),
				},
				sdk.Coin{
					Denom:  "summy",
					Amount: sdk.Dec{},
				},
			},
			Gas: uint64(5),
		},
		{
			Amount: sdk.Coins{},
			Gas:    math.MaxUint64,
		},
	}

	for _, stdFee := range testCases {
		expectData, err := ModuleCdc.MarshalBinaryBare(stdFee)
		require.NoError(t, err)

		var expectValue StdFee
		err = ModuleCdc.UnmarshalBinaryBare(expectData, &expectValue)
		require.NoError(t, err)

		var actualValue StdFee
		err = actualValue.UnmarshalFromAmino(ModuleCdc, expectData)
		require.NoError(t, err)

		require.EqualValues(t, expectValue, actualValue)
	}
}
