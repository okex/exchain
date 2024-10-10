package state

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tendermint/go-amino"

	"github.com/okex/exchain/libs/system/trace"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	"github.com/okex/exchain/libs/tendermint/global"
	"github.com/okex/exchain/libs/tendermint/libs/automation"
	"github.com/okex/exchain/libs/tendermint/libs/fail"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	mempl "github.com/okex/exchain/libs/tendermint/mempool"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
	dbm "github.com/okex/exchain/libs/tm-db"
)

// -----------------------------------------------------------------------------
type (
	// Enum mode for executing [deliverTx, ...]
	DeliverTxsExecMode int
)

const (
	DeliverTxsExecModeSerial   DeliverTxsExecMode = iota // execute [deliverTx,...] sequentially
	DeliverTxsExecModeParallel                    = 2    // execute [deliverTx,...] parallel

	// There are two modes.
	// 0: execute [deliverTx,...] sequentially (default)
	// 1: execute [deliverTx,...] deprecated
	// 2: execute [deliverTx,...] parallel
	FlagDeliverTxsExecMode = "deliver-txs-mode"
	FlagEnableConcurrency  = "enable-concurrency"
)

// BlockExecutor handles block execution and state updates.
// It exposes ApplyBlock(), which validates & executes the block, updates state w/ ABCI responses,
// then commits and updates the mempool atomically, then saves state.

// BlockExecutor provides the context and accessories for properly executing a block.
type BlockExecutor struct {
	// save state, validators, consensus params, abci responses here
	db dbm.DB

	// execute the app against this
	proxyApp proxy.AppConnConsensus

	// events
	eventBus types.BlockEventPublisher

	// manage the mempool lock during commit
	// and update both with block results after commit.
	mempool mempl.Mempool
	evpool  EvidencePool

	logger  log.Logger
	metrics *Metrics

	// download or upload data to dds
	deltaContext *DeltaContext

	prerunCtx *prerunContext

	isFastSync bool

	// async save state, validators, consensus params, abci responses here
	asyncDBContext

	// the owner is validator
	isNullIndexer bool
	eventsChan    chan event
}

type event struct {
	block   *types.Block
	abciRsp *ABCIResponses
	vals    []*types.Validator
}

type BlockExecutorOption func(executor *BlockExecutor)

func BlockExecutorWithMetrics(metrics *Metrics) BlockExecutorOption {
	return func(blockExec *BlockExecutor) {
		blockExec.metrics = metrics
	}
}

// NewBlockExecutor returns a new BlockExecutor with a NopEventBus.
// Call SetEventBus to provide one.
func NewBlockExecutor(
	db dbm.DB,
	logger log.Logger,
	proxyApp proxy.AppConnConsensus,
	mempool mempl.Mempool,
	evpool EvidencePool,
	options ...BlockExecutorOption,
) *BlockExecutor {
	res := &BlockExecutor{
		db:           db,
		proxyApp:     proxyApp,
		eventBus:     types.NopEventBus{},
		mempool:      mempool,
		evpool:       evpool,
		logger:       logger,
		metrics:      NopMetrics(),
		prerunCtx:    newPrerunContex(logger),
		deltaContext: newDeltaContext(logger),
		eventsChan:   make(chan event, 5),
	}

	for _, option := range options {
		option(res)
	}
	automation.LoadTestCase(logger)
	res.deltaContext.init()

	res.initAsyncDBContext()

	return res
}

func (blockExec *BlockExecutor) fireEventsRountine() {
	for et := range blockExec.eventsChan {
		if et.block == nil {
			break
		}
		fireEvents(blockExec.logger, blockExec.eventBus, et.block, et.abciRsp, et.vals)
	}
	blockExec.wg.Done()
}

func (blockExec *BlockExecutor) DB() dbm.DB {
	return blockExec.db
}

func (blockExec *BlockExecutor) SetIsFastSyncing(isSyncing bool) {
	blockExec.isFastSync = isSyncing
}

func (blockExec *BlockExecutor) SetIsNullIndexer(isNullIndexer bool) {
	blockExec.isNullIndexer = isNullIndexer
}

