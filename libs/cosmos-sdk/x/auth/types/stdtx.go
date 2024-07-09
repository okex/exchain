package types

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/exported"
	"github.com/okex/exchain/libs/tendermint/crypto"
	cryptoamino "github.com/okex/exchain/libs/tendermint/crypto/encoding/amino"
	"github.com/okex/exchain/libs/tendermint/crypto/multisig"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/tendermint/go-amino"
	yaml "gopkg.in/yaml.v2"
)

var (
	_ sdk.Tx = (*StdTx)(nil)

	maxGasWanted = uint64((1 << 63) - 1)
)

// StdTx is a standard way to wrap a Msg with Fee and Signatures.
// NOTE: the first signature is the fee payer (Signatures must not be nil).
type StdTx struct {
	Msgs       []sdk.Msg      `json:"msg" yaml:"msg"`
	Fee        StdFee         `json:"fee" yaml:"fee"`
	Signatures []StdSignature `json:"signatures" yaml:"signatures"`
	Memo       string         `json:"memo" yaml:"memo"`

	sdk.BaseTx `json:"-" yaml:"-"`
}

func (tx *StdTx) VerifySequence(index int, acc exported.Account) error {
	//this function no use in stdtx, never add anythin in this
	//only new cosmos44 tx will call this to verify sequence

	return nil
}

func (tx *StdTx) UnmarshalFromAmino(cdc *amino.Codec, data []byte) error {
	var dataLen uint64 = 0
	var subData []byte

	for {
		data = data[dataLen:]

		if len(data) == 0 {
			break
		}

		pos, pbType, err := amino.ParseProtoPosAndTypeMustOneByte(data[0])
		if err != nil {
			return err
		}
		data = data[1:]

		if pbType != amino.Typ3_ByteLength {
			return fmt.Errorf("invalid pbType: %v", pbType)
		}

		var n int
		dataLen, n, err = amino.DecodeUvarint(data)
		if err != nil {
			return err
		}
		data = data[n:]
		if len(data) < int(dataLen) {
			return fmt.Errorf("invalid tx data")
		}
		subData = data[:dataLen]

		switch pos {
		case 1:
			var msg sdk.Msg
			v, err := cdc.UnmarshalBinaryBareWithRegisteredUnmarshaller(subData, &msg)
			if err != nil {
				err = cdc.UnmarshalBinaryBare(subData, &msg)
				if err != nil {
					return err
				} else {
					tx.Msgs = append(tx.Msgs, msg)
				}
			} else {
				tx.Msgs = append(tx.Msgs, v.(sdk.Msg))
			}
		case 2:
			if err := tx.Fee.UnmarshalFromAmino(cdc, subData); err != nil {
				return err
			}
		case 3:
			var sig StdSignature
			if err := sig.UnmarshalFromAmino(cdc, subData); err != nil {
				return err
			}
			tx.Signatures = append(tx.Signatures, sig)
		case 4:
			tx.Memo = string(subData)
		default:
			return fmt.Errorf("unexpect feild num %d", pos)
		}
	}
	return nil
}

func NewStdTx(msgs []sdk.Msg, fee StdFee, sigs []StdSignature, memo string) *StdTx {
	return &StdTx{
		Msgs:       msgs,
		Fee:        fee,
		Signatures: sigs,
		Memo:       memo,
	}
}

// GetMsgs returns the all the transaction's messages.
func (tx *StdTx) GetMsgs() []sdk.Msg { return tx.Msgs }

// ValidateBasic does a simple and lightweight validation check that doesn't
// require access to any other information.
func (tx *StdTx) ValidateBasic() error {
	stdSigs := tx.GetSignatures()

	if tx.Fee.Gas > maxGasWanted {
		return sdkerrors.Wrapf(
			sdkerrors.ErrInvalidRequest,
			"invalid gas supplied; %d > %d", tx.Fee.Gas, maxGasWanted,
		)
	}
	if tx.Fee.Amount.IsAnyNegative() {
		return sdkerrors.Wrapf(
			sdkerrors.ErrInsufficientFee,
			"invalid fee provided: %s", tx.Fee.Amount,
		)
	}
	if len(stdSigs) == 0 {
		return sdkerrors.ErrNoSignatures
	}
	if len(stdSigs) != len(tx.GetSigners()) {
		return sdkerrors.Wrapf(
			sdkerrors.ErrUnauthorized,
			"wrong number of signers; expected %d, got %d", tx.GetSigners(), len(stdSigs),
		)
	}

	return nil
}

