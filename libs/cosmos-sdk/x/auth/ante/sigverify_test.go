package ante_test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/spf13/viper"
	"testing"

	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/crypto/ed25519"
	"github.com/okex/exchain/libs/tendermint/crypto/multisig"
	"github.com/okex/exchain/libs/tendermint/crypto/secp256k1"
	"github.com/stretchr/testify/require"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/cosmos-sdk/types/tx/signing"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/ante"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"

	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	"github.com/okex/exchain/libs/tendermint/crypto/etherhash"
	types2 "github.com/okex/exchain/libs/tendermint/types"
)

func TestSetPubKey(t *testing.T) {
	// setup
	app, ctx := createTestApp(true)

	// keys and addresses
	priv1, pub1, addr1 := types.KeyTestPubAddr()
	priv2, pub2, addr2 := types.KeyTestPubAddr()
	priv3, pub3, addr3 := types.KeyTestPubAddr()

	addrs := []sdk.AccAddress{addr1, addr2, addr3}
	pubs := []crypto.PubKey{pub1, pub2, pub3}

	msgs := make([]sdk.Msg, len(addrs))
	// set accounts and create msg for each address
	for i, addr := range addrs {
		acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
		require.NoError(t, acc.SetAccountNumber(uint64(i)))
		app.AccountKeeper.SetAccount(ctx, acc)
		msgs[i] = types.NewTestMsg(addr)
	}

	fee := types.NewTestStdFee()

	privs, accNums, seqs := []crypto.PrivKey{priv1, priv2, priv3}, []uint64{0, 1, 2}, []uint64{0, 0, 0}
	tx := types.NewTestTx(ctx, msgs, privs, accNums, seqs, fee)

	spkd := ante.NewSetPubKeyDecorator(app.AccountKeeper)
	antehandler := sdk.ChainAnteDecorators(spkd)

	ctx, err := antehandler(ctx, tx, false)
	require.Nil(t, err)

	// Require that all accounts have pubkey set after Decorator runs
	for i, addr := range addrs {
		pk, err := app.AccountKeeper.GetPubKey(ctx, addr)
		require.Nil(t, err, "Error on retrieving pubkey from account")
		require.Equal(t, pubs[i], pk, "Pubkey retrieved from account is unexpected")
	}
}

func TestConsumeSignatureVerificationGas(t *testing.T) {
	params := types.DefaultParams()
	msg := []byte{1, 2, 3, 4}

	pkSet1, sigSet1 := generatePubKeysAndSignatures(5, msg, false)
	multisigKey1 := multisig.NewPubKeyMultisigThreshold(2, pkSet1)
	multisignature1 := multisig.NewMultisig(len(pkSet1))
	expectedCost1 := expectedGasCostByKeys(pkSet1)
	for i := 0; i < len(pkSet1); i++ {
		multisignature1.AddSignatureFromPubKey(sigSet1[i], pkSet1[i], pkSet1)
	}

	type args struct {
		meter  sdk.GasMeter
		sig    []byte
		pubkey crypto.PubKey
		params types.Params
	}
	tests := []struct {
		name        string
		args        args
		gasConsumed uint64
		shouldErr   bool
	}{
		{"PubKeyEd25519", args{sdk.NewInfiniteGasMeter(), nil, ed25519.GenPrivKey().PubKey(), params}, types.DefaultSigVerifyCostED25519, true},
		{"PubKeySecp256k1", args{sdk.NewInfiniteGasMeter(), nil, secp256k1.GenPrivKey().PubKey(), params}, types.DefaultSigVerifyCostSecp256k1, false},
		{"Multisig", args{sdk.NewInfiniteGasMeter(), multisignature1.Marshal(), multisigKey1, params}, expectedCost1, false},
		{"unknown key", args{sdk.NewInfiniteGasMeter(), nil, nil, params}, 0, true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ante.DefaultSigVerificationGasConsumer(tt.args.meter, tt.args.sig, tt.args.pubkey, tt.args.params)

			if tt.shouldErr {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, tt.gasConsumed, tt.args.meter.GasConsumed(), fmt.Sprintf("%d != %d", tt.gasConsumed, tt.args.meter.GasConsumed()))
			}
		})
	}
}