func (blockExec *BlockExecutor) Stop() {
	blockExec.stopAsyncDBContext()
}

// SetEventBus - sets the event bus for publishing block related events.
// If not called, it defaults to types.NopEventBus.
func (blockExec *BlockExecutor) SetEventBus(eventBus types.BlockEventPublisher) {
	blockExec.eventBus = eventBus
	blockExec.mempool.SetEventBus(eventBus)
}

// CreateProposalBlock calls state.MakeBlock with evidence from the evpool
// and txs from the mempool. The max bytes must be big enough to fit the commit.
// Up to 1/10th of the block space is allcoated for maximum sized evidence.
// The rest is given to txs, up to the max gas.
func (blockExec *BlockExecutor) CreateProposalBlock(
	height int64,
	state State, commit *types.Commit,
	proposerAddr []byte,
) (*types.Block, *types.PartSet) {

	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas

	// Fetch a limited amount of valid evidence
	maxNumEvidence, _ := types.MaxEvidencePerBlock(maxBytes)
	evidence := blockExec.evpool.PendingEvidence(maxNumEvidence)

	// Fetch a limited amount of valid txs
	maxDataBytes := types.MaxDataBytes(maxBytes, state.Validators.Size(), len(evidence))
	if cfg.DynamicConfig.GetMaxGasUsedPerBlock() > -1 {
		maxGas = cfg.DynamicConfig.GetMaxGasUsedPerBlock()
	}
	txs := blockExec.mempool.ReapMaxBytesMaxGas(maxDataBytes, maxGas)

	return state.MakeBlock(height, txs, commit, evidence, proposerAddr)
}

// ValidateBlock validates the given block against the given state.
// If the block is invalid, it returns an error.
// Validation does not mutate state, but does require historical information from the stateDB,
// ie. to verify evidence from a validator at an old height.
func (blockExec *BlockExecutor) ValidateBlock(state State, block *types.Block) error {
	if IgnoreSmbCheck {
		// debug only
		return nil
	}
	return validateBlock(blockExec.evpool, blockExec.db, state, block)
}

