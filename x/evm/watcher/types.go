package watcher

import (
	"encoding/binary"
	"encoding/json"
	sdk "github.com/cosmos/cosmos-sdk/types"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rpctypes "github.com/okex/exchain/app/rpc/types"
	"github.com/okex/exchain/x/evm/types"
	"github.com/status-im/keycard-go/hexutils"
	abci "github.com/tendermint/tendermint/abci/types"
)

const (
	prefixTx           = "0x1"
	prefixBlock        = "0x2"
	prefixReceipt      = "0x3"
	prefixCode         = "0x4"
	prefixBlockInfo    = "0x5"
	prefixLatestHeight = "0x6"
	prefixAccount      = "0x7"
	prefixState        = "0x8"
	prefixCodeHash     = "0x9"
	prefixParams       = "0x10"
	prefixWhiteList    = "0x11"
	prefixBlackList    = "0x12"

	KeyLatestHeight = "LatestHeight"

	TransactionSuccess = 1
	TransactionFailed  = 0
)

type WatchMessage interface {
	GetKey() string
	GetValue() string
}

type MsgEthTx struct {
	Key       string
	JsonEthTx string
}

func NewMsgEthTx(tx *types.MsgEthereumTx, txHash, blockHash common.Hash, height, index uint64) *MsgEthTx {
	ethTx, e := rpctypes.NewTransaction(tx, txHash, blockHash, height, index)
	if e != nil {
		return nil
	}
	jsTx, e := json.Marshal(ethTx)
	if e != nil {
		return nil
	}
	msg := MsgEthTx{
		Key:       txHash.String(),
		JsonEthTx: string(jsTx),
	}
	return &msg
}

func (m MsgEthTx) GetKey() string {
	return prefixTx + m.Key
}

func (m MsgEthTx) GetValue() string {
	return m.JsonEthTx
}

type MsgCode struct {
	Key  string
	Code string
}

type CodeInfo struct {
	Height uint64 `json:"height"`
	Code   string `json:"code"`
}

func NewMsgCode(contractAddr common.Address, code []byte, height uint64) *MsgCode {
	codeInfo := CodeInfo{
		Height: height,
		Code:   hexutils.BytesToHex(code),
	}
	jsCode, e := json.Marshal(codeInfo)
	if e != nil {
		return nil
	}
	return &MsgCode{
		Key:  contractAddr.String(),
		Code: string(jsCode),
	}
}

func (m MsgCode) GetKey() string {
	return prefixCode + m.Key
}

func (m MsgCode) GetValue() string {
	return m.Code
}

type MsgCodeByHash struct {
	Key  string
	Code string
}

func NewMsgCodeByHash(hash []byte, code []byte, height uint64) *MsgCodeByHash {
	codeInfo := CodeInfo{
		Height: height,
		Code:   hexutils.BytesToHex(code),
	}
	jsCode, e := json.Marshal(codeInfo)
	if e != nil {
		return nil
	}
	return &MsgCodeByHash{
		Key:  hexutils.BytesToHex(hash),
		Code: string(jsCode),
	}
}

func (m MsgCodeByHash) GetKey() string {
	return prefixCodeHash + m.Key
}

func (m MsgCodeByHash) GetValue() string {
	return m.Code
}

type MsgTransactionReceipt struct {
	txHash  string
	receipt string
}