func TestSigVerification(t *testing.T) {
	// setup
	app, ctx := createTestApp(true)
	// make block height non-zero to ensure account numbers part of signBytes
	ctx.SetBlockHeight(1)

	// keys and addresses
	priv1, _, addr1 := types.KeyTestPubAddr()
	priv2, _, addr2 := types.KeyTestPubAddr()
	priv3, _, addr3 := types.KeyTestPubAddr()

	addrs := []sdk.AccAddress{addr1, addr2, addr3}

	msgs := make([]sdk.Msg, len(addrs))
	// set accounts and create msg for each address
	for i, addr := range addrs {
		acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
		require.NoError(t, acc.SetAccountNumber(uint64(i)))
		app.AccountKeeper.SetAccount(ctx, acc)
		msgs[i] = types.NewTestMsg(addr)
	}

	fee := types.NewTestStdFee()

	spkd := ante.NewSetPubKeyDecorator(app.AccountKeeper)
	svd := ante.NewSigVerificationDecorator(app.AccountKeeper)
	antehandler := sdk.ChainAnteDecorators(spkd, svd)

	type testCase struct {
		name      string
		privs     []crypto.PrivKey
		accNums   []uint64
		seqs      []uint64
		recheck   bool
		shouldErr bool
	}
	testCases := []testCase{
		{"no signers", []crypto.PrivKey{}, []uint64{}, []uint64{}, false, true},
		{"not enough signers", []crypto.PrivKey{priv1, priv2}, []uint64{0, 1}, []uint64{0, 0}, false, true},
		{"wrong order signers", []crypto.PrivKey{priv3, priv2, priv1}, []uint64{2, 1, 0}, []uint64{0, 0, 0}, false, true},
		{"wrong accnums", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{7, 8, 9}, []uint64{0, 0, 0}, false, true},
		{"wrong sequences", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{0, 1, 2}, []uint64{3, 4, 5}, false, true},
		{"valid tx", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{0, 1, 2}, []uint64{0, 0, 0}, false, false},
		{"no err on recheck", []crypto.PrivKey{}, []uint64{}, []uint64{}, true, false},
	}
	for i, tc := range testCases {
		ctx.SetIsReCheckTx(tc.recheck)

		tx := types.NewTestTx(ctx, msgs, tc.privs, tc.accNums, tc.seqs, fee)

		_, err := antehandler(ctx, tx, false)
		if tc.shouldErr {
			require.NotNil(t, err, "TestCase %d: %s did not error as expected", i, tc.name)
		} else {
			require.Nil(t, err, "TestCase %d: %s errored unexpectedly. Err: %v", i, tc.name, err)
		}
	}
}

func TestIbcSignModeSigVerify(t *testing.T) {
	app, ctx := createTestApp(true)
	ctx.SetBlockHeight(1)
	priv, _, addr := types.KeyTestPubAddr()
	app.AccountKeeper.SetAccount(ctx, app.AccountKeeper.NewAccountWithAddress(ctx, addr))
	handler := sdk.ChainAnteDecorators(ante.NewSetPubKeyDecorator(app.AccountKeeper), ante.NewSigVerificationDecorator(app.AccountKeeper))

	type testCase struct {
		name     string
		simulate bool
		signMode signing.SignMode
		err      error
	}
	testCases := []testCase{
		{
			"sign mode unspecified, error",
			false,
			signing.SignMode_SIGN_MODE_UNSPECIFIED,
			sdkerrors.Wrap(sdkerrors.ErrUnauthorized, "signature verification failed"),
		}, {
			"sign mode unspecified, success",
			true,
			signing.SignMode_SIGN_MODE_UNSPECIFIED,
			nil,
		}, {
			"sign mode legacy amino, error",
			false,
			signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON,
			sdkerrors.Wrap(sdkerrors.ErrUnauthorized, "signature verification failed"),
		}, {
			"sign mode legacy amino, success",
			true,
			signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON,
			nil,
		},
	}
	for _, tc := range testCases {
		stdTx := types.NewTestTx(ctx, []sdk.Msg{types.NewTestMsg(addr)}, []crypto.PrivKey{priv}, []uint64{0}, []uint64{0}, types.NewTestStdFee())
		tx := fakeIbcTx(stdTx, []signing.SignMode{tc.signMode}, types.IbcFee{}, []uint64{0})
		_, err := handler(ctx, tx, tc.simulate)
		if tc.err == nil {
			require.Equal(t, tc.err, err)
		} else {
			require.Equal(t, tc.err.Error()[0:30], err.Error()[0:30])
		}
	}
}