// ApplyBlock validates the block against the state, executes it against the app,
// fires the relevant events, commits the app, and saves the new state and responses.
// It returns the new state and the block height to retain (pruning older blocks).
// It's the only function that needs to be called
// from outside this package to process and commit an entire block.
// It takes a blockID to avoid recomputing the parts hash.
func (blockExec *BlockExecutor) ApplyBlock(
	state State, blockID types.BlockID, block *types.Block) (State, int64, error) {
	if ApplyBlockPprofTime >= 0 {
		f, t := PprofStart()
		defer PprofEnd(int(block.Height), f, t)
	}

	trc := trace.NewTracer(trace.ApplyBlock)
	trc.EnableSummary()
	trc.SetWorkloadStatistic(trace.GetApplyBlockWorkloadSttistic())
	dc := blockExec.deltaContext

	defer func() {
		trace.GetElapsedInfo().AddInfo(trace.Height, strconv.FormatInt(block.Height, 10))
		trace.GetElapsedInfo().AddInfo(trace.Tx, strconv.Itoa(len(block.Data.Txs)))
		trace.GetElapsedInfo().AddInfo(trace.BlockSize, strconv.Itoa(block.FastSize()))
		trace.GetElapsedInfo().AddInfo(trace.ApplyBlock, trc.Format())
		trace.GetElapsedInfo().AddInfo(trace.Workload, trace.GetApplyBlockWorkloadSttistic().Format())
		trace.GetElapsedInfo().SetElapsedTime(trc.GetElapsedTime())

		now := time.Now().UnixNano()
		blockExec.metrics.IntervalTime.Set(float64(now-blockExec.metrics.lastBlockTime) / 1e6)
		blockExec.metrics.lastBlockTime = now
		blockExec.metrics.CommittedHeight.Set(float64(block.Height))
	}()

	if err := blockExec.ValidateBlock(state, block); err != nil {
		return state, 0, ErrInvalidBlock(err)
	}

	deltaInfo := dc.prepareStateDelta(block.Height)

	trc.Pin(trace.Abci)
	if cfg.DynamicConfig.GetEnablePGU() {
		global.CommitLock()
		defer global.CommitUnlock()
	}

	startTime := time.Now().UnixNano()

	//wait till the last block async write be saved
	blockExec.tryWaitLastBlockSave(block.Height - 1)

	abciResponses, duration, err := blockExec.runAbci(block, deltaInfo)

	// publish event
	if types.EnableEventBlockTime {
		blockExec.FireBlockTimeEvents(block.Height, len(block.Txs), true)
		if !blockExec.isNullIndexer {
			blockExec.eventsChan <- event{
				block:   block,
				abciRsp: abciResponses,
			}
		}
	}

	trace.GetElapsedInfo().AddInfo(trace.LastRun, fmt.Sprintf("%dms", duration.Milliseconds()))
	trace.GetApplyBlockWorkloadSttistic().Add(trace.LastRun, time.Now(), duration)

	if err != nil {
		return state, 0, ErrProxyAppConn(err)
	}

	fail.Fail() // XXX

	// Save the results before we commit.
	blockExec.trySaveABCIResponsesAsync(block.Height, abciResponses)

	fail.Fail() // XXX
	endTime := time.Now().UnixNano()
	blockExec.metrics.BlockProcessingTime.Observe(float64(endTime-startTime) / 1e6)
	blockExec.metrics.AbciTime.Set(float64(endTime-startTime) / 1e6)

	// validate the validator updates and convert to tendermint types
	abciValUpdates := abciResponses.EndBlock.ValidatorUpdates
	err = validateValidatorUpdates(abciValUpdates, state.ConsensusParams.Validator)
	if err != nil {
		return state, 0, fmt.Errorf("error in validator updates: %v", err)
	}
	validatorUpdates, err := types.PB2TM.ValidatorUpdates(abciValUpdates)
	if err != nil {
		return state, 0, err
	}
	if len(validatorUpdates) > 0 {
		blockExec.logger.Info("Updates to validators", "updates", types.ValidatorListString(validatorUpdates))
	}

	// Update the state with the block and responses.
	state, err = updateState(state, blockID, &block.Header, abciResponses, validatorUpdates)
	if err != nil {
		return state, 0, fmt.Errorf("commit failed for application: %v", err)
	}

	trc.Pin(trace.Persist)
	startTime = time.Now().UnixNano()

	// Lock mempool, commit app state, update mempoool.
	commitResp, retainHeight, err := blockExec.commit(state, block, deltaInfo, abciResponses.DeliverTxs, trc)
	endTime = time.Now().UnixNano()
	blockExec.metrics.CommitTime.Set(float64(endTime-startTime) / 1e6)
	if err != nil {
		return state, 0, fmt.Errorf("commit failed for application: %v", err)
	}
	global.SetGlobalHeight(block.Height)

	// Update evpool with the block and state.
	blockExec.evpool.Update(block, state)

	fail.Fail() // XXX

	trc.Pin("SaveState")
	// Update the app hash and save the state.
	state.AppHash = commitResp.Data
	blockExec.trySaveStateAsync(state)

	blockExec.logger.Debug("SaveState", "state", &state)
	fail.Fail() // XXX

	dc.postApplyBlock(block.Height, deltaInfo, abciResponses, commitResp.DeltaMap, blockExec.isFastSync)

	// Events are fired after everything else.
	// NOTE: if we crash between Commit and Save, events wont be fired during replay
	if !types.EnableEventBlockTime {
		if !blockExec.isNullIndexer {
			blockExec.eventsChan <- event{
				block:   block,
				abciRsp: abciResponses,
			}
		}
	}

	return state, retainHeight, nil
}

func (blockExec *BlockExecutor) ApplyBlockWithTrace(
	state State, blockID types.BlockID, block *types.Block) (State, int64, error) {
	s, id, err := blockExec.ApplyBlock(state, blockID, block)
	trace.GetElapsedInfo().Dump(blockExec.logger.With("module", "main"))
	return s, id, err
}

