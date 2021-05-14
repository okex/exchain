package watcher

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rpctypes "github.com/okex/exchain/app/rpc/types"
	"github.com/okex/exchain/app/types"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/status-im/keycard-go/hexutils"
)

const MsgFunctionDisable = "fast query function has been disabled"

type Querier struct {
	store *WatchStore
	sw    bool
}

func (q Querier) enabled() bool {
	return q.sw
}

func (q *Querier) Enable(sw bool) {
	q.sw = sw
}

func NewQuerier() *Querier {
	return &Querier{store: InstanceOfWatchStore(), sw: IsWatcherEnabled()}
}

func (q Querier) GetTransactionReceipt(hash common.Hash) (*TransactionReceipt, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var receipt TransactionReceipt
	b, e := q.store.Get([]byte(prefixReceipt + hash.String()))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &receipt)
	if e != nil {
		return nil, e
	}
	if receipt.Logs == nil {
		receipt.Logs = []*ethtypes.Log{}
	}
	return &receipt, nil
}

func (q Querier) GetBlockByHash(hash common.Hash, fullTx bool) (*EthBlock, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var block EthBlock
	b, e := q.store.Get([]byte(prefixBlock + hash.String()))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &block)
	if e != nil {
		return nil, e
	}
	if fullTx && block.Transactions != nil {
		txsHash := block.Transactions.([]interface{})
		txList := []rpctypes.Transaction{}
		for _, tx := range txsHash {
			transaction, e := q.GetTransactionByHash(common.HexToHash(tx.(string)))
			if e == nil && transaction != nil {
				txList = append(txList, *transaction)
			}
		}
		block.Transactions = txList
	}
	return &block, nil
}

func (q Querier) GetBlockByNumber(number uint64, fullTx bool) (*EthBlock, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var height = number
	var err error
	if height == 0 {
		height, err = q.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}
	}
	hash, e := q.store.Get([]byte(prefixBlockInfo + strconv.Itoa(int(height))))

	if e != nil {
		return nil, e
	}
	return q.GetBlockByHash(common.HexToHash(string(hash)), fullTx)
}

func (q Querier) GetCode(contractAddr common.Address, height uint64) ([]byte, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var codeInfo CodeInfo
	info, e := q.store.Get([]byte(prefixCode + contractAddr.String()))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(info, &codeInfo)
	if e != nil {
		return nil, e
	}
	if height < codeInfo.Height && height > 0 {
		return nil, errors.New("the target height has not deploy this contract yet")
	}
	return hex.DecodeString(codeInfo.Code)
}

func (q Querier) GetCodeByHash(codeHash []byte) ([]byte, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var codeInfo CodeInfo
	info, e := q.store.Get([]byte(prefixCodeHash + hexutils.BytesToHex(codeHash)))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(info, &codeInfo)
	if e != nil {
		return nil, e
	}
	return hex.DecodeString(codeInfo.Code)
}

func (q Querier) GetLatestBlockNumber() (uint64, error) {
	if !q.enabled() {
		return 0, errors.New(MsgFunctionDisable)
	}
	height, e := q.store.Get([]byte(prefixLatestHeight + KeyLatestHeight))
	if e != nil {
		return 0, e
	}
	h, e := strconv.Atoi(string(height))
	return uint64(h), e
}

func (q Querier) GetTransactionByHash(hash common.Hash) (*rpctypes.Transaction, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var tx rpctypes.Transaction
	transaction, e := q.store.Get([]byte(prefixTx + hash.String()))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(transaction, &tx)
	if e != nil {
		return nil, e
	}
	return &tx, nil
}

func (q Querier) GetTransactionByBlockNumberAndIndex(number uint64, idx uint) (*rpctypes.Transaction, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	block, e := q.GetBlockByNumber(number, true)
	if e != nil {
		return nil, e
	}
	return q.getTransactionByBlockAndIndex(block, idx)
}

func (q Querier) GetTransactionByBlockHashAndIndex(hash common.Hash, idx uint) (*rpctypes.Transaction, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	block, e := q.GetBlockByHash(hash, true)
	if e != nil {
		return nil, e
	}
	return q.getTransactionByBlockAndIndex(block, idx)
}

func (q Querier) getTransactionByBlockAndIndex(block *EthBlock, idx uint) (*rpctypes.Transaction, error) {
	if block.Transactions == nil {
		return nil, errors.New("no such transaction in target block")
	}
	txs := block.Transactions.([]rpctypes.Transaction)

	for _, tx := range txs {
		rawTx := tx
		if idx == uint(*rawTx.TransactionIndex) {
			return &rawTx, nil
		}
	}
	return nil, errors.New("no such transaction in target block")
}

func (q Querier) GetAccount(addr sdk.AccAddress) (*types.EthAccount, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	var acc types.EthAccount
	b, e := q.store.Get([]byte(GetMsgAccountKey(addr.String())))
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &acc)
	if e != nil {
		return nil, e
	}
	return &acc, nil
}

func (q Querier) GetState(addr common.Address, key []byte) ([]byte, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	b, e := q.store.Get([]byte(GetMsgStateKey(addr, key)))
	if e != nil {
		return nil, e
	}
	ret := hexutils.HexToBytes(string(b))
	return ret, nil
}

func (q Querier) GetParams() (*evmtypes.Params, error) {
	if !q.enabled() {
		return nil, errors.New(MsgFunctionDisable)
	}
	b, e := q.store.Get([]byte(prefixParams))
	if e != nil {
		return nil, e
	}
	var params evmtypes.Params
	e = json.Unmarshal(b, &params)
	if e != nil {
		return nil, e
	}
	return &params, nil
}

func (q Querier) HasContractBlockedList(key []byte) bool {
	if !q.enabled() {
		return false
	}
	return q.store.Has([]byte(prefixBlackList + hexutils.BytesToHex(key)))
}

func (q Querier) HasContractDeploymentWhitelist(key []byte) bool {
	if !q.enabled() {
		return false
	}
	return q.store.Has([]byte(prefixWhiteList + hexutils.BytesToHex(key)))
}