package types

import (
	"testing"
	"time"

	"github.com/okex/exchain/libs/tendermint/libs/kv"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/go-amino"
)

var cdc = amino.NewCodec()

func TestEventAmino(t *testing.T) {
	var events = []Event{
		{},
		{
			Type: "test",
		},
		{
			Attributes: []kv.Pair{
				{Key: []byte("key"), Value: []byte("value")},
				{Key: []byte("key2"), Value: []byte("value2")},
			},
		},
		{
			Type: "test",
			Attributes: []kv.Pair{
				{Key: []byte("key"), Value: []byte("value")},
				{Key: []byte("key2"), Value: []byte("value2")},
				{},
			},
		},
		{
			Attributes: []kv.Pair{},
		},
	}

	for _, event := range events {
		expect, err := cdc.MarshalBinaryBare(event)
		require.NoError(t, err)

		actual, err := MarshalEventToAmino(event)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestPubKeyAmino(t *testing.T) {
	var pubkeys = []PubKey{
		{},
		{Type: "type"},
		{Data: []byte("testdata")},
		{
			Type: "test",
			Data: []byte("data"),
		},
	}

	for _, pubkey := range pubkeys {
		expect, err := cdc.MarshalBinaryBare(pubkey)
		require.NoError(t, err)

		actual, err := MarshalPubKeyToAmino(pubkey)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestValidatorUpdateAmino(t *testing.T) {
	var validatorUpdates = []ValidatorUpdate{
		{},
		{
			PubKey: PubKey{
				Type: "test",
			},
		},
		{
			PubKey: PubKey{
				Type: "test",
				Data: []byte("data"),
			},
		},
		{
			Power: 100,
		},
		{
			PubKey: PubKey{
				Type: "test",
				Data: []byte("data"),
			},
			Power: 100,
		},
	}

	for _, validatorUpdate := range validatorUpdates {
		expect, err := cdc.MarshalBinaryBare(validatorUpdate)
		require.NoError(t, err)

		actual, err := MarshalValidatorUpdateToAmino(validatorUpdate)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestBlockParamsAmino(t *testing.T) {
	tests := []BlockParams{
		{
			MaxBytes: 100,
			MaxGas:   200,
		},
		{
			MaxBytes: -100,
			MaxGas:   -200,
		},
	}

	for _, test := range tests {
		expect, err := cdc.MarshalBinaryBare(test)
		require.NoError(t, err)

		actual, err := MarshalBlockParamsToAmino(test)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestEvidenceParamsAmino(t *testing.T) {
	tests := []EvidenceParams{
		{
			MaxAgeNumBlocks: 100,
			MaxAgeDuration:  1000 * time.Second,
		},
		{
			MaxAgeNumBlocks: -100,
			MaxAgeDuration:  time.Second,
		},
	}

	for _, test := range tests {
		expect, err := cdc.MarshalBinaryBare(test)
		require.NoError(t, err)

		actual, err := MarshalEvidenceParamsToAmino(test)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestValidatorParamsAmino(t *testing.T) {
	tests := []ValidatorParams{
		{},
		{
			PubKeyTypes: []string{},
		},
		{
			PubKeyTypes: []string{""},
		},
		{
			PubKeyTypes: []string{"ed25519"},
		},
		{
			PubKeyTypes: []string{"ed25519", "ed25519"},
		},
	}

	for _, test := range tests {
		expect, err := cdc.MarshalBinaryBare(test)
		require.NoError(t, err)

		actual, err := MarshalValidatorParamsToAmino(test)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestConsensusParamsAmino(t *testing.T) {
	tests := []ConsensusParams{
		{
			Block:     &BlockParams{},
			Evidence:  &EvidenceParams{},
			Validator: &ValidatorParams{},
		},
		{
			Block: &BlockParams{
				MaxBytes: 100,
			},
			Evidence: &EvidenceParams{
				MaxAgeDuration: 5 * time.Second,
			},
			Validator: &ValidatorParams{
				PubKeyTypes: nil,
			},
		},
		{
			Validator: &ValidatorParams{
				PubKeyTypes: []string{"ed25519"},
			},
		},
		{
			Block: &BlockParams{
				MaxBytes: 100,
				MaxGas:   200,
			},
			Evidence: &EvidenceParams{
				MaxAgeNumBlocks: 500,
				MaxAgeDuration:  6 * time.Second,
			},
			Validator: &ValidatorParams{
				PubKeyTypes: []string{},
			},
		},
	}

	for _, test := range tests {
		expect, err := cdc.MarshalBinaryBare(test)
		require.NoError(t, err)

		actual, err := MarshalConsensusParamsToAmino(test)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestResponseDeliverTxAmino(t *testing.T) {
	var resps = []*ResponseDeliverTx{
		//{},
		{123, nil, "", "", 0, 0, nil, "", struct{}{}, nil, 0},
		{Code: 123, Data: []byte(""), Log: "log123", Info: "123info", GasWanted: 1234445, GasUsed: 98, Events: nil, Codespace: "sssdasf"},
		{Code: 0, Data: []byte("data"), Info: "info"},
		{Events: []Event{{}, {Type: "Event"}}},
	}

	for _, resp := range resps {
		expect, err := cdc.MarshalBinaryBare(resp)
		require.NoError(t, err)

		actual, err := MarshalResponseDeliverTxToAmino(resp)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestResponseBeginBlockAmino(t *testing.T) {
	var resps = []*ResponseBeginBlock{
		{},
		{
			Events: []Event{
				{
					Type: "test",
				},
			},
		},
		{
			Events: []Event{},
		},
		{
			Events: []Event{{}},
		},
	}
	for _, resp := range resps {
		expect, err := cdc.MarshalBinaryBare(resp)
		require.NoError(t, err)

		actual, err := MarshalResponseBeginBlockToAmino(resp)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}

func TestResponseEndBlockAmino(t *testing.T) {
	var resps = []*ResponseEndBlock{
		{},
		{
			ValidatorUpdates: []ValidatorUpdate{
				{
					PubKey: PubKey{
						Type: "test",
					},
				},
			},
			ConsensusParamUpdates: &ConsensusParams{},
			Events:                []Event{},
		},
		{
			ValidatorUpdates:      []ValidatorUpdate{},
			ConsensusParamUpdates: &ConsensusParams{},
			Events:                []Event{{}},
		},
		{
			ValidatorUpdates:      []ValidatorUpdate{{}},
			ConsensusParamUpdates: &ConsensusParams{Block: &BlockParams{}, Evidence: &EvidenceParams{}, Validator: &ValidatorParams{}},
			Events:                []Event{{}, {Type: "Event"}, {}},
		},
	}
	for _, resp := range resps {
		expect, err := cdc.MarshalBinaryBare(resp)
		require.NoError(t, err)

		actual, err := MarshalResponseEndBlockToAmino(resp)
		require.NoError(t, err)
		require.EqualValues(t, expect, actual)
	}
}
