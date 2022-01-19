package state

import (
	"encoding/hex"
	"fmt"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/trace"
	"sync/atomic"

	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

var err_delta_invoked = fmt.Errorf("prerrun stoped because of delta runnging")

type executionResult struct {
	res *ABCIResponses
	err error
}

type executionTask struct {
	height         int64
	index          int64
	block          *types.Block
	stopped        bool
	taskResultChan chan *executionTask
	result         *executionResult
	proxyApp       proxy.AppConnConsensus
	db             dbm.DB
	logger         log.Logger

	blockHash      string
	acquire IAcquire
	notifyC chan struct{}
	// why: atomic is better, if we use mutex ,we have to define more variables
	status int32
}

func newExecutionTask(blockExec *BlockExecutor, block *types.Block, index int64, c IAcquire) *executionTask {
	ret:=&executionTask{
		height:         block.Height,
		block:          block,
		db:             blockExec.db,
		proxyApp:       blockExec.proxyApp,
		logger:         blockExec.logger,
		taskResultChan: blockExec.prerunCtx.taskResultChan,
		index:          index,
		acquire:        c,
		notifyC:        make(chan struct{}),
	}
	ret.blockHash=hex.EncodeToString(block.Hash())

	return ret
}

func (e *executionTask) dump(when string) {

	e.logger.Info(when,
		"stopped", e.stopped,
		"Height", e.block.Height,
		"index", e.index,
		"blockHash", e.blockHash,
		//"AppHash", e.block.AppHash,
	)
}

func (t *executionTask) stop() {
	if t.stopped {
		return
	}
	t.stopped = true
}

func (t *executionTask) run() {
	needClose := true
	defer func() {
		if nil != t.notifyC && needClose {
			close(t.notifyC)
		}
	}()

	var (
		abciResponses *ABCIResponses
		err           error
	)
	trc := trace.NewTracer(fmt.Sprintf("num<%d>, lastRun", t.index))

	deltas, _ := t.acquire.acquire(t.block.Height)

	beginStatus := int32(TaskBeginByPrerunWithCacheExists)
	if deltas != nil && !deltas.Validate(t.block.Height) {
		t.dump(fmt.Sprintf("invalid delta,height=%d", t.block.Height))
		deltas = nil
	}
	if deltas != nil {
		t.dump("start beginBlock by  prerun_cache_delta")
		if !atomic.CompareAndSwapInt32(&t.status, 0, TaskBeginByPrerunWithCacheExists) {
			// case delta running
			needClose=false
			t.dump("prerun discard,because delta is running")
			return
		}
		// delta  already downloaded
		execBlockOnProxyAppWithDeltas(t.proxyApp, t.block, t.db)
		resp := ABCIResponses{}
		err = types.Json.Unmarshal(deltas.ABCIRsp(), &resp)
		abciResponses = &resp
	} else {
		t.dump("start beginBlock by prerun")
		if !atomic.CompareAndSwapInt32(&t.status, 0, TaskBeginByPrerunWithNoCache) {
			// case: delta get the beginBlock lock
			needClose=false
			return
		}
		if t.height != 1 {
			t.proxyApp.SetOptionSync(abci.RequestSetOption{Key: "ResetDeliverState"})
		}
		beginStatus = TaskBeginByPrerunWithNoCache
		abciResponses, err = execBlockOnProxyApp(t)
		if nil != err && err == err_delta_invoked {
			// just finish
			return
		}
	}

	notifyResult(t, abciResponses, err, beginStatus|TaskEndByPrerun, trc)
}

//========================================================
func (blockExec *BlockExecutor) InitPrerun() {
	if blockExec.deltaContext.downloadDelta {
		go blockExec.prerunCtx.consume()
	}
	go blockExec.prerunCtx.prerunRoutine()
}

func (blockExec *BlockExecutor) NotifyPrerun(block *types.Block) {
	if block.Height == 1+types.GetStartBlockHeight() {
		return
	}
	blockExec.prerunCtx.notifyPrerun(blockExec, block)
}
