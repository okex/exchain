package consensus

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/okex/exchain/libs/iavl"
	iavlcfg "github.com/okex/exchain/libs/iavl/config"
	"github.com/okex/exchain/libs/system/trace"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/libs/fail"
	tmos "github.com/okex/exchain/libs/tendermint/libs/os"
	sm "github.com/okex/exchain/libs/tendermint/state"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
	"time"
)

func (cs *State) dumpElapsed(trc *trace.Tracer, schema string) {
	trace.GetElapsedInfo().AddInfo(schema, trc.Format())
	trc.Reset()
}

func (cs *State) initNewHeight() {
	// waiting finished and enterNewHeight by timeoutNewHeight
	if cs.Step == cstypes.RoundStepNewHeight {
		// init StartTime
		cs.StartTime = tmtime.Now()
		cs.dumpElapsed(cs.blockTimeTrc, trace.LastBlockTime)
		cs.traceDump()
	}
}

func (cs *State) traceDump() {
	if cs.Logger == nil {
		return
	}

	trace.GetElapsedInfo().AddInfo(trace.CommitRound, fmt.Sprintf("%d", cs.CommitRound))
	trace.GetElapsedInfo().AddInfo(trace.Round, fmt.Sprintf("%d", cs.Round))
	trace.GetElapsedInfo().AddInfo(trace.BlockParts, fmt.Sprintf("%d|%d|%d|%d/%d",
		cs.bt.droppedDue2WrongHeight,
		cs.bt.droppedDue2NotExpected,
		cs.bt.droppedDue2Error,
		cs.bt.droppedDue2NotAdded,
		cs.bt.totalParts,
	))

	trace.GetElapsedInfo().AddInfo(trace.BlockPartsP2P, fmt.Sprintf("%d|%d|%d",
		cs.bt.bpNOTransByACK, cs.bt.bpNOTransByData, cs.bt.bpSend))

	trace.GetElapsedInfo().AddInfo(trace.Produce, cs.trc.Format())
	trace.GetElapsedInfo().Dump(cs.Logger.With("module", "main"))
	cs.trc.Reset()
}

