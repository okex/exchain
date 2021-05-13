package watcher

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/abci/types"
)

type Watcher struct {
	store         *WatchStore
	height        uint64
	blockHash     common.Hash
	header        types.Header
	batch         []WatchMessage
	cumulativeGas map[uint64]uint64
	gasUsed       uint64
	blockTxs      []common.Hash
	sw            bool
}

func IsWatcherEnabled() bool {
	return viper.GetBool(FlagFastQuery)
}

func NewWatcher() *Watcher {
	return &Watcher{store: InstanceOfWatchStore(), sw: IsWatcherEnabled()}
}

func (w *Watcher) Enabled() bool {
	return w.sw
}

func (w *Watcher) Enable(sw bool) {
	w.sw = sw
}

func (w *Watcher) NewHeight(height uint64, blockHash common.Hash, header types.Header) {
	if !w.Enabled() {
		return
	}
	w.batch = []WatchMessage{}
	w.header = header
	w.height = height
	w.blockHash = blockHash
	w.cumulativeGas = make(map[uint64]uint64)
	w.gasUsed = 0
	w.blockTxs = []common.Hash{}
}

func (w *Watcher) SaveEthereumTx(msg evmtypes.MsgEthereumTx, txHash common.Hash, index uint64) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgEthTx(&msg, txHash, w.blockHash, w.height, index)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
	w.UpdateBlockTxs(txHash)
}

func (w *Watcher) SaveContractCode(addr common.Address, code []byte) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgCode(addr, code, w.height)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveContractCodeByHash(hash []byte, code []byte) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgCodeByHash(hash, code, w.height)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveTransactionReceipt(status uint32, msg evmtypes.MsgEthereumTx, txHash common.Hash, txIndex uint64, data *evmtypes.ResultData, gasUsed uint64) {
	if !w.Enabled() {
		return
	}
	w.UpdateCumulativeGas(txIndex, gasUsed)
	wMsg := NewMsgTransactionReceipt(status, &msg, txHash, w.blockHash, txIndex, w.height, data, w.cumulativeGas[txIndex], gasUsed)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) UpdateCumulativeGas(txIndex, gasUsed uint64) {
	if !w.Enabled() {
		return
	}
	if len(w.cumulativeGas) == 0 {
		w.cumulativeGas[txIndex] = gasUsed
	} else {
		w.cumulativeGas[txIndex] = w.cumulativeGas[txIndex-1] + gasUsed
	}
	w.gasUsed += gasUsed
}

func (w *Watcher) UpdateBlockTxs(txHash common.Hash) {
	if !w.Enabled() {
		return
	}
	w.blockTxs = append(w.blockTxs, txHash)
}

func (w *Watcher) SaveAccount(account auth.Account) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgAccount(account)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) DeleteAccount(addr sdk.AccAddress) {
	if !w.Enabled() {
		return
	}
	w.store.Delete([]byte(GetMsgAccountKey(addr.String())))
}

func (w *Watcher) SaveState(addr common.Address, key, value []byte) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgState(addr, key, value)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveBlock(bloom ethtypes.Bloom) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgBlock(w.height, bloom, w.blockHash, w.header, uint64(0xffffffff), big.NewInt(int64(w.gasUsed)), w.blockTxs)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}

	wInfo := NewMsgBlockInfo(w.height, w.blockHash)
	if wInfo != nil {
		w.batch = append(w.batch, wInfo)
	}
	w.SaveLatestHeight(w.height)
}

func (w *Watcher) SaveLatestHeight(height uint64) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgLatestHeight(height)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveParams(params evmtypes.Params) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgParams(params)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveContractBlockedListItem(addr sdk.AccAddress) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgContractBlockedListItem(addr)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) SaveContractDeploymentWhitelistItem(addr sdk.AccAddress) {
	if !w.Enabled() {
		return
	}
	wMsg := NewMsgContractDeploymentWhitelistItem(addr)
	if wMsg != nil {
		w.batch = append(w.batch, wMsg)
	}
}

func (w *Watcher) HasContractBlockedList(addr sdk.AccAddress) bool {
	if !w.Enabled() {
		return false
	}
	return w.store.Has(evmtypes.GetContractBlockedListMemberKey(addr))
}

func (w *Watcher) HasContractDeploymentWhitelist(addr sdk.AccAddress) bool {
	if !w.Enabled() {
		return false
	}
	return w.store.Has(evmtypes.GetContractDeploymentWhitelistMemberKey(addr))
}

func (w *Watcher) DeleteContractBlockedList(addr sdk.AccAddress) {
	if !w.Enabled() {
		return
	}
	w.store.Delete(evmtypes.GetContractBlockedListMemberKey(addr))
}

func (w *Watcher) DeleteContractDeploymentWhitelist(addr sdk.AccAddress) {
	if !w.Enabled() {
		return
	}
	w.store.Delete(evmtypes.GetContractDeploymentWhitelistMemberKey(addr))
}

func (w *Watcher) Commit() {
	if !w.Enabled() {
		return
	}
	//hold it in temp
	batch := w.batch
	go func() {
		for _, b := range batch {
			w.store.Set([]byte(b.GetKey()), []byte(b.GetValue()))
		}
	}()
}