func (blockExec *BlockExecutor) runAbci(block *types.Block, deltaInfo *DeltaInfo) (*ABCIResponses, time.Duration, error) {
	var abciResponses *ABCIResponses
	var err error
	var duration time.Duration

	if deltaInfo != nil {
		blockExec.logger.Info("Apply delta", "height", block.Height, "deltas-length", deltaInfo.deltaLen)
		t0 := time.Now()
		execBlockOnProxyAppWithDeltas(blockExec.proxyApp, block, blockExec.db)
		abciResponses = deltaInfo.abciResponses
		duration = time.Now().Sub(t0)
	} else {
		pc := blockExec.prerunCtx
		if pc.prerunTx {
			abciResponses, duration, err = pc.getPrerunResult(block)
		}

		if abciResponses == nil {
			t0 := time.Now()
			ctx := &executionTask{
				logger:   blockExec.logger,
				block:    block,
				db:       blockExec.db,
				proxyApp: blockExec.proxyApp,
			}
			mode := DeliverTxsExecMode(cfg.DynamicConfig.GetDeliverTxsExecuteMode())
			switch mode {
			case DeliverTxsExecModeSerial:
				abciResponses, err = execBlockOnProxyApp(ctx)
			case DeliverTxsExecModeParallel:
				abciResponses, err = execBlockOnProxyAppAsync(blockExec.logger, blockExec.proxyApp, block, blockExec.db)
			default:
				abciResponses, err = execBlockOnProxyApp(ctx)
			}
			duration = time.Now().Sub(t0)
		}
	}

	return abciResponses, duration, err
}

// Commit locks the mempool, runs the ABCI Commit message, and updates the
// mempool.
// It returns the result of calling abci.Commit (the AppHash) and the height to retain (if any).
// The Mempool must be locked during commit and update because state is
// typically reset on Commit and old txs must be replayed against committed
// state before new txs are run in the mempool, lest they be invalid.
func (blockExec *BlockExecutor) commit(
	state State,
	block *types.Block,
	deltaInfo *DeltaInfo,
	deliverTxResponses []*abci.ResponseDeliverTx,
	trc *trace.Tracer,
) (*abci.ResponseCommit, int64, error) {
	blockExec.mempool.Lock()
	defer func() {
		blockExec.mempool.Unlock()
		// Forced flushing mempool
		if cfg.DynamicConfig.GetMempoolFlush() {
			blockExec.mempool.Flush()
		}
	}()

	// while mempool is Locked, flush to ensure all async requests have completed
	// in the ABCI app before Commit.
	err := blockExec.mempool.FlushAppConn()
	if err != nil {
		blockExec.logger.Error("Client error during mempool.FlushAppConn", "err", err)
		return nil, 0, err
	}

	// Commit block, get hash back
	var treeDeltaMap interface{}
	if deltaInfo != nil {
		treeDeltaMap = deltaInfo.treeDeltaMap
	}
	res, err := blockExec.proxyApp.CommitSync(abci.RequestCommit{DeltaMap: treeDeltaMap})
	if err != nil {
		blockExec.logger.Error(
			"Client error during proxyAppConn.CommitSync",
			"err", err,
		)
		return nil, 0, err
	}

	// ResponseCommit has no error code - just data
	blockExec.logger.Debug(
		"Committed state",
		"height", block.Height,
		"txs", len(block.Txs),
		"appHash", amino.BytesHexStringer(res.Data),
		"blockLen", amino.FuncStringer(func() string { return strconv.Itoa(block.FastSize()) }),
	)

	trc.Pin("mpUpdate")
	// Update mempool.
	err = blockExec.mempool.Update(
		block.Height,
		block.Txs,
		deliverTxResponses,
		TxPreCheck(state),
		TxPostCheck(state),
	)

	if !cfg.DynamicConfig.GetMempoolRecheck() && block.Height%cfg.DynamicConfig.GetMempoolForceRecheckGap() == 0 {
		proxyCb := func(req *abci.Request, res *abci.Response) {

		}
		blockExec.proxyApp.SetResponseCallback(proxyCb)
		// reset checkState
		blockExec.proxyApp.SetOptionAsync(abci.RequestSetOption{
			Key: "ResetCheckState",
		})
	}

	return res, res.RetainHeight, err
}