func (tx *StdTx) ValidWithHeight(h int64) error {
	for _, msg := range tx.Msgs {
		if v, ok := msg.(sdk.HeightSensitive); ok {
			if err := v.ValidWithHeight(h); nil != err {
				return err
			}
		}
	}
	return nil
}

// CountSubKeys counts the total number of keys for a multi-sig public key.
func CountSubKeys(pub crypto.PubKey) int {
	v, ok := pub.(multisig.PubKeyMultisigThreshold)
	if !ok {
		return 1
	}

	numKeys := 0
	for _, subkey := range v.PubKeys {
		numKeys += CountSubKeys(subkey)
	}

	return numKeys
}

// GetSigners returns the addresses that must sign the transaction.
// Addresses are returned in a deterministic order.
// They are accumulated from the GetSigners method for each Msg
// in the order they appear in tx.GetMsgs().
// Duplicate addresses will be omitted.
func (tx *StdTx) GetSigners() []sdk.AccAddress {
	seen := map[string]bool{}
	var signers []sdk.AccAddress
	for _, msg := range tx.GetMsgs() {
		for _, addr := range msg.GetSigners() {
			if !seen[addr.String()] {
				signers = append(signers, addr)
				seen[addr.String()] = true
			}
		}
	}
	return signers
}

func (tx *StdTx) GetType() sdk.TransactionType {
	return sdk.StdTxType
}

// GetMemo returns the memo
func (tx *StdTx) GetMemo() string { return tx.Memo }

// GetSignatures returns the signature of signers who signed the Msg.
// CONTRACT: Length returned is same as length of
// pubkeys returned from MsgKeySigners, and the order
// matches.
// CONTRACT: If the signature is missing (ie the Msg is
// invalid), then the corresponding signature is
// .Empty().
func (tx *StdTx) GetSignatures() [][]byte {
	sigs := make([][]byte, len(tx.Signatures))
	for i, stdSig := range tx.Signatures {
		sigs[i] = stdSig.Signature
	}
	return sigs
}

// GetPubkeys returns the pubkeys of signers if the pubkey is included in the signature
// If pubkey is not included in the signature, then nil is in the slice instead
func (tx *StdTx) GetPubKeys() []crypto.PubKey {
	pks := make([]crypto.PubKey, len(tx.Signatures))
	for i, stdSig := range tx.Signatures {
		pks[i] = stdSig.PubKey
	}
	return pks
}

// GetSignBytes returns the signBytes of the tx for a given signer
func (tx *StdTx) GetSignBytes(ctx sdk.Context, index int, acc exported.Account) []byte {
	genesis := ctx.BlockHeight() == 0
	chainID := ctx.ChainID()
	var accNum uint64
	if !genesis {
		accNum = acc.GetAccountNumber()
	}

	return StdSignBytes(
		chainID, accNum, acc.GetSequence(), tx.Fee, tx.Msgs, tx.Memo, ctx.BlockHeight(),
	)
}

// GetGas returns the Gas in StdFee
func (tx *StdTx) GetGas() uint64 { return tx.Fee.Gas }

// GetFee returns the FeeAmount in StdFee
func (tx *StdTx) GetFee() sdk.Coins { return tx.Fee.Amount }

// FeePayer returns the address that is responsible for paying fee
// StdTx returns the first signer as the fee payer
// If no signers for tx, return empty address
func (tx *StdTx) FeePayer(ctx sdk.Context) sdk.AccAddress {
	if tx.GetSigners() != nil {
		return tx.GetSigners()[0]
	}
	return sdk.AccAddress{}
}

// GetGasPrice return gas price
func (tx *StdTx) GetGasPrice() *big.Int {
	if tx.Fee.Gas == 0 {
		return big.NewInt(0)
	}
	gasPrices := tx.Fee.GasPrices()
	if len(gasPrices) == 0 {
		return big.NewInt(0)
	}
	return tx.Fee.GasPrices()[0].Amount.BigInt()
}

type WasmMsgChecker interface {
	FnSignatureInfo() (string, int, error)
}

func (tx *StdTx) GetTxFnSignatureInfo() ([]byte, int) {
	// hgu can't be right simulated with many Msgs.
	if len(tx.Msgs) != 1 {
		return nil, 0
	}

	fnSign := ""
	deploySize := 0
	for _, msg := range tx.Msgs {
		v, ok := msg.(WasmMsgChecker)
		if !ok {
			break
		}
		fn, size, err := v.FnSignatureInfo()
		if err != nil || len(fn) <= 0 {
			break
		}

		deploySize = size
		fnSign = fn
		break
	}
	return []byte(fnSign), deploySize
}

