package dydx

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/okex/exchain/libs/dydx/contracts"
	"github.com/stretchr/testify/require"
)

const (
	orderHex       = "000000000000000000000000000000000000000000000000000000000000000173646861000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000004000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb9226600000000000000000000000070997970c51812dc3a010c7d01b50e0d17dc79c80000000000000000000000000000000000000000000000000000000000000005"
	signedOrderHex = "000000000000000000000000000000000000000000000000000000000000004073646861617364617364000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000120000000000000000000000000000000000000000000000000000000000000000173646861000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000004000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb9226600000000000000000000000070997970c51812dc3a010c7d01b50e0d17dc79c80000000000000000000000000000000000000000000000000000000000000005"
	orderSigHex    = "7364686161736461736400000000000000000000000000000000000000000000"
	flagsHex       = "7364686100000000000000000000000000000000000000000000000000000000"
	makerHex       = "f39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	takerHex       = "70997970C51812dc3A010C7d01b50e0d17dc79C8"
)

func TestDecodeSignedMsg(t *testing.T) {
	signedMsgBytes, err := hex.DecodeString(signedOrderHex)
	require.NoError(t, err)
	var so SignedOrder
	err = so.DecodeFrom(signedMsgBytes)
	require.NoError(t, err)
	require.Equal(t, orderHex, hex.EncodeToString(so.Msg))
	require.Equal(t, orderSigHex, hex.EncodeToString(so.Sig[:]))
}

func TestDecodeOrder(t *testing.T) {
	flagsBytes, err := hex.DecodeString(flagsHex)
	require.NoError(t, err)
	makerBytes, err := hex.DecodeString(makerHex)
	require.NoError(t, err)
	takerBytes, err := hex.DecodeString(takerHex)
	require.NoError(t, err)
	odr := P1Order{
		CallType: 1,
		P1OrdersOrder: contracts.P1OrdersOrder{
			Amount:       big.NewInt(1),
			LimitPrice:   big.NewInt(2),
			TriggerPrice: big.NewInt(3),
			LimitFee:     big.NewInt(4),
			Expiration:   big.NewInt(5),
		},
	}
	copy(odr.Flags[:], flagsBytes)
	copy(odr.Maker[:], makerBytes)
	copy(odr.Taker[:], takerBytes)
	orderBytes, err := odr.Encode()
	require.NoError(t, err)
	require.Equal(t, orderHex, hex.EncodeToString(orderBytes))

	var odr2 P1Order
	err = odr2.DecodeFrom(orderBytes)
	require.NoError(t, err)
	require.Equal(t, odr, odr2)
}

func TestDecodeRLP(t *testing.T) {
	data := "f8ab819f843b9aca00832dc6c0945d64795f3f815924e607c7e9651e89db4dbddb6280b844a9059cbb00000000000000000000000033c866e121fa09a23a7dbecb87ad9c394d3d452300000000000000000000000000000000000000000000000000000002540be40081aaa08838daee659574adbea5efb9c36c3901a8d33275122403d10eec9c1bab461be5a0446b28b2014bf7b490e2297f19202fd6290b0e82657713df9661ee21b78a647e"
	txBytes, err := hex.DecodeString(data)
	require.NoError(t, err)
	var tx TxData
	err = rlp.DecodeBytes(txBytes, &tx)
	require.NoError(t, err)
	require.Equal(t, "0x5D64795f3f815924E607C7e9651e89Db4Dbddb62", tx.Recipient.String())
}

func TestDecodeParallel(t *testing.T) {
	var wg sync.WaitGroup
	f := func() {
		defer wg.Done()
		signedMsgBytes, err := hex.DecodeString(signedOrderHex)
		require.NoError(t, err)
		var so SignedOrder
		err = so.DecodeFrom(signedMsgBytes)
		require.NoError(t, err)
		require.Equal(t, orderHex, hex.EncodeToString(so.Msg))
		require.Equal(t, orderSigHex, hex.EncodeToString(so.Sig[:]))
	}

	total := 10000
	wg.Add(total)
	for i := 0; i < total; i++ {
		f()
	}
	wg.Wait()
}

func TestHash(t *testing.T) {
	hashHex := "0x8e67b6484476c8e2a168e37bed7be6212d5aaedc08869f8d6083fffdee2eb3ea"
	orderBytes, err := hex.DecodeString(orderHex)
	require.NoError(t, err)
	var odr P1Order
	err = odr.DecodeFrom(orderBytes)
	require.NoError(t, err)
	require.Equal(t, hashHex, odr.Hash().String())
	odr.CallType += 1
	require.Equal(t, hashHex, odr.Hash().String())
}

//TODO
func TestRealOrder(t *testing.T) {
	maker := common.FromHex("0xbbe4733d85bc2b90682147779da49cab38c0aa1f")
	odr := P1Order{
		CallType: 1,
		P1OrdersOrder: contracts.P1OrdersOrder{
			Amount:       big.NewInt(0).Mul(big.NewInt(6666), big.NewInt(1)),
			LimitPrice:   big.NewInt(0).Mul(big.NewInt(22), big.NewInt(1e18)),
			TriggerPrice: big.NewInt(0),
			LimitFee:     big.NewInt(0),
			Expiration:   big.NewInt(1668065275),
		},
	}
	fmt.Println(odr.Amount, odr.LimitPrice)
	copy(odr.Maker[:], maker)
	fmt.Println(odr.Hash())

}

