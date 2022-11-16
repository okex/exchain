package watcher

import (
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/okex/exchain/app/crypto/ethsecp256k1"
	ethermint "github.com/okex/exchain/app/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/crypto/secp256k1"
	"github.com/okex/exchain/libs/tendermint/libs/rand"
	"github.com/okex/exchain/x/evm/types"
	prototypes "github.com/okex/exchain/x/evm/watcher/proto"
	"github.com/stretchr/testify/require"
	"math/big"
	"strconv"
	"testing"
)

var (
	wsgHash = common.BytesToHash([]byte("0x01"))
	// batch set data hash
	batchHash = common.BytesToHash([]byte("0x02"))
	delHash1  = common.BytesToHash([]byte("0x03"))
	// del set nil
	delHash2 = common.BytesToHash([]byte("0x04"))
)

type data struct {
	wsg   WatchMessage
	batch *Batch
	del1  *Batch
	del2  []byte
}

func newTestTransactionReceipt(txHash common.Hash) TransactionReceipt {
	addr := common.BytesToAddress([]byte("test_address"))
	ethTx := types.NewMsgEthereumTx(0, &addr, nil, 100000, nil, []byte("test"))
	return newTransactionReceipt(
		0,
		ethTx,
		txHash,
		common.Hash{0x02},
		0,
		0,
		&types.ResultData{},
		1,
		1)
}

func TestGetTransactionReceipt(t *testing.T) {
	acq := newACProcessorQuerier(nil)
	acProcessor := acq.p

	testcases := []struct {
		d       *data
		fnInit  func(d *data)
		fnCheck func(d *data)
	}{
		{
			d: &data{},
			fnInit: func(d *data) {
				d.wsg = NewMsgTransactionReceipt(newTestTransactionReceipt(wsgHash), wsgHash)

				btx := NewMsgTransactionReceipt(newTestTransactionReceipt(batchHash), batchHash)
				d.batch = &Batch{
					Key:       btx.GetKey(),
					Value:     []byte(btx.GetValue()),
					TypeValue: btx.GetType(),
				}

				dtx1 := NewMsgTransactionReceipt(newTestTransactionReceipt(delHash1), delHash1)
				d.del1 = &Batch{
					Key:       dtx1.GetKey(),
					TypeValue: TypeDelete,
				}

				dtx2 := NewMsgTransactionReceipt(newTestTransactionReceipt(delHash2), delHash2)
				d.del2 = dtx2.GetKey()

				acProcessor.BatchSet([]WatchMessage{d.wsg})
				acProcessor.BatchSetEx([]*Batch{d.batch, d.del1})
				acProcessor.BatchDel([][]byte{d.del2})
			},
			fnCheck: func(d *data) {
				recp, err := acq.GetTransactionReceipt(d.wsg.GetKey())
				require.Nil(t, err)

				var protoReceipt prototypes.TransactionReceipt
				e := proto.Unmarshal([]byte(d.wsg.GetValue()), &protoReceipt)
				require.NoError(t, e)
				receipt := protoToReceipt(&protoReceipt)
				require.Equal(t, recp, receipt)

				recp, err = acq.GetTransactionReceipt(d.batch.GetKey())
				require.Nil(t, err)

				recp, err = acq.GetTransactionReceipt(d.del1.GetKey())
				require.Nil(t, err)
				require.Nil(t, recp)

				recp, err = acq.GetTransactionReceipt(d.del2)
				require.Nil(t, err)
				require.Nil(t, recp)
			},
		},
	}

	for _, ts := range testcases {
		ts.fnInit(ts.d)
		for i := 0; i < 3; i++ {
			ts.fnCheck(ts.d)
		}
	}
}

func newTestBlock(hash common.Hash) Block {
	return newBlock(
		0,
		ethtypes.Bloom{},
		hash,
		abci.Header{},
		0,
		big.NewInt(1),
		nil,
	)
}

func TestGetBlockByHash(t *testing.T) {
	acq := newACProcessorQuerier(nil)
	acProcessor := acq.p

	testcases := []struct {
		d       *data
		fnInit  func(d *data)
		fnCheck func(d *data)
	}{
		{
			d: &data{},
			fnInit: func(d *data) {
				d.wsg = NewMsgBlock(newTestBlock(wsgHash))
				btx := NewMsgBlock(newTestBlock(batchHash))
				d.batch = &Batch{
					Key:       btx.GetKey(),
					Value:     []byte(btx.GetValue()),
					TypeValue: btx.GetType(),
				}

				dtx1 := NewMsgBlock(newTestBlock(delHash1))
				d.del1 = &Batch{
					Key:       dtx1.GetKey(),
					TypeValue: TypeDelete,
				}

				dtx2 := NewMsgBlock(newTestBlock(delHash2))
				d.del2 = dtx2.GetKey()

				acProcessor.BatchSet([]WatchMessage{d.wsg})
				acProcessor.BatchSetEx([]*Batch{d.batch, d.del1})
				acProcessor.BatchDel([][]byte{d.del2})
			},
			fnCheck: func(d *data) {
				recp, err := acq.GetBlockByHash(d.wsg.GetKey())
				require.Nil(t, err)
				require.Equal(t, d.wsg.(*MsgBlock).blockHash, recp.Hash[:])

				recp, err = acq.GetBlockByHash(d.batch.GetKey())
				require.Nil(t, err)

				recp, err = acq.GetBlockByHash(d.del1.GetKey())
				require.Nil(t, err)
				require.Nil(t, recp)

				recp, err = acq.GetBlockByHash(d.del2)
				require.Nil(t, err)
				require.Nil(t, recp)
			},
		},
	}

	for _, ts := range testcases {
		ts.fnInit(ts.d)
		for i := 0; i < 3; i++ {
			ts.fnCheck(ts.d)
		}
	}
}