func (tx *StdTx) GetFrom() string {
	signers := tx.GetSigners()
	if len(signers) == 0 {
		return ""
	}
	return signers[0].String()
}

func (tx *StdTx) GetEthAddr() string {
	signers := tx.GetSigners()
	if len(signers) == 0 {
		return ""
	}
	return ethcmn.BytesToAddress(signers[0]).String()
}

func (tx *StdTx) GetSender(_ sdk.Context) string {
	return tx.GetFrom()
}

func (tx *StdTx) GetNonce() uint64 {
	return tx.Nonce
}

//__________________________________________________________

// StdFee includes the amount of coins paid in fees and the maximum
// gas to be used by the transaction. The ratio yields an effective "gasprice",
// which must be above some miminum to be accepted into the mempool.
type StdFee struct {
	Amount sdk.Coins `json:"amount" yaml:"amount"`
	Gas    uint64    `json:"gas" yaml:"gas"`
}

// NewStdFee returns a new instance of StdFee
func NewStdFee(gas uint64, amount sdk.Coins) StdFee {
	return StdFee{
		Amount: amount,
		Gas:    gas,
	}
}

// Bytes for signing later
func (fee *StdFee) Bytes() []byte {
	// normalize. XXX
	// this is a sign of something ugly
	// (in the lcd_test, client side its null,
	// server side its [])
	if len(fee.Amount) == 0 {
		fee.Amount = sdk.NewCoins()
	}
	bz, err := ModuleCdc.MarshalJSON(fee) // TODO
	if err != nil {
		panic(err)
	}
	return bz
}

// GasPrices returns the gas prices for a StdFee.
//
// NOTE: The gas prices returned are not the true gas prices that were
// originally part of the submitted transaction because the fee is computed
// as fee = ceil(gasWanted * gasPrices).
func (fee *StdFee) GasPrices() sdk.DecCoins {
	// NOTE: here fee.Gas must be greater than 0.
	return sdk.NewDecCoinsFromCoins(fee.Amount...).QuoDec(sdk.NewDec(int64(fee.Gas)))
	//return fee.Amount.QuoDec(sdk.NewDec(int64(fee.Gas)))
}

func (fee *StdFee) UnmarshalFromAmino(cdc *amino.Codec, data []byte) error {
	var dataLen uint64 = 0
	var subData []byte

	for {
		data = data[dataLen:]
		if len(data) == 0 {
			break
		}

		pos, pbType, err := amino.ParseProtoPosAndTypeMustOneByte(data[0])
		if err != nil {
			return err
		}

		data = data[1:]
		if pbType == amino.Typ3_ByteLength {
			var n int
			dataLen, n, err = amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			data = data[n:]
			if len(data) < int(dataLen) {
				return fmt.Errorf("invalid tx data")
			}
			subData = data[:dataLen]
		}

		switch pos {
		case 1:
			var coin sdk.DecCoin
			err = coin.UnmarshalFromAmino(cdc, subData)
			if err != nil {
				return err
			}
			fee.Amount = append(fee.Amount, coin)
		case 2:
			var n int
			fee.Gas, n, err = amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			dataLen = uint64(n)
		default:
			return fmt.Errorf("unexpect feild num %d", pos)
		}
	}
	return nil
}

//__________________________________________________________

// StdSignDoc is replay-prevention structure.
// It includes the result of msg.GetSignBytes(),
// as well as the ChainID (prevent cross chain replay)
// and the Sequence numbers for each signature (prevent
// inchain replay and enforce tx ordering per account).
type StdSignDoc struct {
	AccountNumber uint64            `json:"account_number" yaml:"account_number"`
	ChainID       string            `json:"chain_id" yaml:"chain_id"`
	Fee           json.RawMessage   `json:"fee" yaml:"fee"`
	Memo          string            `json:"memo" yaml:"memo"`
	Msgs          []json.RawMessage `json:"msgs" yaml:"msgs"`
	Sequence      uint64            `json:"sequence" yaml:"sequence"`
}