func TestOrderHash(t *testing.T) {
	chainID := big.NewInt(65)
	orderContractAddr := "0x9D7f74d0C41E726EC95884E0e97Fa6129e3b5E99"

	odr := newP1Order()
	data, err := odr.encodeOrder()
	require.NoError(t, err)
	require.Equal(t, "0xa992c4f874c3f90d79d458707aecfec8c0698b47aaa2019f85a8ab462376adaf", crypto.Keccak256Hash(data).String())
	structHash := crypto.Keccak256Hash(EIP712_ORDER_STRUCT_SCHEMA_HASH[:], data)
	require.Equal(t, "0x906d8a993d6a3a8cb350ee8b21113ca34d9131ae513dc7ed605966427996580c", structHash.String())

	EIP712DomainHash := crypto.Keccak256Hash(EIP712_DOMAIN_SEPARATOR_SCHEMA_HASH[:], EIP712_DOMAIN_NAME_HASH[:], EIP712_DOMAIN_VERSION_HASH[:], common.LeftPadBytes(chainID.Bytes(), 32), common.LeftPadBytes(common.FromHex(orderContractAddr), 32))
	require.Equal(t, "0x1905e070f1c3100dda9bd4ea427b9c63d9b73b6b66af8ef935fd1ec9e3e66a91", EIP712DomainHash.String())

	orderHash := crypto.Keccak256Hash(EIP191_HEADER, EIP712DomainHash[:], structHash[:])
	require.Equal(t, "0xab28adc95c1e76ec402f1f62d9b7cc4596d40ce227a9d8ac04cc59b758bb2a89", orderHash.String())
}

func TestP1Order_VerifySignature(t *testing.T) {
	odr := newP1Order()
	sig, err := signOrder(odr, "8ff3ca2d9985c3a52b459e2f6e7822b23e1af845961e22128d5f372fb9aa5f17", 65, contractAddress)
	require.NoError(t, err)
	addr, err := ecrecover(odr.Hash(), sig)
	require.NoError(t, err)
	require.Equal(t, "0xbbE4733d85bc2b90682147779DA49caB38C0aA1F", addr.String())
}

func TestEcrecover(t *testing.T) {
	orderHash := common.BytesToHash(common.FromHex("0xb2e6dd6b159169d762132a47520fb10fdfe2e5f3acc5d8eda789d645a0ad243d"))
	sig := common.FromHex("0x14d533b96d159578ef239cc969818c00a16050e815e596555318e298c6536f8b3c253d22f5ae0d80033ed3c8c753e95192ca3e100a01fd4d75cd19a02d9e8f721b01")
	addr := common.HexToAddress("0xbbE4733d85bc2b90682147779DA49caB38C0aA1F")
	addr2, err := ecrecover(orderHash, sig)
	require.NoError(t, err)
	require.Equal(t, addr, addr2)
}

func TestSignature(t *testing.T) {
	orderHash := common.FromHex("0xb2e6dd6b159169d762132a47520fb10fdfe2e5f3acc5d8eda789d645a0ad243d")
	signedHash := crypto.Keccak256Hash([]byte(PREPEND_DEC), orderHash[:])
	sig := common.FromHex("0x14d533b96d159578ef239cc969818c00a16050e815e596555318e298c6536f8b3c253d22f5ae0d80033ed3c8c753e95192ca3e100a01fd4d75cd19a02d9e8f721b01")
	sig = sig[:65]
	sig[64] -= 27

	priv, err := crypto.HexToECDSA("8ff3ca2d9985c3a52b459e2f6e7822b23e1af845961e22128d5f372fb9aa5f17")
	addr := crypto.PubkeyToAddress(priv.PublicKey)
	require.Equal(t, "0xbbE4733d85bc2b90682147779DA49caB38C0aA1F", addr.String())

	data, err := crypto.Sign(signedHash[:], priv)
	require.NoError(t, err)
	require.Equal(t, sig, data)
}

func TestSignOrder(t *testing.T) {
	//TODO: test more order
	odr := newP1Order()
	sig, err := signOrder(odr, "8ff3ca2d9985c3a52b459e2f6e7822b23e1af845961e22128d5f372fb9aa5f17", 65, "0xf1730217Bd65f86D2F008f1821D8Ca9A26d64619")
	require.NoError(t, err)
	require.Equal(t, 66, len(sig))
	require.True(t, sig[65] == 1)
	require.True(t, sig[64] >= 27)
	require.True(t, sig[64] <= 28)
}

func signOrder(odr P1Order, hexPriv string, chainId int64, orderContractaddr string) ([]byte, error) {
	priv, err := crypto.HexToECDSA(hexPriv)
	if err != nil {
		return nil, err
	}
	orderHash := odr.Hash2(chainId, orderContractaddr)
	signedHash := crypto.Keccak256Hash([]byte(PREPEND_DEC), orderHash[:])
	sig, err := crypto.Sign(signedHash[:], priv)
	if err != nil {
		return nil, err
	}

	sig[len(sig)-1] += 27
	sig = append(sig, 1)
	return sig, nil
}

func newP1Order() P1Order {
	odr := P1Order{
		CallType: 1,
		P1OrdersOrder: contracts.P1OrdersOrder{
			Amount:       big.NewInt(1),
			LimitPrice:   big.NewInt(1),
			TriggerPrice: big.NewInt(1),
			LimitFee:     big.NewInt(1),
			Expiration:   big.NewInt(0),
		},
	}

	flags, _ := hex.DecodeString("4554480000000000000000000000000000000000000000000000000000000000")
	addr, _ := hex.DecodeString("4554480000000000000000000000000000000000")
	copy(odr.Flags[:], flags)
	copy(odr.Maker[:], addr)
	copy(odr.Taker[:], addr)
	return odr
}