func newTestMsgEthTx(txHash common.Hash) *MsgEthTx {
	addr := common.BytesToAddress([]byte("test_address"))
	ethTx := types.NewMsgEthereumTx(0, &addr, big.NewInt(1), 100000, big.NewInt(1), []byte("test"))
	privkey, err := ethsecp256k1.GenerateKey()
	if err != nil {
		panic(err)
	}
	ethTx.Sign(big.NewInt(1), privkey.ToECDSA())
	if err != nil {
		panic(err)
	}
	return newMsgEthTx(
		ethTx,
		txHash,
		common.Hash{0x02},
		0,
		0)
}

func TestGetTransactionByHash(t *testing.T) {
	acq := newACProcessorQuerier(nil)
	acProcessor := acq.p

	testcases := []struct {
		d       *data
		fnInit  func(d *data)
		fnCheck func(d *data)
	}{
		{
			d: &data{},
			fnInit: func(d *data) {
				d.wsg = newTestMsgEthTx(wsgHash)

				btx := newTestMsgEthTx(batchHash)
				d.batch = &Batch{
					Key:       btx.GetKey(),
					Value:     []byte(btx.GetValue()),
					TypeValue: btx.GetType(),
				}

				dtx1 := newTestMsgEthTx(delHash1)
				d.del1 = &Batch{
					Key:       dtx1.GetKey(),
					TypeValue: TypeDelete,
				}

				dtx2 := newTestMsgEthTx(delHash2)
				d.del2 = dtx2.GetKey()

				acProcessor.BatchSet([]WatchMessage{d.wsg})
				acProcessor.BatchSetEx([]*Batch{d.batch, d.del1})
				acProcessor.BatchDel([][]byte{d.del2})
			},
			fnCheck: func(d *data) {
				recp, err := acq.GetTransactionByHash(d.wsg.GetKey())
				require.Nil(t, err)

				var prototx prototypes.Transaction
				e := proto.Unmarshal([]byte(d.wsg.GetValue()), &prototx)
				require.NoError(t, e)
				tx := protoToTransaction(&prototx)
				require.Equal(t, recp, tx)

				recp, err = acq.GetTransactionByHash(d.batch.GetKey())
				require.Nil(t, err)

				recp, err = acq.GetTransactionByHash(d.del1.GetKey())
				require.Nil(t, err)
				require.Nil(t, recp)

				recp, err = acq.GetTransactionByHash(d.del2)
				require.Nil(t, err)
				require.Nil(t, recp)
			},
		},
	}

	for _, ts := range testcases {
		ts.fnInit(ts.d)
		for i := 0; i < 3; i++ {
			ts.fnCheck(ts.d)
		}
	}
}

func TestGetLatestBlockNumber(t *testing.T) {
	testcases := []struct {
		fnCheckMsg   func()
		fnCheckBatch func()
		fnCheckDel1  func()
		fnCheckDel2  func()
	}{
		{
			fnCheckMsg: func() {
				acq := newACProcessorQuerier(nil)
				wsg := NewMsgLatestHeight(1)
				acq.p.BatchSet([]WatchMessage{wsg})

				r, err := acq.GetLatestBlockNumber(wsg.GetKey())
				require.Nil(t, err)
				require.Equal(t, wsg.height, strconv.Itoa(int(r)))
			},
			fnCheckBatch: func() {
				acq := newACProcessorQuerier(nil)
				btx := NewMsgLatestHeight(2)
				acq.p.BatchSetEx([]*Batch{
					{
						Key:       btx.GetKey(),
						Value:     []byte(btx.GetValue()),
						TypeValue: btx.GetType(),
					},
				})
				r, err := acq.GetLatestBlockNumber(btx.GetKey())
				require.Nil(t, err)
				require.Equal(t, btx.height, strconv.Itoa(int(r)))
			},
			fnCheckDel1: func() {
				acq := newACProcessorQuerier(nil)
				del1 := NewMsgLatestHeight(2)
				acq.p.BatchSetEx([]*Batch{{Key: del1.GetKey(), TypeValue: TypeDelete}})
				r, err := acq.GetLatestBlockNumber(del1.GetKey())
				require.Nil(t, err)
				require.Equal(t, 0, int(r))
			},
			fnCheckDel2: func() {
				acq := newACProcessorQuerier(nil)
				del2 := NewMsgLatestHeight(3)
				acq.p.BatchDel([][]byte{del2.GetKey()})
				r, err := acq.GetLatestBlockNumber(del2.GetKey())
				require.Nil(t, err)
				require.Equal(t, 0, int(r))
			},
		},
	}

	for _, ts := range testcases {
		ts.fnCheckMsg()
		ts.fnCheckBatch()
		ts.fnCheckDel1()
		ts.fnCheckDel2()
	}
}