func (blockExec *BlockExecutor) MempoolLogPgu(isBlockProducer bool) error {
	clistmempool, ok := blockExec.mempool.(*mempl.CListMempool)
	if !ok {
		return fmt.Errorf("error on Printing PGU Log SimTx and SimGasUsed in mempool")
	}
	clistmempool.LogPgu(isBlockProducer)
	return nil
}

func transTxsToBytes(txs types.Txs) [][]byte {
	ret := make([][]byte, 0)
	for _, v := range txs {
		ret = append(ret, v)
	}
	return ret
}

//---------------------------------------------------------
// Helper functions for executing blocks and updating state

// Executes block's transactions on proxyAppConn.
// Returns a list of transaction results and updates to the validator set
func execBlockOnProxyApp(context *executionTask) (*ABCIResponses, error) {
	block := context.block
	proxyAppConn := context.proxyApp
	stateDB := context.db
	logger := context.logger

	var validTxs, invalidTxs = 0, 0

	txIndex := 0
	abciResponses := NewABCIResponses(block)

	// Execute transactions and get hash.
	proxyCb := func(req *abci.Request, res *abci.Response) {
		if r, ok := res.Value.(*abci.Response_DeliverTx); ok {
			// TODO: make use of res.Log
			// TODO: make use of this info
			// Blocks may include invalid txs.
			txRes := r.DeliverTx
			if txRes.Code == abci.CodeTypeOK {
				validTxs++
			} else {
				logger.Debug("Invalid tx", "code", txRes.Code, "log", txRes.Log, "index", txIndex)
				invalidTxs++
			}
			abciResponses.DeliverTxs[txIndex] = txRes
			txIndex++
		}
	}
	proxyAppConn.SetResponseCallback(proxyCb)

	// proxyAppConn.ParallelTxs(transTxsToBytes(block.Txs), true)
	commitInfo, byzVals := getBeginBlockValidatorInfo(block, stateDB)

	// Begin block
	var err error
	abciResponses.BeginBlock, err = proxyAppConn.BeginBlockSync(abci.RequestBeginBlock{
		Hash:                block.Hash(),
		Header:              types.TM2PB.Header(&block.Header),
		LastCommitInfo:      commitInfo,
		ByzantineValidators: byzVals,
	})
	if err != nil {
		logger.Error("Error in proxyAppConn.BeginBlock", "err", err)
		return nil, err
	}

	realTxCh := make(chan abci.TxEssentials, len(block.Txs))
	stopedCh := make(chan struct{}, 1)

	go preDeliverRoutine(proxyAppConn, block.Txs, realTxCh, stopedCh)

	count := 0
	for realTx := range realTxCh {
		if realTx != nil {
			proxyAppConn.DeliverRealTxAsync(realTx)
		} else {
			proxyAppConn.DeliverTxAsync(abci.RequestDeliverTx{Tx: block.Txs[count]})
		}

		if err = proxyAppConn.Error(); err != nil {
			return nil, err
		}
		if context != nil && context.stopped {
			stopedCh <- struct{}{}
			close(stopedCh)
			context.dump(fmt.Sprintf("Prerun stopped, %d/%d tx executed", count+1, len(block.Txs)))
			return nil, fmt.Errorf("Prerun stopped")
		}
		count += 1
	}
	close(stopedCh)

	// End block.
	abciResponses.EndBlock, err = proxyAppConn.EndBlockSync(abci.RequestEndBlock{Height: block.Height})
	if err != nil {
		logger.Error("Error in proxyAppConn.EndBlock", "err", err)
		return nil, err
	}

	trace.GetElapsedInfo().AddInfo(trace.InvalidTxs, fmt.Sprintf("%d", invalidTxs))

	return abciResponses, nil
}

func preDeliverRoutine(proxyAppConn proxy.AppConnConsensus, txs types.Txs, realTxCh chan<- abci.TxEssentials, stopedCh <-chan struct{}) {
	for _, tx := range txs {
		realTx := proxyAppConn.PreDeliverRealTxAsync(tx)
		select {
		case realTxCh <- realTx:
		case <-stopedCh:
			close(realTxCh)
			return
		}
	}
	close(realTxCh)
}