// StdSignBytes returns the bytes to sign for a transaction.
func StdSignBytes(chainID string, accnum uint64, sequence uint64, fee StdFee, msgs []sdk.Msg, memo string, height int64) []byte {
	msgsBytes := make([]json.RawMessage, 0, len(msgs))
	for _, msg := range msgs {
		msgByte := json.RawMessage(msg.GetSignBytes())
		if height > 0 && !tmtypes.HigherThanJupiter(height) && msg.Type() == "edit_validator" {
			msgByte = []byte(strings.ReplaceAll(string(msgByte), `,"pubkey":null`, ""))
		}
		msgsBytes = append(msgsBytes, msgByte)
	}
	bz, err := ModuleCdc.MarshalJSON(StdSignDoc{
		AccountNumber: accnum,
		ChainID:       chainID,
		Fee:           json.RawMessage(fee.Bytes()),
		Memo:          memo,
		Msgs:          msgsBytes,
		Sequence:      sequence,
	})
	if err != nil {
		panic(err)
	}
	return sdk.MustSortJSON(bz)
}

// StdSignature represents a sig
type StdSignature struct {
	crypto.PubKey `json:"pub_key" yaml:"pub_key"` // optional
	Signature     []byte                          `json:"signature" yaml:"signature"`
}

// DefaultTxDecoder logic for standard transaction decoding
func DefaultTxDecoder(cdc *codec.Codec) sdk.TxDecoder {
	return func(txBytes []byte, heights ...int64) (sdk.Tx, error) {
		if len(heights) > 0 {
			return nil, fmt.Errorf("too many height parameters")
		}
		var tx StdTx

		if len(txBytes) == 0 {
			return nil, sdkerrors.Wrap(sdkerrors.ErrTxDecode, "tx bytes are empty")
		}

		// StdTx.Msg is an interface. The concrete types
		// are registered by MakeTxCodec
		err := cdc.UnmarshalBinaryLengthPrefixed(txBytes, &tx)
		if err != nil {
			return nil, sdkerrors.Wrap(sdkerrors.ErrTxDecode, err.Error())
		}
		tx.BaseTx.Raw = txBytes

		return &tx, nil
	}
}

// DefaultTxEncoder logic for standard transaction encoding
func DefaultTxEncoder(cdc *codec.Codec) sdk.TxEncoder {
	return func(tx sdk.Tx) ([]byte, error) {
		return cdc.MarshalBinaryLengthPrefixed(tx)
	}
}

func EthereumTxEncoder(_ *codec.Codec) sdk.TxEncoder {
	return func(tx sdk.Tx) ([]byte, error) {
		return EthereumTxEncode(tx)
	}
}

func EthereumTxEncode(tx sdk.Tx) ([]byte, error) {
	return rlp.EncodeToBytes(tx)
}

func EthereumTxDecode(b []byte, tx interface{}) error {
	return rlp.DecodeBytes(b, tx)
}

// MarshalYAML returns the YAML representation of the signature.
func (ss StdSignature) MarshalYAML() (interface{}, error) {
	var (
		bz     []byte
		pubkey string
		err    error
	)

	if ss.PubKey != nil {
		pubkey, err = sdk.Bech32ifyPubKey(sdk.Bech32PubKeyTypeAccPub, ss.PubKey)
		if err != nil {
			return nil, err
		}
	}

	bz, err = yaml.Marshal(struct {
		PubKey    string
		Signature string
	}{
		PubKey:    pubkey,
		Signature: fmt.Sprintf("%s", ss.Signature),
	})
	if err != nil {
		return nil, err
	}

	return string(bz), err
}

func (ss *StdSignature) UnmarshalFromAmino(cdc *amino.Codec, data []byte) error {
	var dataLen uint64 = 0
	var subData []byte

	for {
		data = data[dataLen:]

		if len(data) == 0 {
			break
		}

		pos, pbType, err := amino.ParseProtoPosAndTypeMustOneByte(data[0])
		if err != nil {
			return err
		}
		data = data[1:]

		if pbType != amino.Typ3_ByteLength {
			return fmt.Errorf("invalid field type in StdSignature")
		}

		var n int
		dataLen, n, err = amino.DecodeUvarint(data)
		if err != nil {
			return err
		}
		data = data[n:]
		if len(data) < int(dataLen) {
			return fmt.Errorf("invalid tx data")
		}
		subData = data[:dataLen]

		switch pos {
		case 1:
			ss.PubKey, err = cryptoamino.UnmarshalPubKeyFromAmino(cdc, subData)
			if err != nil {
				return err
			}
		case 2:
			ss.Signature = make([]byte, dataLen)
			copy(ss.Signature, subData)
		default:
			return fmt.Errorf("unexpect feild num %d", pos)
		}
	}
	return nil
}