func fakeIbcTx(stdTx sdk.Tx, signMode []signing.SignMode, sigFee types.IbcFee, sequences []uint64) *types.IbcTx {
	return &types.IbcTx{
		StdTx:     stdTx.(*types.StdTx),
		SignMode:  signMode,
		SigFee:    sigFee,
		Sequences: sequences,
	}
}

func TestSigIntegration(t *testing.T) {
	// generate private keys
	privs := []crypto.PrivKey{secp256k1.GenPrivKey(), secp256k1.GenPrivKey(), secp256k1.GenPrivKey()}

	params := types.DefaultParams()
	initialSigCost := params.SigVerifyCostSecp256k1
	initialCost, err := runSigDecorators(t, params, false, privs...)
	require.Nil(t, err)

	params.SigVerifyCostSecp256k1 *= 2
	doubleCost, err := runSigDecorators(t, params, false, privs...)
	require.Nil(t, err)

	require.Equal(t, initialSigCost*uint64(len(privs)), doubleCost-initialCost)
}

func runSigDecorators(t *testing.T, params types.Params, multisig bool, privs ...crypto.PrivKey) (sdk.Gas, error) {
	// setup
	app, ctx := createTestApp(true)
	// Make block-height non-zero to include accNum in SignBytes
	ctx.SetBlockHeight(1)
	app.AccountKeeper.SetParams(ctx, params)

	msgs := make([]sdk.Msg, len(privs))
	accNums := make([]uint64, len(privs))
	seqs := make([]uint64, len(privs))
	// set accounts and create msg for each address
	for i, priv := range privs {
		addr := sdk.AccAddress(priv.PubKey().Address())
		acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
		require.NoError(t, acc.SetAccountNumber(uint64(i)))
		app.AccountKeeper.SetAccount(ctx, acc)
		msgs[i] = types.NewTestMsg(addr)
		accNums[i] = uint64(i)
		seqs[i] = uint64(0)
	}

	fee := types.NewTestStdFee()

	tx := types.NewTestTx(ctx, msgs, privs, accNums, seqs, fee)

	spkd := ante.NewSetPubKeyDecorator(app.AccountKeeper)
	svgc := ante.NewSigGasConsumeDecorator(app.AccountKeeper, ante.DefaultSigVerificationGasConsumer)
	svd := ante.NewSigVerificationDecorator(app.AccountKeeper)
	antehandler := sdk.ChainAnteDecorators(spkd, svgc, svd)

	// Determine gas consumption of antehandler with default params
	before := ctx.GasMeter().GasConsumed()
	ctx, err := antehandler(ctx, tx, false)
	after := ctx.GasMeter().GasConsumed()

	return after - before, err
}

func TestJudgeIncontinuousNonce(t *testing.T) {
	app, ctx := createTestApp(true)
	_, _, addr := types.KeyTestPubAddr()
	acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
	require.NoError(t, acc.SetSequence(uint64(50)))
	app.AccountKeeper.SetAccount(ctx, acc)

	isd := ante.NewIncrementSequenceDecorator(app.AccountKeeper)

	testCases := []struct {
		ctx      sdk.Context
		simulate bool
		txNonce  uint64

		result bool
	}{
		{ctx.WithIsReCheckTx(true), true, 1, false},
		{ctx.WithIsCheckTx(true).WithIsReCheckTx(false), false, 2, true},
		{ctx.WithIsCheckTx(true).WithIsReCheckTx(false), false, 50, false},
		{ctx.WithIsCheckTx(true), true, 4, false},
		{ctx.WithIsCheckTx(true), false, 0, false},
	}

	for _, tc := range testCases {
		tx := &types.StdTx{}
		tx.Nonce = tc.txNonce
		re := isd.JudgeIncontinuousNonce(ctx, tx, []sdk.AccAddress{acc.GetAddress()}, tc.simulate)
		assert.Equal(t, tc.result, re)
	}
}

