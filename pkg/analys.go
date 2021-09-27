package pkg

import (
	"encoding/json"
	"fmt"
	"github.com/tendermint/tendermint/libs/log"
)

var singleAnalys *analyer

type analyer struct {
	logger          log.Logger
	status          bool
	currentTxIndex  int64
	blockHeight     int64
	startBeginBlock int64
	beginBlockCost  int64
	startdelliverTx int64
	delliverTxCost  int64
	startEndBlock   int64
	endBlockCost    int64
	startCommit     int64
	commitCost      int64
	allCost         int64
	tx              []*txLog
}

type txFormat struct {
	evm      int64                              `json:"evm_cost"`
	txDetail map[string]map[string]*operateInfo `json:"tx_detail"`
}

type blockFormat struct {
	blockHeight int64      `json:"height"`
	blockCost   int64      `json:"block_cost"`
	tx          []txFormat `json:"tx_list"`
}

func NewAnalys(log log.Logger, height int64) *analyer {
	singleAnalys = &analyer{
		logger:      log,
		status:      true,
		blockHeight: height,
	}
	return singleAnalys
}

func GetCurrentAnalys() *analyer {
	return singleAnalys
}

func  OnCommitExit() {
	if singleAnalys != nil {
		singleAnalys.OnCommitExit()
	}
	singleAnalys = nil
}

func CloseAnalys() {
	singleAnalys.Close()
}

func (s *analyer) OnAppBeginBlockEnter() {
	if s.status {
		s.startBeginBlock = GetNowTimeMs()
	}
}

func (s *analyer) OnAppBeginBlockExit() {
	if s.status {
		s.beginBlockCost = GetNowTimeMs() - s.startBeginBlock
	}
}

func (s *analyer) OnAppDeliverTxEnter() {
	if s.status {
		s.startdelliverTx = GetNowTimeMs()
		s.newTxLog()
	}
}

func (s *analyer) OnAppDeliverTxExit() {
	if s.status {
		s.beginBlockCost = GetNowTimeMs() - s.startdelliverTx
	}
}

func (s *analyer) OnAppEndBlockEnter() {
	if s.status {
		s.startEndBlock = GetNowTimeMs()
	}
}

func (s *analyer) OnAppEndBlockExit() {
	if s.status {
		s.endBlockCost = GetNowTimeMs() - s.startEndBlock
	}
}

func (s *analyer) OnCommitEnter() {
	if s.status {
		s.startCommit = GetNowTimeMs()
	}
}

func (s *analyer) OnCommitExit() {
	if s.status {
		s.commitCost = GetNowTimeMs() - s.startCommit
		//format to print log and release current
		s.formatLog()
	}
}

func (s *analyer) newTxLog() {
	s.currentTxIndex++
	s.tx = append(s.tx, newTxLog(module))
}

func (s *analyer) StartTxLog(module, oper string) {
	if s.status {
		if s.currentTxIndex > 0 && int64(len(s.tx)) == s.currentTxIndex {
			s.tx[s.currentTxIndex-1].StartTxLog(module, oper)
		}
	}
}

func (s *analyer) StopTxLog(module, oper string) {
	if s.status {
		if s.currentTxIndex > 0 && int64(len(s.tx)) == s.currentTxIndex {
			s.tx[s.currentTxIndex-1].StopTxLog(module, oper)
		}
	}
}

func (s *analyer) Close() {
	s.status = false
}

func (s *analyer) formatLog() {
	s.allCost = s.beginBlockCost + s.delliverTxCost + s.endBlockCost + s.commitCost
	var txs []txFormat
	block := &blockFormat{
		blockHeight: s.blockHeight,
		blockCost:   s.allCost,
	}

	for _, v := range s.tx {
		txMap := make(map[string]map[string]*operateInfo)
		for module, operInfo := range v.Record {
			if _, ok := txMap[module]; !ok {
				txMap[module] = make(map[string]*operateInfo)
			}
			for oper, detail := range operInfo.Record {
				if _, ok := txMap[module]; !ok {
					txMap[module][oper] = detail
				}
			}
		}
		txLocal := txFormat{
			evm:      v.EvmCost,
			txDetail: txMap,
		}
		txs = append(txs, txLocal)
	}

	block.tx = txs
	txsByte, _ := json.Marshal(txs)
	s.logger.Info(fmt.Sprintf(DEBUG_FORMAT, s.blockHeight, s.allCost, string(txsByte)))
}
