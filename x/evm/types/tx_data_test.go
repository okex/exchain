package types

import (
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/tendermint/go-amino"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/stretchr/testify/require"

	ethcmn "github.com/ethereum/go-ethereum/common"
)

func TestMarshalAndUnmarshalData(t *testing.T) {
	addr := GenerateEthAddress()
	hash := ethcmn.BigToHash(big.NewInt(2))

	txData := TxData{
		AccountNonce: 2,
		Price:        big.NewInt(3),
		GasLimit:     1,
		Recipient:    &addr,
		Amount:       big.NewInt(4),
		Payload:      []byte("test"),
		V:            big.NewInt(5),
		R:            big.NewInt(6),
		S:            big.NewInt(7),
		Hash:         &hash,
	}

	cdc := amino.NewCodec()

	bz, err := txData.MarshalAmino()
	require.NoError(t, err)
	require.NotNil(t, bz)

	var txData2 TxData
	err = txData2.UnmarshalAmino(bz)
	require.NoError(t, err)

	require.Equal(t, txData, txData2)

	var txData3 TxData
	err = txData3.UnmarshalFromAmino(cdc, bz)
	require.NoError(t, err)
	require.Equal(t, txData2, txData3)

	// check error
	err = txData2.UnmarshalAmino(bz[1:])
	require.Error(t, err)
	err = txData3.UnmarshalAmino(bz[1:])
	require.Error(t, err)
}

func TestTxDataAmino(t *testing.T) {
	addr := GenerateEthAddress()
	hash := ethcmn.BigToHash(big.NewInt(2))

	testCases := []TxData{
		{
			AccountNonce: 2,
			Price:        big.NewInt(3),
			GasLimit:     1,
			Recipient:    &addr,
			Amount:       big.NewInt(4),
			Payload:      []byte("test"),
			V:            big.NewInt(5),
			R:            big.NewInt(6),
			S:            big.NewInt(7),
			Hash:         &hash,
		},
		{
			Price:     big.NewInt(math.MinInt64),
			Recipient: &ethcmn.Address{},
			Amount:    big.NewInt(math.MinInt64),
			Payload:   []byte{},
			V:         big.NewInt(math.MinInt64),
			R:         big.NewInt(math.MinInt64),
			S:         big.NewInt(math.MinInt64),
			Hash:      &ethcmn.Hash{},
		},
		{
			AccountNonce: math.MaxUint64,
			Price:        big.NewInt(math.MaxInt64),
			GasLimit:     math.MaxUint64,
			Amount:       big.NewInt(math.MaxInt64),
			V:            big.NewInt(math.MaxInt64),
			R:            big.NewInt(math.MaxInt64),
			S:            big.NewInt(math.MaxInt64),
		},
	}

	cdc := amino.NewCodec()
	RegisterCodec(cdc)

	for _, txData := range testCases {
		expectData, err := cdc.MarshalBinaryBare(txData)
		require.NoError(t, err)

		var expectValue TxData
		err = cdc.UnmarshalBinaryBare(expectData, &expectValue)
		require.NoError(t, err)

		var actualValue TxData
		v, err := cdc.UnmarshalBinaryBareWithRegisteredUnmarshaller(expectData, &actualValue)
		//err = actualValue.UnmarshalFromAmino(expectData)
		require.NoError(t, err)
		actualValue = v.(TxData)

		require.EqualValues(t, expectValue, actualValue)
	}
}

func BenchmarkUnmarshalTxData(b *testing.B) {
	addr := GenerateEthAddress()
	hash := ethcmn.BigToHash(big.NewInt(2))

	txData := TxData{
		AccountNonce: 2,
		Price:        big.NewInt(3),
		GasLimit:     1,
		Recipient:    &addr,
		Amount:       big.NewInt(4),
		Payload:      []byte("test"),
		V:            big.NewInt(5),
		R:            big.NewInt(6),
		S:            big.NewInt(7),
		Hash:         &hash,
	}

	bz, _ := txData.MarshalAmino()
	cdc := amino.NewCodec()

	b.ResetTimer()
	b.ReportAllocs()

	b.Run("amino", func(b *testing.B) {
		var txData2 TxData
		_ = txData2.UnmarshalAmino(bz)
	})

	b.Run("unmarshaller", func(b *testing.B) {
		var txData3 TxData
		_ = txData3.UnmarshalFromAmino(cdc, bz)
	})
}

func TestMsgEthereumTxAmino(t *testing.T) {
	addr := GenerateEthAddress()
	msg := NewMsgEthereumTx(5, &addr, big.NewInt(1), 100000, big.NewInt(3), []byte("test"))

	msg.Data.V = big.NewInt(1)
	msg.Data.R = big.NewInt(2)
	msg.Data.S = big.NewInt(3)

	raw, err := ModuleCdc.MarshalBinaryBare(msg)
	require.NoError(t, err)

	var msg2 MsgEthereumTx

	err = ModuleCdc.UnmarshalBinaryBare(raw, &msg2)
	require.NoError(t, err)
	require.Equal(t, msg, &msg2)
}