func TestIncrementSequenceDecorator(t *testing.T) {
	app, ctx := createTestApp(true)

	priv, _, addr := types.KeyTestPubAddr()
	acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
	require.NoError(t, acc.SetAccountNumber(uint64(50)))
	app.AccountKeeper.SetAccount(ctx, acc)

	msgs := []sdk.Msg{types.NewTestMsg(addr)}
	privKeys := []crypto.PrivKey{priv}
	accNums := []uint64{app.AccountKeeper.GetAccount(ctx, addr).GetAccountNumber()}
	accSeqs := []uint64{app.AccountKeeper.GetAccount(ctx, addr).GetSequence()}
	fee := types.NewTestStdFee()
	tx := types.NewTestTx(ctx, msgs, privKeys, accNums, accSeqs, fee)

	isd := ante.NewIncrementSequenceDecorator(app.AccountKeeper)
	antehandler := sdk.ChainAnteDecorators(isd)

	testCases := []struct {
		ctx         sdk.Context
		simulate    bool
		expectedSeq uint64
	}{
		{ctx.WithIsReCheckTx(true), false, 1},
		{ctx.WithIsCheckTx(true).WithIsReCheckTx(false), false, 2},
		{ctx.WithIsReCheckTx(true), false, 3},
		{ctx.WithIsReCheckTx(true), false, 4},
		{ctx.WithIsReCheckTx(true), true, 5},
	}

	for i, tc := range testCases {
		_, err := antehandler(tc.ctx, tx, tc.simulate)
		require.NoError(t, err, "unexpected error; tc #%d, %v", i, tc)
		require.Equal(t, tc.expectedSeq, app.AccountKeeper.GetAccount(ctx, addr).GetSequence())
	}
}

func TestVerifySig(t *testing.T) {
	// setup
	app, ctx := createTestApp(true)
	// make block height non-zero to ensure account numbers part of signBytes
	ctx.SetBlockHeight(1)
	viper.SetDefault(tmtypes.FlagSigCacheSize, 30000)
	tmtypes.InitSignatureCache()

	// keys and addresses
	priv1, _, addr1 := types.KeyTestPubAddr()
	priv2, _, addr2 := types.KeyTestPubAddr()
	priv3, _, addr3 := types.KeyTestPubAddr()

	addrs := []sdk.AccAddress{addr1, addr2, addr3}
	msgList := [][]sdk.Msg{}
	for i := range addrs {
		msgs := []sdk.Msg{types.NewTestMsg(addrs[i])}
		acc := app.AccountKeeper.NewAccountWithAddress(ctx, addrs[i])
		require.NoError(t, acc.SetAccountNumber(uint64(i)))
		app.AccountKeeper.SetAccount(ctx, acc)
		msgList = append(msgList, msgs)
	}

	fee := types.NewTestStdFee()
	spkd := ante.NewSetPubKeyDecorator(app.AccountKeeper)
	svd := ante.NewSigVerificationDecorator(app.AccountKeeper)
	antehandler := sdk.ChainAnteDecorators(spkd, svd)

	type testCase struct {
		name      string
		privs     []crypto.PrivKey
		seqs      []uint64
		shouldErr []bool
	}
	testCases := []testCase{
		{"error priv", []crypto.PrivKey{priv3, priv1, priv2}, []uint64{0, 0, 0}, []bool{true, true, true}},
		{"error seq", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{1, 2, 3}, []bool{true, true, true}},
		{"valid tx", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{0, 0, 0}, []bool{false, false, false}},
		{"error priv", []crypto.PrivKey{priv3, priv1, priv2}, []uint64{0, 0, 0}, []bool{true, true, true}},
		{"error priv", []crypto.PrivKey{priv3, priv1, priv2}, []uint64{1, 1, 1}, []bool{true, true, true}},
		{"error seq", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{2, 2, 2}, []bool{true, true, true}},
		{"error seq", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{0, 0, 0}, []bool{true, true, true}},
		{"valid tx", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{1, 1, 1}, []bool{false, false, false}},
		{"valid tx", []crypto.PrivKey{priv1, priv2, priv3}, []uint64{2, 2, 2}, []bool{false, false, false}},
		{"1 valid tx", []crypto.PrivKey{priv3, priv2, priv1}, []uint64{3, 3, 3}, []bool{true, false, true}},
	}

	for caseI, tc := range testCases {
		for n := range addrs {
			sigs := NewSig(ctx, msgList[n], tc.privs[n], uint64(n), tc.seqs[n], fee)
			tx := NewTestTx(msgList[n], sigs, fee)
			sigTx, _ := tx.(ante.SigVerifiableTx)
			signerAddrs := sigTx.GetSigners()
			signerAccs := make([]exported.Account, len(signerAddrs))

			//first check
			_, err := antehandler(ctx, tx, false)
			for i, sig := range sigs {
				signerAccs[i], _ = ante.GetSignerAcc(ctx, app.AccountKeeper, signerAddrs[i])
				signBytes := sigTx.GetSignBytes(ctx, i, signerAccs[i])
				_, ok := types2.SignatureCache().Get(etherhash.Sum(append(signBytes, sig.Signature...)))
				require.Equal(t, !tc.shouldErr[n], ok)
			}

			//second check
			_, err = antehandler(ctx, tx, false)
			if tc.shouldErr[n] {
				require.NotNil(t, err, "TestCase %d: %s did not error as expected", caseI, tc.name)
			} else {
				require.Nil(t, err, "TestCase %d: %s errored unexpectedly. Err: %v", caseI, tc.name, err)
				acc := app.AccountKeeper.GetAccount(ctx, addrs[n])
				acc.SetSequence(acc.GetSequence() + 1)
				app.AccountKeeper.SetAccount(ctx, acc)
			}
			for i, sig := range sigs {
				signerAccs[i], _ = ante.GetSignerAcc(ctx, app.AccountKeeper, signerAddrs[i])
				signBytes := sigTx.GetSignBytes(ctx, i, signerAccs[i])
				_, ok := types2.SignatureCache().Get(etherhash.Sum(append(signBytes, sig.Signature...)))
				require.Equal(t, false, ok)
			}
		}
	}
}

