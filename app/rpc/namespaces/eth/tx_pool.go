package eth

import (
	"fmt"
	clientcontext "github.com/cosmos/cosmos-sdk/client/context"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ethereum/go-ethereum/common"
	rpctypes "github.com/okex/exchain/app/rpc/types"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"sync"
)

const (
	FlagEnableTxPool = "enable-tx-pool"
	TxPoolSliceMaxLen = 2000
)

type TxPool struct {
	addressTxsPool map[common.Address][]*evmtypes.MsgEthereumTx // All currently processable transactions
	mu             sync.Mutex
}

func NewTxPool() *TxPool {
	pool := &TxPool{
		addressTxsPool: make(map[common.Address][]*evmtypes.MsgEthereumTx),
	}

	return pool
}

func (pool *TxPool) CacheAndBroadcastTx(api *PublicEthereumAPI, address common.Address, tx *evmtypes.MsgEthereumTx) error {
	// map need lock
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// get currentNonce
	pCurrentNonce, err := api.GetTransactionCount(address, rpctypes.PendingBlockNumber)
	if err != nil {
		return err
	}
	currentNonce := uint64(*pCurrentNonce)

	if tx.Data.AccountNonce < currentNonce {
		return fmt.Errorf("AccountNonce of tx is less than currentNonce in memPool: AccountNonce[%d], currentNonce[%d]", tx.Data.AccountNonce, currentNonce)
	}

	if err = pool.insertTx(address, tx); err != nil {
		return err
	}

	if err = pool.continueBroadcast(api.clientCtx, currentNonce, address); err != nil {
		return err
	}

	return nil
}

func (pool *TxPool) update(index int, address common.Address, tx *evmtypes.MsgEthereumTx) error {
	txsLen := len(pool.addressTxsPool[address])
	var err error
	if index >= txsLen {
		if txsLen > TxPoolSliceMaxLen {
			return fmt.Errorf("TxPool is full, the last tx:nonce[%d] has been dropped", tx.Data.AccountNonce)
		}
		pool.addressTxsPool[address] = append(pool.addressTxsPool[address], tx)
	} else {
		endIndex := txsLen
		if txsLen > TxPoolSliceMaxLen {
			endIndex = txsLen - 1
			err = fmt.Errorf("TxPool is full, the last tx:nonce[%d] has been dropped", pool.addressTxsPool[address][endIndex].Data.AccountNonce)
		}
		tmpTx := make([]*evmtypes.MsgEthereumTx, len(pool.addressTxsPool[address][index:endIndex]))
		copy(tmpTx, pool.addressTxsPool[address][index:endIndex])

		pool.addressTxsPool[address] =
			append(append(pool.addressTxsPool[address][:index], tx), tmpTx...)
	}

	return err
}

// insert the tx into the txPool
func (pool *TxPool) insertTx(address common.Address, tx *evmtypes.MsgEthereumTx) error {
	index := 0
	for index < len(pool.addressTxsPool[address]) {
		// the tx nonce has in txPool, drop duplicate tx
		if tx.Data.AccountNonce == pool.addressTxsPool[address][index].Data.AccountNonce {
			return fmt.Errorf("duplicate tx, this AccountNonce of tx has been send. AccountNonce[%d]", tx.Data.AccountNonce)
		}
		// find the index to insert
		if tx.Data.AccountNonce < pool.addressTxsPool[address][index].Data.AccountNonce {
			break
		}
		index++
	}

	// update txPool
	return pool.update(index, address, tx)
}

// iterate through the txPool map, check if need to continue broadcast tx and do it
func (pool *TxPool) continueBroadcast(clientCtx clientcontext.CLIContext, currentNonce uint64, address common.Address) error {
	i := 0
	for i < len(pool.addressTxsPool[address]) {
		if pool.addressTxsPool[address][i].Data.AccountNonce != currentNonce {
			break
		}
		// do broadcast
		if err := pool.broadcast(clientCtx, pool.addressTxsPool[address][i]); err != nil {
			return err
		}
		// update currentNonce
		currentNonce++
		i++
	}

	// update txPool
	if i != 0 {
		pool.addressTxsPool[address] = pool.addressTxsPool[address][i:]
	}

	return nil
}

func (pool *TxPool) broadcast(clientCtx clientcontext.CLIContext, tx *evmtypes.MsgEthereumTx) error {
	txEncoder := authclient.GetTxEncoder(clientCtx.Codec)
	txBytes, err := txEncoder(tx)
	if err != nil {
		return err
	}
	_, err = clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return err
	}
	return nil
}