type TransactionReceipt struct {
	Status            hexutil.Uint64  `json:"status"`
	CumulativeGasUsed hexutil.Uint64  `json:"cumulativeGasUsed"`
	LogsBloom         ethtypes.Bloom  `json:"logsBloom"`
	Logs              []*ethtypes.Log `json:"logs"`
	TransactionHash   string          `json:"transactionHash"`
	ContractAddress   *common.Address `json:"contractAddress"`
	GasUsed           hexutil.Uint64  `json:"gasUsed"`
	BlockHash         string          `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	From              string          `json:"from"`
	To                *common.Address `json:"to"`
}

func NewMsgTransactionReceipt(status uint32, tx *types.MsgEthereumTx, txHash, blockHash common.Hash, txIndex, height uint64, data *types.ResultData, cumulativeGas, GasUsed uint64) *MsgTransactionReceipt {

	tr := TransactionReceipt{
		Status:            hexutil.Uint64(status),
		CumulativeGasUsed: hexutil.Uint64(cumulativeGas),
		LogsBloom:         data.Bloom,
		Logs:              data.Logs,
		TransactionHash:   txHash.String(),
		ContractAddress:   &data.ContractAddress,
		GasUsed:           hexutil.Uint64(GasUsed),
		BlockHash:         blockHash.String(),
		BlockNumber:       hexutil.Uint64(height),
		TransactionIndex:  hexutil.Uint64(txIndex),
		From:              common.BytesToAddress(tx.From().Bytes()).Hex(),
		To:                tx.To(),
	}

	//contract address will be set to 0x0000000000000000000000000000000000000000 if contract deploy failed
	if tr.ContractAddress != nil && tr.ContractAddress.String() == "0x0000000000000000000000000000000000000000" {
		//set to nil to keep sync with ethereum rpc
		tr.ContractAddress = nil
	}
	jsTr, e := json.Marshal(tr)
	if e != nil {
		return nil
	}
	return &MsgTransactionReceipt{txHash: txHash.String(), receipt: string(jsTr)}
}

func (m MsgTransactionReceipt) GetKey() string {
	return prefixReceipt + m.txHash
}

func (m MsgTransactionReceipt) GetValue() string {
	return m.receipt
}

type MsgBlock struct {
	blockHash string
	block     string
}

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

type EthBlock struct {
	Number           hexutil.Uint64 `json:"number"`
	Hash             common.Hash    `json:"hash"`
	ParentHash       common.Hash    `json:"parentHash"`
	Nonce            BlockNonce     `json:"nonce"`
	Sha3Uncles       common.Hash    `json:"sha3Uncles"`
	LogsBloom        ethtypes.Bloom `json:"logsBloom"`
	TransactionsRoot common.Hash    `json:"transactionsRoot"`
	StateRoot        common.Hash    `json:"stateRoot"`
	Miner            common.Address `json:"miner"`
	MixHash          common.Hash    `json:"mixHash"`
	Difficulty       hexutil.Uint64 `json:"difficulty"`
	TotalDifficulty  hexutil.Uint64 `json:"totalDifficulty"`
	ExtraData        hexutil.Bytes  `json:"extraData"`
	Size             hexutil.Uint64 `json:"size"`
	GasLimit         hexutil.Uint64 `json:"gasLimit"`
	GasUsed          *hexutil.Big   `json:"gasUsed"`
	Timestamp        hexutil.Uint64 `json:"timestamp"`
	Uncles           []string       `json:"uncles"`
	ReceiptsRoot     common.Hash    `json:"receiptsRoot"`
	Transactions     interface{}    `json:"transactions"`
}

func NewMsgBlock(height uint64, blockBloom ethtypes.Bloom, blockHash common.Hash, header abci.Header, gasLimit uint64, gasUsed *big.Int, txs interface{}) *MsgBlock {
	b := EthBlock{
		Number:           hexutil.Uint64(height),
		Hash:             blockHash,
		ParentHash:       common.BytesToHash(header.LastBlockId.Hash),
		Nonce:            BlockNonce{},
		Sha3Uncles:       common.Hash{},
		LogsBloom:        blockBloom,
		TransactionsRoot: common.BytesToHash(header.DataHash),
		StateRoot:        common.BytesToHash(header.AppHash),
		Miner:            common.BytesToAddress(header.ProposerAddress),
		MixHash:          common.Hash{},
		Difficulty:       0,
		TotalDifficulty:  0,
		ExtraData:        nil,
		Size:             hexutil.Uint64(header.Size()),
		GasLimit:         hexutil.Uint64(gasLimit),
		GasUsed:          (*hexutil.Big)(gasUsed),
		Timestamp:        hexutil.Uint64(header.Time.Unix()),
		Uncles:           []string{},
		ReceiptsRoot:     common.Hash{},
		Transactions:     txs,
	}
	jsBlock, e := json.Marshal(b)
	if e != nil {
		return nil
	}
	return &MsgBlock{blockHash: blockHash.String(), block: string(jsBlock)}
}

func (m MsgBlock) GetKey() string {
	return prefixBlock + m.blockHash
}

func (m MsgBlock) GetValue() string {
	return m.block
}

type MsgBlockInfo struct {
	height string
	hash   string
}

func NewMsgBlockInfo(height uint64, blockHash common.Hash) *MsgBlockInfo {
	return &MsgBlockInfo{
		height: strconv.Itoa(int(height)),
		hash:   blockHash.String(),
	}
}

func (b MsgBlockInfo) GetKey() string {
	return prefixBlockInfo + b.height
}

func (b MsgBlockInfo) GetValue() string {
	return b.hash
}

type MsgLatestHeight struct {
	height string
}

func NewMsgLatestHeight(height uint64) *MsgLatestHeight {
	return &MsgLatestHeight{
		height: strconv.Itoa(int(height)),
	}
}

func (b MsgLatestHeight) GetKey() string {
	return prefixLatestHeight + KeyLatestHeight
}

func (b MsgLatestHeight) GetValue() string {
	return b.height
}

type MsgAccount struct {
	addr         string
	accountValue string
}

func NewMsgAccount(acc auth.Account) *MsgAccount {
	jsonAcc, err := json.Marshal(acc)
	if err != nil {
		return nil
	}
	return &MsgAccount{
		addr:         acc.GetAddress().String(),
		accountValue: string(jsonAcc),
	}
}

func GetMsgAccountKey(addr string) string {
	return prefixAccount + addr
}

func (msgAccount *MsgAccount) GetKey() string {
	return GetMsgAccountKey(msgAccount.addr)
}

func (msgAccount *MsgAccount) GetValue() string {
	return msgAccount.accountValue
}

type MsgState struct {
	addr  common.Address
	key   []byte
	value string
}

func NewMsgState(addr common.Address, key, value []byte) *MsgState {
	return &MsgState{
		addr:  addr,
		key:   key,
		value: hexutils.BytesToHex(value),
	}
}

func GetMsgStateKey(addr common.Address, key []byte) string {
	prefix := addr.Bytes()
	compositeKey := make([]byte, len(prefix)+len(key))

	copy(compositeKey, prefix)
	copy(compositeKey[len(prefix):], key)

	return ethcrypto.Keccak256Hash(compositeKey).String()
}

func (msgState *MsgState) GetKey() string {
	return GetMsgStateKey(msgState.addr, msgState.key)
}

func (msgState *MsgState) GetValue() string {
	return msgState.value
}

type MsgParams struct {
	types.Params
}

func NewMsgParams(params types.Params) *MsgParams {
	return &MsgParams{
		params,
	}
}

func (msgParams *MsgParams) GetKey() string {
	return prefixParams
}

func (msgParams *MsgParams) GetValue() string {
	jsonValue, err := json.Marshal(msgParams)
	if err != nil {
		panic(err)
	}
	return string(jsonValue)
}

type MsgContractBlockedListItem struct {
	addr sdk.AccAddress
}

func NewMsgContractBlockedListItem(addr sdk.AccAddress) *MsgContractBlockedListItem {
	return &MsgContractBlockedListItem{
		addr: addr,
	}
}

func (msgItem *MsgContractBlockedListItem) GetKey() string {
	return prefixBlackList + hexutils.BytesToHex(msgItem.addr)
}

func (msgItem *MsgContractBlockedListItem) GetValue() string {
	return ""
}

type MsgContractDeploymentWhitelistItem struct {
	addr sdk.AccAddress
}

func NewMsgContractDeploymentWhitelistItem(addr sdk.AccAddress) *MsgContractDeploymentWhitelistItem {
	return &MsgContractDeploymentWhitelistItem{
		addr: addr,
	}
}

func (msgItem *MsgContractDeploymentWhitelistItem) GetKey() string {
	return prefixWhiteList + hexutils.BytesToHex(msgItem.addr)
}

func (msgItem *MsgContractDeploymentWhitelistItem) GetValue() string {
	return ""
}