func TestTxData_String(t *testing.T) {
	const expectedStrWithoutRecipient = "nonce=2 price=3 gasLimit=1 recipient=nil amount=4 data=0x1234567890abcdef v=5 r=6 s=7"
	payload, err := hexutil.Decode("0x1234567890abcdef")
	require.NoError(t, err)
	txData := TxData{
		AccountNonce: 2,
		Price:        big.NewInt(3),
		GasLimit:     1,
		Amount:       big.NewInt(4),
		Payload:      payload,
		V:            big.NewInt(5),
		R:            big.NewInt(6),
		S:            big.NewInt(7),
	}

	require.True(t, strings.EqualFold(expectedStrWithoutRecipient, txData.String()))

	// add recipient
	const expectedStrWithRecipient = "nonce=2 price=3 gasLimit=1 recipient=0x0000000000000000000000000000000000000000 amount=4 data=0x1234567890abcdef v=5 r=6 s=7"
	expectedEthAddr := ethcmn.HexToAddress("0x0000000000000000000000000000000000000000")
	txData.Recipient = &expectedEthAddr
	require.True(t, strings.EqualFold(expectedStrWithRecipient, txData.String()))
}

func Test_IsInscription(t *testing.T) {
	testCase := []struct {
		name   string
		input  string
		expect bool
	}{
		{
			name:   "eth inscription 0 ",
			input:  "data:,{\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"}",
			expect: true,
		},
		{
			name:   "eth inscription 1 ",
			input:  "data:application/json,{\"p\":\"rerc-20\",\"op\":\"mint\",\"tick\":\"rETH\",\"id\":\"0xa40d770d9055260547b27280135553effd4c1a0e9fa508dfd3866228559fb2ae\",\"amt\":\"10000\"}",
			expect: true,
		},
		{
			name:   "eth inscription 2",
			input:  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACoAAAAqCAIAAABKoV4MAAADtklEQVR4nO2XTUhUURTH/0ZZ9EnkIvKpRTE0ZVn2MW5GU2c07GNRwe0t7AOkksmPyIWLhMKFE7jITKZEKIuYJsgYNGFyxqmGPozKipExUbQYo6AWQkUUUYtjt9fMfW/eCOEiz+p67jnn9z/n3ncHEz4/PYbJs2mTyJ7CT+Gn8FP4/w4/fWJpJQWt0c6W2/v/LZ5T7Rkmtd24RCTo/MWj0kWSlL0oOXp3wUrDWP8AgHsfRzvDYf0idOFLClqLJAmAkM0VAIhXROyrp4cdYdmLkoskqUiShFckDjxnx7Sx/gFqnSvQkxXH1eMj1W80AI0j0MJT60qkgxWXuq44WDEAnQttfTG67wyHlRVpvHzIehYOVqwxAFV8SUEr9dp+308evrj3cVT/Qnkh4sAredzoIPhx6FlwHXHjoyueffFIueXp6gbgvfYdgGXvDHJWtzXwAPOD953hsMbV03p26KudufGURU4stOTGFOrp6q5ua1h26E/kcLN/YcD8yDdHLUWr++CPPRtMayxy4rPnL6tvnCGnfVdFoTVPGB/BNmTMx6HcYfiBrRPBc8tct/b6UDeVrm5uUI5XqGbgyK134ZHsDlvMyqr4rPzPG0xrov3fn30BYDi/DcBws7/QmpdustLWjMzZPGyxtJRiJoiPNkPGfACGhzL3DAPpJmvm+qwlyUlpy1c0NZwRxmiY+M1nrI+vvc5vAOy7Kz02d0tKXUtKHfk9Nrd9VwUPez00CMC952pEzMKAWQMv6J6xvuCnJ/OwNMJv312ZsnMV/7PX4abF8RN1S5K30zrU5lPGvLrsy1p2krE+l2u1Xny0eZ3fLHKiEsmt0Jp3/EQdgI6OjmBPV0TMTXc7gOCnJ4xBqEA8/MDph2lJIx+G2pUKAHi8fo838ikE8Hb0Q4SHIikrfe5GtcZUuy+XbQDOOpsAJC3fYUzN8TrvArDIiaSAP0TBnq50k5VaJzCXCyAtacRR01haWyakCF49xvou3jkHoPdCiDwkwpxbDyD0ZlyEmm4CG1NzAAT8VeWyzbhiZWltmXD44u6/+n6GBvvXHzaOTwI0iSougiYhNA7G7xHOyk9ArThYgHe5VjNWxqkASAcXYc6tJ4aaUdM88avvp1qk+MNz1DRear9MMy+XbXQKXAQp0GZTMCVSHb14bqZ1m/5KdmK8rlMjCSS090JISZ34d08iTMbN41KcABDwiwdAR35wy1EA5bKtJ/QYwIEd+xiL5+qpGZ0oALrMEQoC/ipHTWNosF9/wUn+D/cXUz3PFF/Yc5cAAAAASUVORK5CYII=",
			expect: true,
		},
		{
			name:   "nostandard eth inscription 0 ",
			input:  "nodata:,{\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"}",
			expect: true,
		},
		{
			name:   "nostandard eth inscription 1 ",
			input:  "nodata:application/json,{\"p\":\"rerc-20\",\"op\":\"mint\",\"tick\":\"rETH\",\"id\":\"0xa40d770d9055260547b27280135553effd4c1a0e9fa508dfd3866228559fb2ae\",\"amt\":\"10000\"}",
			expect: true,
		},
		{
			name:   "nostandard eth inscription 2",
			input:  "nodata:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACoAAAAqCAIAAABKoV4MAAADtklEQVR4nO2XTUhUURTH/0ZZ9EnkIvKpRTE0ZVn2MW5GU2c07GNRwe0t7AOkksmPyIWLhMKFE7jITKZEKIuYJsgYNGFyxqmGPozKipExUbQYo6AWQkUUUYtjt9fMfW/eCOEiz+p67jnn9z/n3ncHEz4/PYbJs2mTyJ7CT+Gn8FP4/w4/fWJpJQWt0c6W2/v/LZ5T7Rkmtd24RCTo/MWj0kWSlL0oOXp3wUrDWP8AgHsfRzvDYf0idOFLClqLJAmAkM0VAIhXROyrp4cdYdmLkoskqUiShFckDjxnx7Sx/gFqnSvQkxXH1eMj1W80AI0j0MJT60qkgxWXuq44WDEAnQttfTG67wyHlRVpvHzIehYOVqwxAFV8SUEr9dp+308evrj3cVT/Qnkh4sAredzoIPhx6FlwHXHjoyueffFIueXp6gbgvfYdgGXvDHJWtzXwAPOD953hsMbV03p26KudufGURU4stOTGFOrp6q5ua1h26E/kcLN/YcD8yDdHLUWr++CPPRtMayxy4rPnL6tvnCGnfVdFoTVPGB/BNmTMx6HcYfiBrRPBc8tct/b6UDeVrm5uUI5XqGbgyK134ZHsDlvMyqr4rPzPG0xrov3fn30BYDi/DcBws7/QmpdustLWjMzZPGyxtJRiJoiPNkPGfACGhzL3DAPpJmvm+qwlyUlpy1c0NZwRxmiY+M1nrI+vvc5vAOy7Kz02d0tKXUtKHfk9Nrd9VwUPez00CMC952pEzMKAWQMv6J6xvuCnJ/OwNMJv312ZsnMV/7PX4abF8RN1S5K30zrU5lPGvLrsy1p2krE+l2u1Xny0eZ3fLHKiEsmt0Jp3/EQdgI6OjmBPV0TMTXc7gOCnJ4xBqEA8/MDph2lJIx+G2pUKAHi8fo838ikE8Hb0Q4SHIikrfe5GtcZUuy+XbQDOOpsAJC3fYUzN8TrvArDIiaSAP0TBnq50k5VaJzCXCyAtacRR01haWyakCF49xvou3jkHoPdCiDwkwpxbDyD0ZlyEmm4CG1NzAAT8VeWyzbhiZWltmXD44u6/+n6GBvvXHzaOTwI0iSougiYhNA7G7xHOyk9ArThYgHe5VjNWxqkASAcXYc6tJ4aaUdM88avvp1qk+MNz1DRear9MMy+XbXQKXAQp0GZTMCVSHb14bqZ1m/5KdmK8rlMjCSS090JISZ34d08iTMbN41KcABDwiwdAR35wy1EA5bKtJ/QYwIEd+xiL5+qpGZ0oALrMEQoC/ipHTWNosF9/wUn+D/cXUz3PFF/Yc5cAAAAASUVORK5CYII=",
			expect: false,
		},
		{
			name:   "nostandard eth inscription 3 ",
			input:  "nodata:,}\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"{",
			expect: false,
		},
		{
			name:   "nostandard eth inscription 4 ",
			input:  "nodata:,{\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"",
			expect: false,
		},
		{
			name:   "nostandard eth inscription 5 ",
			input:  "nodata:,\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"}",
			expect: false,
		},
		{
			name:   "nostandard eth inscription 6 ",
			input:  "nodata:,\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"",
			expect: false,
		},
		{
			name:   "nostandard eth inscription 7 ",
			input:  "nodata:,\"p\":\"xrc-20\",\"op\":\"mint\",\"tick\":\"okts\",\"amt\":\"1000\"",
			expect: false,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(hex.EncodeToString([]byte(tc.input)))
			require.NoError(t, err)
			result := IsInscription(data)
			require.Equal(t, tc.expect, result)
		})
	}

}
