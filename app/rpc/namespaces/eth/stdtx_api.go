package eth

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ctypes "github.com/okex/exchain/libs/tendermint/rpc/core/types"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/spf13/viper"

	"github.com/okex/exchain/app/rpc/monitor"
	rpctypes "github.com/okex/exchain/app/rpc/types"
	"github.com/okex/exchain/x/evm/watcher"
)

func (api *PublicEthereumAPI) getTransactionWithStdByBlockAndIndex(block *tmtypes.Block, idx hexutil.Uint) (*watcher.Transaction, error) {
	// return if index out of bounds
	if uint64(idx) >= uint64(len(block.Txs)) {
		return nil, nil
	}

	rpcTx, err := rpctypes.RawTxToWatcherTx(api.clientCtx, block.Txs[idx], common.BytesToHash(block.Hash()), uint64(block.Height), uint64(idx))
	if err != nil {
		return nil, err
	}

	return rpcTx, nil
}

// GetTransactionsByBlock returns some transactions identified by number or hash.
func (api *PublicEthereumAPI) GetTransactionsWithStdByBlock(blockNrOrHash rpctypes.BlockNumberOrHash,
	offset, limit hexutil.Uint) ([]*watcher.Transaction, error) {
	if !viper.GetBool(FlagEnableMultiCall) {
		return nil, errors.New("the method is not allowed")
	}

	monitor := monitor.GetMonitor("eth_getTransactionsWithStdByBlock", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("block number", blockNrOrHash, "offset", offset, "limit", limit)

	blockNum, err := api.backend.ConvertToBlockNumber(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	txs, e := api.wrappedBackend.GetTransactionsWithStdByBlockNumber(uint64(blockNum), uint64(offset), uint64(limit))
	if e == nil && txs != nil {
		return txs, nil
	}

	height := blockNum.Int64()
	switch blockNum {
	case rpctypes.PendingBlockNumber:
		// get all the EVM pending txs
		pendingTxs, err := api.backend.PendingTransactionsWithStd()
		if err != nil {
			return nil, err
		}
		switch {
		case len(pendingTxs) <= int(offset):
			return nil, nil
		case len(pendingTxs) < int(offset+limit):
			return pendingTxs[offset:], nil
		default:
			return pendingTxs[offset : offset+limit], nil
		}
	case rpctypes.LatestBlockNumber:
		height, err = api.backend.LatestBlockNumber()
		if err != nil {
			return nil, err
		}
	}

	resBlock, err := api.clientCtx.Client.Block(&height)
	if err != nil {
		return nil, err
	}
	for idx := offset; idx < offset+limit && int(idx) < len(resBlock.Block.Txs); idx++ {
		tx, _ := api.getTransactionWithStdByBlockAndIndex(resBlock.Block, idx)
		if tx != nil {
			txs = append(txs, tx)
		}
	}
	return txs, nil
}

// GetTransactionReceiptsByBlock returns the transaction receipt identified by block hash or number.
func (api *PublicEthereumAPI) GetAllTransactionResultsByBlock(blockNrOrHash rpctypes.BlockNumberOrHash, offset, limit hexutil.Uint) ([]*watcher.TransactionResult, error) {
	if !viper.GetBool(FlagEnableMultiCall) {
		return nil, errors.New("the method is not allowed")
	}

	monitor := monitor.GetMonitor("eth_getTransactionReceiptsByBlock", api.logger, api.Metrics).OnBegin()
	defer monitor.OnEnd("block number", blockNrOrHash, "offset", offset, "limit", limit)

	txs, err := api.GetTransactionsWithStdByBlock(blockNrOrHash, offset, limit)
	if err != nil || len(txs) == 0 {
		return nil, err
	}

	var results []*watcher.TransactionResult
	var block *ctypes.ResultBlock
	var blockHash common.Hash

	for _, tx := range txs {
		var res *watcher.TransactionResult
		var isEthTx bool
		// std tx
		if tx.R == nil && tx.S == nil && tx.V == nil {
			isEthTx = false
			stdResponse, _ := api.wrappedBackend.GetTransactionResponse(tx.Hash)
			if stdResponse != nil {
				res, err = rpctypes.RawTxResultToStdResponse(api.clientCtx, stdResponse.ResultTx, stdResponse.Timestamp)
				if err != nil {
					return nil, err
				}
			}
		} else {
			isEthTx = true
			receipt, _ := api.wrappedBackend.GetTransactionReceipt(tx.Hash)
			if receipt != nil {
				res = &watcher.TransactionResult{TxType: hexutil.Uint64(watcher.EthReceipt), EthTx: tx, Receipt: receipt}
			}
		}

		if res != nil {
			results = append(results, res)
			continue
		}

		queryTx, err := api.clientCtx.Client.Tx(tx.Hash.Bytes(), false)
		if err != nil {
			// Return nil for transaction when not found
			return nil, nil
		}

		if block == nil {
			// Query block for consensus hash
			block, err = api.clientCtx.Client.Block(&queryTx.Height)
			if err != nil {
				return nil, err
			}
			blockHash = common.BytesToHash(block.Block.Hash())
		}

		if isEthTx {
			res, err = rpctypes.RawTxResultToEthReceipt(api.clientCtx, queryTx, blockHash)
		} else {
			res, err = rpctypes.RawTxResultToStdResponse(api.clientCtx, queryTx, block.Block.Time)
		}

		if err != nil {
			return nil, err
		}

		if res != nil {
			results = append(results, res)
		}

	}

	return results, nil
}