func execBlockOnProxyAppWithDeltas(
	proxyAppConn proxy.AppConnConsensus,
	block *types.Block,
	stateDB dbm.DB,
) {
	proxyCb := func(req *abci.Request, res *abci.Response) {
	}
	proxyAppConn.SetResponseCallback(proxyCb)

	commitInfo, byzVals := getBeginBlockValidatorInfo(block, stateDB)
	_, _ = proxyAppConn.BeginBlockSync(abci.RequestBeginBlock{
		Hash:                block.Hash(),
		Header:              types.TM2PB.Header(&block.Header),
		LastCommitInfo:      commitInfo,
		ByzantineValidators: byzVals,
	})
}

func getBeginBlockValidatorInfo(block *types.Block, stateDB dbm.DB) (abci.LastCommitInfo, []abci.Evidence) {
	voteInfos := make([]abci.VoteInfo, block.LastCommit.Size())
	// block.Height=1 -> LastCommitInfo.Votes are empty.
	// Remember that the first LastCommit is intentionally empty, so it makes
	// sense for LastCommitInfo.Votes to also be empty.
	if block.Height > types.GetStartBlockHeight()+1 {
		lastValSet, err := LoadValidators(stateDB, block.Height-1)
		if err != nil {
			panic(err)
		}

		// Sanity check that commit size matches validator set size - only applies
		// after first block.
		var (
			commitSize = block.LastCommit.Size()
			valSetLen  = len(lastValSet.Validators)
		)
		if commitSize != valSetLen {
			panic(fmt.Sprintf("commit size (%d) doesn't match valset length (%d) at height %d\n\n%v\n\n%v",
				commitSize, valSetLen, block.Height, block.LastCommit.Signatures, lastValSet.Validators))
		}

		for i, val := range lastValSet.Validators {
			commitSig := block.LastCommit.Signatures[i]
			voteInfos[i] = abci.VoteInfo{
				Validator:       types.TM2PB.Validator(val),
				SignedLastBlock: !commitSig.Absent(),
			}
		}
	}

	byzVals := make([]abci.Evidence, len(block.Evidence.Evidence))
	for i, ev := range block.Evidence.Evidence {
		// We need the validator set. We already did this in validateBlock.
		// TODO: Should we instead cache the valset in the evidence itself and add
		// `SetValidatorSet()` and `ToABCI` methods ?
		valset, err := LoadValidators(stateDB, ev.Height())
		if err != nil {
			panic(err)
		}
		byzVals[i] = types.TM2PB.Evidence(ev, valset, block.Time)
	}

	return abci.LastCommitInfo{
		Round: int32(block.LastCommit.Round),
		Votes: voteInfos,
	}, byzVals
}

func validateValidatorUpdates(abciUpdates []abci.ValidatorUpdate,
	params types.ValidatorParams) error {
	for _, valUpdate := range abciUpdates {
		if valUpdate.GetPower() < 0 {
			return fmt.Errorf("voting power can't be negative %v", valUpdate)
		} else if valUpdate.GetPower() == 0 {
			// continue, since this is deleting the validator, and thus there is no
			// pubkey to check
			continue
		}

		// Check if validator's pubkey matches an ABCI type in the consensus params
		thisKeyType := valUpdate.PubKey.Type
		if !params.IsValidPubkeyType(thisKeyType) {
			return fmt.Errorf("validator %v is using pubkey %s, which is unsupported for consensus",
				valUpdate, thisKeyType)
		}
	}
	return nil
}

