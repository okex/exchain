package app

import (
	"fmt"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/trace"
)

var once sync.Once

func init() {
	once.Do(func() {
		elapsedInfo := &ElapsedTimeInfos{
			infoMap: make(map[string]string),
		}
		trace.SetInfoObject(elapsedInfo)
	})
}

type ElapsedTimeInfos struct {
	infoMap     map[string]string
	elapsedTime int64
}

func (e *ElapsedTimeInfos) AddInfo(key string, info string) {
	if len(key) == 0 || len(info) == 0 {
		return
	}

	e.infoMap[key] = info
}

func (e *ElapsedTimeInfos) Dump(logger log.Logger) {

	if len(e.infoMap) == 0 {
		return
	}
	info := fmt.Sprintf("%s<%s>, %s<%s>, %s<%s>, %s[%s], %s[%s], %s[%s], %s[%s], %s[%s], %s[%s], %s[%s]",
		trace.Height, e.infoMap[trace.Height],
		trace.Tx, e.infoMap[trace.Tx],
		trace.GasUsed, e.infoMap[trace.GasUsed],
		trace.RunTx, e.infoMap[trace.RunTx],
		trace.Evm, e.infoMap[trace.Evm],
		"Iavl", e.infoMap["Iavl"],
		"DeliverTxs",  e.infoMap["DeliverTxs"],
		trace.Round, e.infoMap[trace.Round],
		trace.CommitRound, e.infoMap[trace.CommitRound],
		trace.Produce, e.infoMap[trace.Produce],
	)

	logger.Info(info)
	e.infoMap = make(map[string]string)
}

func (e *ElapsedTimeInfos) SetElapsedTime(elapsedTime int64) {
	e.elapsedTime = elapsedTime
}

func (e *ElapsedTimeInfos) GetElapsedTime() int64 {
	return e.elapsedTime
}