// Enter: +2/3 precommits for block
func (cs *State) enterCommit(height int64, commitRound int) {
	logger := cs.Logger.With("height", height, "commitRound", commitRound)

	if cs.Height != height || cstypes.RoundStepCommit <= cs.Step {
		logger.Debug(fmt.Sprintf(
			"enterCommit(%v/%v): Invalid args. Current step: %v/%v/%v",
			height,
			commitRound,
			cs.Height,
			cs.Round,
			cs.Step))
		return
	}

	cs.initNewHeight()
	cs.trc.Pin("%s-%d", "Commit", cs.Round)

	logger.Info(fmt.Sprintf("enterCommit(%v/%v). Current: %v/%v/%v", height, commitRound, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterCommit:
		// keep cs.Round the same, commitRound points to the right Precommits set.
		cs.updateRoundStep(cs.Round, cstypes.RoundStepCommit)
		cs.CommitRound = commitRound
		cs.newStep()

		// Maybe finalize immediately.
		cs.tryFinalizeCommit(height)
	}()

	blockID, ok := cs.Votes.Precommits(commitRound).TwoThirdsMajority()
	if !ok {
		panic("RunActionCommit() expects +2/3 precommits")
	}

	// The Locked* fields no longer matter.
	// Move them over to ProposalBlock if they match the commit hash,
	// otherwise they'll be cleared in updateToState.
	if cs.LockedBlock.HashesTo(blockID.Hash) {
		logger.Info("Commit is for locked block. Set ProposalBlock=LockedBlock", "blockHash", blockID.Hash)
		cs.ProposalBlock = cs.LockedBlock
		cs.ProposalBlockParts = cs.LockedBlockParts
	}

	// If we don't have the block being committed, set up to get it.
	if !cs.ProposalBlock.HashesTo(blockID.Hash) {
		if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {
			logger.Info(
				"Commit is for a block we don't know about. Set ProposalBlock=nil",
				"proposal",
				cs.ProposalBlock.Hash(),
				"commit",
				blockID.Hash)
			// We're getting the wrong block.
			// Set up ProposalBlockParts and keep waiting.
			cs.ProposalBlock = nil
			cs.Logger.Info("enterCommit proposalBlockPart reset ,because of mismatch hash,",
				"origin", hex.EncodeToString(cs.ProposalBlockParts.Hash()), "after", blockID.Hash)
			cs.ProposalBlockParts = types.NewPartSetFromHeader(blockID.PartsHeader)
			cs.eventBus.PublishEventValidBlock(cs.RoundStateEvent())
			cs.evsw.FireEvent(types.EventValidBlock, &cs.RoundState)
		}
		// else {
		// We just need to keep waiting.
		// }
	}
}

// If we have the block AND +2/3 commits for it, finalize.
func (cs *State) tryFinalizeCommit(height int64) {
	logger := cs.Logger.With("height", height)

	if cs.Height != height {
		panic(fmt.Sprintf("tryFinalizeCommit() cs.Height: %v vs height: %v", cs.Height, height))
	}

	blockID, ok := cs.Votes.Precommits(cs.CommitRound).TwoThirdsMajority()
	if !ok || len(blockID.Hash) == 0 {
		logger.Error("Attempt to finalize failed. There was no +2/3 majority, or +2/3 was for <nil>.")
		return
	}
	if !cs.ProposalBlock.HashesTo(blockID.Hash) {
		// TODO: this happens every time if we're not a validator (ugly logs)
		// TODO: ^^ wait, why does it matter that we're a validator?
		logger.Info(
			"Attempt to finalize failed. We don't have the commit block.",
			"proposal-block",
			cs.ProposalBlock.Hash(),
			"commit-block",
			blockID.Hash)
		return
	}

	//	go
	cs.finalizeCommit(height)
}

// Increment height and goto cstypes.RoundStepNewHeight
func (cs *State) finalizeCommit(height int64) {
	if cs.Height != height || cs.Step != cstypes.RoundStepCommit {
		cs.Logger.Debug(fmt.Sprintf(
			"finalizeCommit(%v): Invalid args. Current step: %v/%v/%v",
			height,
			cs.Height,
			cs.Round,
			cs.Step))
		return
	}

	blockID, ok := cs.Votes.Precommits(cs.CommitRound).TwoThirdsMajority()
	block, blockParts := cs.ProposalBlock, cs.ProposalBlockParts

	if !ok {
		panic(fmt.Sprintf("Cannot finalizeCommit, commit does not have two thirds majority"))
	}
	if !blockParts.HasHeader(blockID.PartsHeader) {
		panic(fmt.Sprintf("Expected ProposalBlockParts header to be commit header"))
	}
	if !block.HashesTo(blockID.Hash) {
		panic(fmt.Sprintf("Cannot finalizeCommit, ProposalBlock does not hash to commit hash"))
	}
	if err := cs.blockExec.ValidateBlock(cs.state, block); err != nil {
		panic(fmt.Sprintf("+2/3 committed an invalid block: %v", err))
	}

	cs.Logger.Info("Finalizing commit of block with N txs",
		"height", block.Height,
		"hash", block.Hash(),
		"root", block.AppHash,
		"N", len(block.Txs))
	cs.Logger.Info(fmt.Sprintf("%v", block))

	fail.Fail() // XXX

	// Save to blockStore.
	blockTime := block.Time
	if cs.blockStore.Height() < block.Height {
		// NOTE: the seenCommit is local justification to commit this block,
		// but may differ from the LastCommit included in the next block
		precommits := cs.Votes.Precommits(cs.CommitRound)
		seenCommit := precommits.MakeCommit()
		blockTime = sm.MedianTime(seenCommit, cs.Validators)
		cs.blockStore.SaveBlock(block, blockParts, seenCommit)
	} else {
		// Happens during replay if we already saved the block but didn't commit
		cs.Logger.Info("Calling finalizeCommit on already stored block", "height", block.Height)
	}
	trace.GetElapsedInfo().AddInfo(trace.BTInterval, fmt.Sprintf("%dms", blockTime.Sub(block.Time).Milliseconds()))

	fail.Fail() // XXX

	// Write EndHeightMessage{} for this height, implying that the blockstore
	// has saved the block.
	//
	// If we crash before writing this EndHeightMessage{}, we will recover by
	// running ApplyBlock during the ABCI handshake when we restart.  If we
	// didn't save the block to the blockstore before writing
	// EndHeightMessage{}, we'd have to change WAL replay -- currently it
	// complains about replaying for heights where an #ENDHEIGHT entry already
	// exists.
	//
	// Either way, the State should not be resumed until we
	// successfully call ApplyBlock (ie. later here, or in Handshake after
	// restart).
	endMsg := EndHeightMessage{height}
	if err := cs.wal.WriteSync(endMsg); err != nil { // NOTE: fsync
		panic(fmt.Sprintf("Failed to write %v msg to consensus wal due to %v. Check your FS and restart the node",
			endMsg, err))
	}

	fail.Fail() // XXX

	// Create a copy of the state for staging and an event cache for txs.
	stateCopy := cs.state.Copy()

	// Execute and commit the block, update and save the state, and update the mempool.
	// NOTE The block.AppHash wont reflect these txs until the next block.

	var err error
	var retainHeight int64

	cs.trc.Pin("%s", trace.ApplyBlock)

	if iavl.EnableAsyncCommit {
		cs.handleCommitGapOffset(height)
	}

	stateCopy, retainHeight, err = cs.blockExec.ApplyBlock(
		stateCopy,
		types.BlockID{Hash: block.Hash(), PartsHeader: blockParts.Header()},
		block)
	if err != nil {
		cs.Logger.Error("Error on ApplyBlock. Did the application crash? Please restart tendermint", "err", err)
		err := tmos.Kill()
		if err != nil {
			cs.Logger.Error("Failed to kill this process - please do so manually", "err", err)
		}
		return
	}

	//reset offset after commitGap
	if iavl.EnableAsyncCommit &&
		height%iavlcfg.DynamicConfig.GetCommitGapHeight() == iavl.GetFinalCommitGapOffset() {
		iavl.SetFinalCommitGapOffset(0)
	}

	fail.Fail() // XXX

	cs.trc.Pin("%s", trace.UpdateState)

	// Prune old heights, if requested by ABCI app.
	if retainHeight > 0 {
		pruned, err := cs.pruneBlocks(retainHeight)
		if err != nil {
			cs.Logger.Error("Failed to prune blocks", "retainHeight", retainHeight, "err", err)
		} else {
			cs.Logger.Info("Pruned blocks", "pruned", pruned, "retainHeight", retainHeight)
		}
	}

	// must be called before we update state
	cs.recordMetrics(height, block)

	// NewHeightStep!
	cs.stateMtx.Lock()
	cs.updateToState(stateCopy)
	cs.stateMtx.Unlock()

	fail.Fail() // XXX

	// Private validator might have changed it's key pair => refetch pubkey.
	if err := cs.updatePrivValidatorPubKey(); err != nil {
		cs.Logger.Error("Can't get private validator pubkey", "err", err)
	}

	// publish event
	if types.EnableEventBlockTime {
		cs.blockExec.FireBlockTimeEvents(block.Height, len(block.Txs), false)
	}

	cs.trc.Pin("%s", trace.Waiting)

	// cs.StartTime is already set.
	// Schedule Round0 to start soon.
	cs.scheduleRound0(&cs.RoundState)

	// By here,
	// * cs.Height has been increment to height+1
	// * cs.Step is now cstypes.RoundStepNewHeight
	// * cs.StartTime is set to when we will start round0.
}

// Updates State and increments height to match that of state.
// The round becomes 0 and cs.Step becomes cstypes.RoundStepNewHeight.
func (cs *State) updateToState(state sm.State) {
	// Do not consider this situation that the consensus machine was stopped
	// when the fast-sync mode opens. So remove it!
	//if cs.CommitRound > -1 && 0 < cs.Height && cs.Height != state.LastBlockHeight {
	//	panic(fmt.Sprintf("updateToState() expected state height of %v but found %v",
	//		cs.Height, state.LastBlockHeight))
	//}
	//if !cs.state.IsEmpty() && cs.state.LastBlockHeight+1 != cs.Height {
	//	// This might happen when someone else is mutating cs.state.
	//	// Someone forgot to pass in state.Copy() somewhere?!
	//	panic(fmt.Sprintf("Inconsistent cs.state.LastBlockHeight+1 %v vs cs.Height %v",
	//		cs.state.LastBlockHeight+1, cs.Height))
	//}

	for k, _ := range cs.vcHeight {
		if k <= cs.Height {
			delete(cs.vcHeight, k)
		}
	}
	select {
	case <-cs.taskResultChan:
	default:
	}

	// If state isn't further out than cs.state, just ignore.
	// This happens when SwitchToConsensus() is called in the reactor.
	// We don't want to reset e.g. the Votes, but we still want to
	// signal the new round step, because other services (eg. txNotifier)
	// depend on having an up-to-date peer state!
	if !cs.state.IsEmpty() && (state.LastBlockHeight <= cs.state.LastBlockHeight) {
		cs.Logger.Info(
			"Ignoring updateToState()",
			"newHeight",
			state.LastBlockHeight+1,
			"oldHeight",
			cs.state.LastBlockHeight+1)
		cs.newStep()
		return
	}

	// Reset fields based on state.
	validators := state.Validators
	switch {
	case state.LastBlockHeight == types.GetStartBlockHeight(): // Very first commit should be empty.
		cs.LastCommit = (*types.VoteSet)(nil)
	case cs.CommitRound > -1 && cs.Votes != nil: // Otherwise, use cs.Votes
		if !cs.Votes.Precommits(cs.CommitRound).HasTwoThirdsMajority() {
			panic(fmt.Sprintf(
				"wanted to form a commit, but precommits (H/R: %d/%d) didn't have 2/3+: %v",
				state.LastBlockHeight, cs.CommitRound, cs.Votes.Precommits(cs.CommitRound),
			))
		}

		cs.LastCommit = cs.Votes.Precommits(cs.CommitRound)

	case cs.LastCommit == nil:
		// NOTE: when Tendermint starts, it has no votes. reconstructLastCommit
		// must be called to reconstruct LastCommit from SeenCommit.
		panic(fmt.Sprintf(
			"last commit cannot be empty after initial block (H:%d)",
			state.LastBlockHeight+1,
		))
	}

	// Next desired block height
	height := state.LastBlockHeight + 1

	// RoundState fields
	cs.updateHeight(height)
	cs.updateRoundStep(0, cstypes.RoundStepNewHeight)
	cs.bt.reset(height)

	cs.Validators = validators
	cs.Proposal = nil
	cs.ProposalBlock = nil
	cs.ProposalBlockParts = nil
	cs.LockedRound = -1
	cs.LockedBlock = nil
	cs.LockedBlockParts = nil
	cs.ValidRound = -1
	cs.ValidBlock = nil
	cs.ValidBlockParts = nil
	cs.Votes = cstypes.NewHeightVoteSet(state.ChainID, height, validators)
	cs.CommitRound = -1
	cs.LastValidators = state.LastValidators
	cs.TriggeredTimeoutPrecommit = false
	cs.state = state

	// Finally, broadcast RoundState
	cs.newStep()
}

func (cs *State) updateHeight(height int64) {
	cs.metrics.Height.Set(float64(height))
	cs.Height = height
}

func (cs *State) pruneBlocks(retainHeight int64) (uint64, error) {
	base := cs.blockStore.Base()
	if retainHeight <= base {
		return 0, nil
	}
	pruned, err := cs.blockStore.PruneBlocks(retainHeight)
	if err != nil {
		return 0, fmt.Errorf("failed to prune block store: %w", err)
	}
	err = sm.PruneStates(cs.blockExec.DB(), base, retainHeight)
	if err != nil {
		return 0, fmt.Errorf("failed to prune state database: %w", err)
	}
	return pruned, nil
}

func (cs *State) preMakeBlock(height int64, waiting time.Duration) {
	tNow := tmtime.Now()
	block, blockParts := cs.createProposalBlock()
	cs.taskResultChan <- &preBlockTaskRes{block: block, blockParts: blockParts}

	propBlockID := types.BlockID{Hash: block.Hash(), PartsHeader: blockParts.Header()}
	proposal := types.NewProposal(height, 0, cs.ValidRound, propBlockID)

	if cs.Height != height {
		return
	}
	isBlockProducer, _ := cs.isBlockProducer()
	if GetActiveVC() && isBlockProducer != "y" {
		// request for proposer of new height
		prMsg := ProposeRequestMessage{Height: height, CurrentProposer: cs.Validators.GetProposer().Address, NewProposer: cs.privValidatorPubKey.Address(), Proposal: proposal}
		go func() {
			time.Sleep(waiting - tmtime.Now().Sub(tNow))
			cs.requestForProposer(prMsg)
		}()
	}
}

func (cs *State) getPreBlockResult(height int64) *preBlockTaskRes {
	if !GetActiveVC() {
		return nil
	}
	t := time.NewTimer(time.Second)
	for {
		select {
		case res := <-cs.taskResultChan:
			if res.block.Height == height {
				if !t.Stop() {
					<-t.C
				}
				return res
			} else {
				return nil
			}
		case <-t.C:
			return nil
		}

	}
}

// handle AC offset to avoid block proposal
func (cs *State) handleCommitGapOffset(height int64) {
	commitGap := iavlcfg.DynamicConfig.GetCommitGapHeight()
	offset := cfg.DynamicConfig.GetCommitGapOffset()

	// close offset
	if offset <= 0 || (commitGap <= offset) {
		iavl.SetFinalCommitGapOffset(0)
		// only try to offset at commitGap height
	} else if (height % commitGap) == 0 {
		selfAddress := cs.privValidatorPubKey.Address()
		futureValidators := cs.state.Validators.Copy()

		var i int64
		for ; i < offset; i++ {
			futureBPAddress := futureValidators.GetProposer().Address

			// self is the validator at the offset height
			if bytes.Equal(futureBPAddress, selfAddress) {
				// trigger ac ahead of the offset
				iavl.SetFinalCommitGapOffset(i + 1)
				//originACHeight|newACHeight|nextProposeHeight|Offset
				trace.GetElapsedInfo().AddInfo(trace.ACOffset, fmt.Sprintf("%d|%d|%d|%d|",
					height, height+i+1, height+i, offset))
				break
			}
			futureValidators.IncrementProposerPriority(1)
		}
	}
}