func newTestAccount() *MsgAccount {
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	addr := sdk.AccAddress(pubKey.Address())
	balance := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(1)))
	a1 := &ethermint.EthAccount{
		BaseAccount: auth.NewBaseAccount(addr, balance, pubKey, 1, 1),
		CodeHash:    ethcrypto.Keccak256(nil),
	}
	return NewMsgAccount(a1)
}

func TestGetAccount(t *testing.T) {
	acq := newACProcessorQuerier(nil)
	acProcessor := acq.p

	testcases := []struct {
		d       *data
		fnInit  func(d *data)
		fnCheck func(d *data)
	}{
		{
			d: &data{},
			fnInit: func(d *data) {
				d.wsg = newTestAccount()
				btx := newTestAccount()
				d.batch = &Batch{
					Key:       btx.GetKey(),
					Value:     []byte(btx.GetValue()),
					TypeValue: btx.GetType(),
				}

				dtx1 := newTestAccount()
				d.del1 = &Batch{
					Key:       dtx1.GetKey(),
					TypeValue: TypeDelete,
				}

				dtx2 := newTestAccount()
				d.del2 = dtx2.GetKey()

				acProcessor.BatchSet([]WatchMessage{d.wsg})
				acProcessor.BatchSetEx([]*Batch{d.batch, d.del1})
				acProcessor.BatchDel([][]byte{d.del2})
			},
			fnCheck: func(d *data) {
				recp, err := acq.GetAccount(d.wsg.GetKey())
				require.Nil(t, err)
				require.Equal(t, d.wsg.(*MsgAccount).account.Address, recp.Address)

				recp, err = acq.GetAccount(d.batch.GetKey())
				require.Nil(t, err)

				recp, err = acq.GetAccount(d.del1.GetKey())
				require.Nil(t, err)
				require.Nil(t, recp)

				recp, err = acq.GetAccount(d.del2)
				require.Nil(t, err)
				require.Nil(t, recp)
			},
		},
	}

	for _, ts := range testcases {
		ts.fnInit(ts.d)
		for i := 0; i < 3; i++ {
			ts.fnCheck(ts.d)
		}
	}
}

func newTestState() *MsgState {
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	addr := pubKey.Address()
	key := rand.Str(32)
	value := rand.Str(32)
	return NewMsgState(common.BytesToAddress(addr), []byte(key), []byte(value))
}

func TestGetState(t *testing.T) {
	acq := newACProcessorQuerier(nil)
	acProcessor := acq.p

	testcases := []struct {
		d       *data
		fnInit  func(d *data)
		fnCheck func(d *data)
	}{
		{
			d: &data{},
			fnInit: func(d *data) {
				d.wsg = newTestState()
				btx := newTestState()
				d.batch = &Batch{
					Key:       btx.GetKey(),
					Value:     []byte(btx.GetValue()),
					TypeValue: btx.GetType(),
				}

				dtx1 := newTestState()
				d.del1 = &Batch{
					Key:       dtx1.GetKey(),
					TypeValue: TypeDelete,
				}

				dtx2 := newTestState()
				d.del2 = dtx2.GetKey()

				acProcessor.BatchSet([]WatchMessage{d.wsg})
				acProcessor.BatchSetEx([]*Batch{d.batch, d.del1})
				acProcessor.BatchDel([][]byte{d.del2})
			},
			fnCheck: func(d *data) {
				recp, err := acq.GetState(d.wsg.GetKey())
				require.Nil(t, err)
				require.Equal(t, d.wsg.(*MsgState).GetValue(), string(recp))

				recp, err = acq.GetState(d.batch.GetKey())
				require.Nil(t, err)
				require.Equal(t, d.batch.GetValue(), string(recp))

				recp, err = acq.GetState(d.del1.GetKey())
				require.Nil(t, err)
				require.Nil(t, recp)

				recp, err = acq.GetState(d.del2)
				require.Nil(t, err)
				require.Nil(t, recp)
			},
		},
	}

	for _, ts := range testcases {
		ts.fnInit(ts.d)
		for i := 0; i < 3; i++ {
			ts.fnCheck(ts.d)
		}
	}
}