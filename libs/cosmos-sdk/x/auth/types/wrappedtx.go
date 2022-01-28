package types

import (
	"fmt"
	ocdc "github.com/okex/exchain/libs/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
)
var (
	_ sdk.Tx = (*WrappedTx)(nil)
)

type RawWrappedTx struct {
	Payload   []byte  `json:"payload"`   // std tx or evm tx
	Metadata  []byte  `json:"metadata"`  // customized message from the node who signs the tx
	Signature []byte  `json:"signature"` // signature for payload+metadata
	NodeKey   []byte  `json:"nodeKey"`   // pub key of the node who signs the tx
}

type WrappedTx struct {
	sdk.Tx
	*RawWrappedTx
}

func (msg WrappedTx) String() string {
	return fmt.Sprintf("StdTx=<%s>, Metadata=<%s>, Signature=<%s>, NodeKey=<%s>",
		msg.Tx,
		string(msg.Metadata),
		string(msg.Signature),
		string(msg.NodeKey),
		)
}

func (wtx WrappedTx) GetPayloadTx() sdk.Tx {
	return wtx.Tx
}

func (wtx WrappedTx) GetPayloadTxBytes() []byte {
	return wtx.Payload
}

func (tx WrappedTx) GetType() sdk.TransactionType {
	return sdk.WrappedTxType
}

func EncodeWrappedTx(payload []byte, info *sdk.ExTxInfo) ([]byte, error) {

	wrapped := &RawWrappedTx{
		Payload: payload,
		NodeKey: info.NodeKey,
		Signature: info.Signature,
		Metadata: info.Metadata,
	}

	return ocdc.Encode(wrapped)
}

func DecodeWrappedTx(txbytes []byte, payloadDecoder sdk.TxDecoder, heights ...int64) (sdk.Tx, error) {

	raw := &RawWrappedTx{}
	err := ocdc.Decode(txbytes, raw)
	if err != nil {
		return nil, err
	}

	payloadTx, err := payloadDecoder(raw.Payload, heights...)
	if err != nil {
		return nil, err
	}

	tx := WrappedTx {
		Tx: payloadTx,
		RawWrappedTx: raw,
	}
	return tx, err
}