// updateState returns a new State updated according to the header and responses.
func updateState(
	state State,
	blockID types.BlockID,
	header *types.Header,
	abciResponses *ABCIResponses,
	validatorUpdates []*types.Validator,
) (State, error) {

	// Copy the valset so we can apply changes from EndBlock
	// and update s.LastValidators and s.Validators.
	nValSet := state.NextValidators.Copy()

	// Update the validator set with the latest abciResponses.
	lastHeightValsChanged := state.LastHeightValidatorsChanged
	if len(validatorUpdates) > 0 {
		err := nValSet.UpdateWithChangeSet(validatorUpdates)
		if err != nil {
			return state, fmt.Errorf("error changing validator set: %v", err)
		}
		// Change results from this height but only applies to the next next height.
		lastHeightValsChanged = header.Height + 1 + 1
	}

	// Update validator proposer priority and set state variables.
	nValSet.IncrementProposerPriority(1)

	// Update the params with the latest abciResponses.
	nextParams := state.ConsensusParams
	lastHeightParamsChanged := state.LastHeightConsensusParamsChanged
	if abciResponses.EndBlock.ConsensusParamUpdates != nil {
		// NOTE: must not mutate s.ConsensusParams
		nextParams = state.ConsensusParams.Update(abciResponses.EndBlock.ConsensusParamUpdates)
		err := nextParams.Validate()
		if err != nil {
			return state, fmt.Errorf("error updating consensus params: %v", err)
		}
		// Change results from this height but only applies to the next height.
		lastHeightParamsChanged = header.Height + 1
	}

	// TODO: allow app to upgrade version
	nextVersion := state.Version
	if types.HigherThanVenus1(header.Height) && !state.Version.IsUpgraded() {
		nextVersion = state.Version.UpgradeToIBCVersion()
	}

	// NOTE: the AppHash has not been populated.
	// It will be filled on state.Save.
	return State{
		Version:                          nextVersion,
		ChainID:                          state.ChainID,
		LastBlockHeight:                  header.Height,
		LastBlockID:                      blockID,
		LastBlockTime:                    header.Time,
		NextValidators:                   nValSet,
		Validators:                       state.NextValidators.Copy(),
		LastValidators:                   state.Validators.Copy(),
		LastHeightValidatorsChanged:      lastHeightValsChanged,
		ConsensusParams:                  nextParams,
		LastHeightConsensusParamsChanged: lastHeightParamsChanged,
		LastResultsHash:                  abciResponses.ResultsHash(),
		AppHash:                          nil,
	}, nil
}

// Fire NewBlock, NewBlockHeader.
// Fire TxEvent for every tx.
// NOTE: if Tendermint crashes before commit, some or all of these events may be published again.
func fireEvents(
	logger log.Logger,
	eventBus types.BlockEventPublisher,
	block *types.Block,
	abciResponses *ABCIResponses,
	validatorUpdates []*types.Validator,
) {
	eventBus.PublishEventNewBlock(types.EventDataNewBlock{
		Block:            block,
		ResultBeginBlock: *abciResponses.BeginBlock,
		ResultEndBlock:   *abciResponses.EndBlock,
	})
	eventBus.PublishEventNewBlockHeader(types.EventDataNewBlockHeader{
		Header:           block.Header,
		NumTxs:           int64(len(block.Txs)),
		ResultBeginBlock: *abciResponses.BeginBlock,
		ResultEndBlock:   *abciResponses.EndBlock,
	})

	//publish tx event 1by1
	for i, tx := range block.Data.Txs {
		eventBus.PublishEventTx(types.EventDataTx{TxResult: types.TxResult{
			Height: block.Height,
			Index:  uint32(i),
			Tx:     tx,
			Result: *(abciResponses.DeliverTxs[i]),
		}})
	}

	//publish batch txs event
	if len(block.Data.Txs) > 0 {
		eventBus.PublishEventTxs(types.EventDataTxs{
			Height:  block.Height,
			Results: abciResponses.DeliverTxs,
		})
	}

	//if len(validatorUpdates) > 0 {
	//	eventBus.PublishEventValidatorSetUpdates(
	//		types.EventDataValidatorSetUpdates{ValidatorUpdates: validatorUpdates})
	//}
}

func (blockExec *BlockExecutor) FireBlockTimeEvents(height int64, txNum int, available bool) {
	blockExec.eventBus.PublishEventLatestBlockTime(
		types.EventDataBlockTime{Height: height, TimeNow: tmtime.Now().UnixMilli(), TxNum: txNum, Available: available})
}