func NewSig(ctx sdk.Context, msgs []sdk.Msg, priv crypto.PrivKey, accNum uint64, seq uint64, fee types.StdFee) []types.StdSignature {
	signBytes := types.StdSignBytes(ctx.ChainID(), accNum, seq, fee, msgs, "")
	sig, err := priv.Sign(signBytes)
	if err != nil {
		panic(err)
	}

	sigs := types.StdSignature{PubKey: priv.PubKey(), Signature: sig}
	return []types.StdSignature{sigs}
}

func NewTestTx(msgs []sdk.Msg, sigs []types.StdSignature, fee types.StdFee) sdk.Tx {
	tx := types.NewStdTx(msgs, fee, sigs, "")
	return tx
}

func BenchmarkVerifySig(b *testing.B) {
	app, ctx := createTestApp(true)
	// make block height non-zero to ensure account numbers part of signBytes
	ctx.SetBlockHeight(1)
	viper.SetDefault(tmtypes.FlagSigCacheSize, 30000)
	tmtypes.InitSignatureCache()
	priv, _, addr := types.KeyTestPubAddr()
	msgs := []sdk.Msg{types.NewTestMsg(addr)}
	acc := app.AccountKeeper.NewAccountWithAddress(ctx, addr)
	require.NoError(b, acc.SetAccountNumber(uint64(0)))
	app.AccountKeeper.SetAccount(ctx, acc)
	fee := types.NewTestStdFee()
	anteHandler := sdk.ChainAnteDecorators(ante.NewSetPubKeyDecorator(app.AccountKeeper), ante.NewSigVerificationDecorator(app.AccountKeeper))

	type testCase struct {
		name string
		priv crypto.PrivKey
		seq  uint64
	}
	tc := testCase{"valid tx", priv, 0}
	b.ResetTimer()
	for i := 0; i < 2; i++ {
		sigs := NewSig(ctx, msgs, tc.priv, 0, tc.seq, fee)
		tx := NewTestTx(msgs, sigs, fee)
		_, err := anteHandler(ctx, tx, false)
		require.Nil(b, err, "TestCase %s errored unexpectedly. Err: %v", tc.name, err)
	}
}
