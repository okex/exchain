package consensus

import (
	"bytes"
	"fmt"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/libs/fail"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
	"reflect"
	"runtime/debug"
	"time"
)

//-----------------------------------------
// the main go routines

// receiveRoutine handles messages which may cause state transitions.
// it's argument (n) is the number of messages to process before exiting - use 0 to run forever
// It keeps the RoundState and is the only thing that updates it.
// Updates (state transitions) happen on timeouts, complete proposals, and 2/3 majorities.
// State must be locked before any internal state is updated.
func (cs *State) receiveRoutine(maxSteps int) {
	onExit := func(cs *State) {
		// NOTE: the internalMsgQueue may have signed messages from our
		// priv_val that haven't hit the WAL, but its ok because
		// priv_val tracks LastSig

		// close wal now that we're done writing to it
		cs.wal.Stop()
		cs.wal.Wait()

		close(cs.done)
		cs.done = nil
	}

	defer func() {
		if r := recover(); r != nil {
			cs.Logger.Error("CONSENSUS FAILURE!!!", "err", r, "stack", string(debug.Stack()))
			// stop gracefully
			//
			// NOTE: We most probably shouldn't be running any further when there is
			// some unexpected panic. Some unknown error happened, and so we don't
			// know if that will result in the validator signing an invalid thing. It
			// might be worthwhile to explore a mechanism for manual resuming via
			// some console or secure RPC system, but for now, halting the chain upon
			// unexpected consensus bugs sounds like the better option.
			onExit(cs)
		}
	}()

	for {
		if maxSteps > 0 {
			if cs.nSteps >= maxSteps {
				cs.Logger.Info("reached max steps. exiting receive routine")
				cs.nSteps = 0
				return
			}
		}
		rs := cs.RoundState
		var mi msgInfo

		select {
		case <-cs.txNotifier.TxsAvailable():
			cs.handleTxsAvailable()
		case mi = <-cs.peerMsgQueue:
			// handles proposals, block parts, votes
			// may generate internal events (votes, complete proposals, 2/3 majorities)
			if cs.handleMsg(mi) {
				cs.wal.Write(mi)
			}
		case mi = <-cs.internalMsgQueue:
			err := cs.wal.WriteSync(mi) // NOTE: fsync
			if err != nil {
				panic(fmt.Sprintf("Failed to write %v msg to consensus wal due to %v. Check your FS and restart the node", mi, err))
			}

			if _, ok := mi.Msg.(*VoteMessage); ok {
				// we actually want to simulate failing during
				// the previous WriteSync, but this isn't easy to do.
				// Equivalent would be to fail here and manually remove
				// some bytes from the end of the wal.
				fail.Fail() // XXX
			}

			// handles proposals, block parts, votes
			cs.handleMsg(mi)
		case ti := <-cs.timeoutTicker.Chan(): // tockChan:
			cs.wal.Write(ti)
			// if the timeout is relevant to the rs
			// go to the next step
			cs.handleTimeout(ti, rs)
		case <-cs.Quit():
			onExit(cs)
			return
		}
	}
}

func (cs *State) handleAVCProposal(proposal *types.Proposal) {
	if !GetActiveVC() ||
		cs.Height != proposal.Height || cs.Round != proposal.Round ||
		len(cs.taskResultChan) == 0 {
		return
	}
	res := cs.getPreBlockResult(proposal.Height)
	if res == nil {
		cs.Logger.Error("handleAVCProposal get block nil", "cs height", cs.Height, "proposal height", proposal.Height)
		return
	}
	if !bytes.Equal(proposal.BlockID.PartsHeader.Hash, res.blockParts.Header().Hash) || proposal.Height != res.block.Height {
		return
	}
	pi := ProposalBlockMessage{proposal, res.block}
	cs.blockCtx.deltaBroker.Pub(pi.Marshal())
	cs.sendInternalMessage(msgInfo{&ProposalMessage{proposal}, ""})
	for i := 0; i < res.blockParts.Total(); i++ {
		part := res.blockParts.GetPart(i)
		cs.sendInternalMessage(msgInfo{&BlockPartMessage{cs.Height, cs.Round, part}, ""})
	}
}

// state transitions on complete-proposal, 2/3-any, 2/3-one
func (cs *State) handleMsg(mi msgInfo) (added bool) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	var (
		err error
	)
	msg, peerID := mi.Msg, mi.PeerID
	switch msg := msg.(type) {
	case *ProposeResponseMessage:
		cs.handleAVCProposal(msg.Proposal)

	case *ViewChangeMessage:
		if !GetActiveVC() {
			return
		}

		// no need to handle duplicate vcMsg
		if cs.vcMsg != nil && cs.vcMsg.Height >= msg.Height {
			return
		}

		// enterNewHeight use cs.vcMsg
		if msg.Height == cs.Height+1 {
			cs.vcMsg = msg
			cs.Logger.Info("handle vcMsg", "height", cs.Height, "vcMsg", cs.vcMsg)
		} else if msg.Height == cs.Height {
			// ApplyBlock of height-1 has finished
			// at this height, it has enterNewHeight
			// vc immediately
			cs.vcMsg = msg
			cs.Logger.Info("handle vcMsg", "height", cs.Height, "vcMsg", cs.vcMsg)
			if cs.Step != cstypes.RoundStepNewHeight && cs.Round == 0 {
				_, val := cs.Validators.GetByAddress(msg.NewProposer)
				cs.enterNewRoundAVC(cs.Height, 0, val)
			}
		}

	case *ProposalMessage:
		// will not cause transition.
		// once proposal is set, we can receive block parts
		if added, err = cs.setProposal(msg.Proposal); added {
			cs.handleAVCProposal(msg.Proposal)
		}
	case *ProposalBlockMessage:
		// not handle this msg if
		// 1.the Height is not equal
		// 2.has received the proposal and the proposal is not equal
		// 3.has received the block
		if cs.Height != msg.Block.Height || cs.Height != msg.Proposal.Height ||
			(cs.Proposal != nil && !bytes.Equal(cs.Proposal.Signature, msg.Proposal.Signature)) ||
			cs.ProposalBlock != nil {
			return
		}
		if cs.Proposal == nil {
			if add, _ := cs.defaultSetProposal(msg.Proposal); !add {
				return
			}
		}
		cs.ProposalBlock = msg.Block
		cs.trc.Pin("recvBlock")
		cs.finishReceiveBlock(msg.Block.Height)
		cs.Logger.Info("GetBlockRedis", "height", msg.Proposal.Height, "time", tmtime.Now())
	case *BlockPartMessage:
		// if avc and has 2/3 votes, it can use the blockPartsHeader from votes
		if cs.HasVC && cs.ProposalBlockParts == nil && cs.Round == 0 {
			prevotes := cs.Votes.Prevotes(cs.Round)
			blockID, hasTwoThirds := prevotes.TwoThirdsMajority()
			if hasTwoThirds && !blockID.IsZero() {
				cs.ProposalBlockParts = types.NewPartSetFromHeader(blockID.PartsHeader)
			}
		}
		// if the proposal is complete, we'll enterPrevote or tryFinalizeCommit
		added, err = cs.addProposalBlockPart(msg, peerID)

		// We unlock here to yield to any routines that need to read the the RoundState.
		// Previously, this code held the lock from the point at which the final block
		// part was received until the block executed against the application.
		// This prevented the reactor from being able to retrieve the most updated
		// version of the RoundState. The reactor needs the updated RoundState to
		// gossip the now completed block.
		//
		// This code can be further improved by either always operating on a copy
		// of RoundState and only locking when switching out State's copy of
		// RoundState with the updated copy or by emitting RoundState events in
		// more places for routines depending on it to listen for.

		if added && cs.ProposalBlockParts.IsComplete() {
			cs.trc.Pin("lastPart")
			cs.bt.onRecvBlock(msg.Height)
			cs.bt.totalParts = cs.ProposalBlockParts.Total()
			cs.Logger.Info("GetBlockP2P", "height", msg.Height, "time", tmtime.Now())

			if cs.ProposalBlock == nil {
				err = cs.unmarshalBlock()
				if err != nil {
					return
				}
				cs.finishReceiveBlock(msg.Height)
			}
		}

		if added {
			cs.statsMsgQueue <- mi
		}

		if err != nil && msg.Round != cs.Round {
			cs.Logger.Debug(
				"Received block part from wrong round",
				"height",
				cs.Height,
				"csRound",
				cs.Round,
				"blockRound",
				msg.Round)
			err = nil
		}
	case *VoteMessage:
		// attempt to add the vote and dupeout the validator if its a duplicate signature
		// if the vote gives us a 2/3-any or 2/3-one, we transition
		added, err = cs.tryAddVote(msg.Vote, peerID)
		if added {
			cs.statsMsgQueue <- mi
		}

		// if err == ErrAddingVote {
		// TODO: punish peer
		// We probably don't want to stop the peer here. The vote does not
		// necessarily comes from a malicious peer but can be just broadcasted by
		// a typical peer.
		// https://github.com/tendermint/tendermint/issues/1281
		// }

		// NOTE: the vote is broadcast to peers by the reactor listening
		// for vote events

		// TODO: If rs.Height == vote.Height && rs.Round < vote.Round,
		// the peer is sending us CatchupCommit precommits.
		// We could make note of this and help filter in broadcastHasVoteMessage().
	default:
		cs.Logger.Error("Unknown msg type", "type", reflect.TypeOf(msg))
		return
	}

	if err != nil { // nolint:staticcheck
		// Causes TestReactorValidatorSetChanges to timeout
		// https://github.com/tendermint/tendermint/issues/3406
		// cs.Logger.Error("Error with msg", "height", cs.Height, "round", cs.Round,
		// 	"peer", peerID, "err", err, "msg", msg)
	}
	return
}

func (cs *State) handleTimeout(ti timeoutInfo, rs cstypes.RoundState) {
	cs.Logger.Debug("Received tock", "timeout", ti.Duration, "height", ti.Height, "round", ti.Round, "step", ti.Step)

	// timeouts must be for current height, round, step
	if ti.Height != rs.Height || ti.Round < rs.Round || (ti.Round == rs.Round && ti.Step < rs.Step) {
		cs.Logger.Debug("Ignoring tock because we're ahead", "height", rs.Height, "round", rs.Round, "step", rs.Step)
		return
	}

	// the timeout will now cause a state transition
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	switch ti.Step {
	case cstypes.RoundStepNewHeight:
		// NewRound event fired from enterNewRound.
		// XXX: should we fire timeout here (for timeout commit)?
		cs.enterNewHeight(ti.Height)
	case cstypes.RoundStepNewRound:
		cs.enterPropose(ti.Height, 0)
	case cstypes.RoundStepPropose:
		cs.eventBus.PublishEventTimeoutPropose(cs.RoundStateEvent())
		cs.enterPrevote(ti.Height, ti.Round)
	case cstypes.RoundStepPrevoteWait:
		cs.eventBus.PublishEventTimeoutWait(cs.RoundStateEvent())
		cs.enterPrecommit(ti.Height, ti.Round)
	case cstypes.RoundStepPrecommitWait:
		cs.eventBus.PublishEventTimeoutWait(cs.RoundStateEvent())
		cs.enterPrecommit(ti.Height, ti.Round)
		cs.enterNewRound(ti.Height, ti.Round+1)
	default:
		panic(fmt.Sprintf("Invalid timeout step: %v", ti.Step))
	}

}

// enterNewRound(height, 0) at cs.StartTime.
func (cs *State) scheduleRound0(rs *cstypes.RoundState) {
	overDuration := tmtime.Now().Sub(cs.StartTime)
	if overDuration < 0 {
		overDuration = 0
	}
	sleepDuration := cfg.DynamicConfig.GetCsTimeoutCommit() - overDuration
	if sleepDuration < 0 {
		sleepDuration = 0
	}

	if !cs.config.Waiting {
		sleepDuration = 0
	}

	if GetActiveVC() && cs.privValidator != nil {
		select {
		case cs.preBlockTaskChan <- &preBlockTask{cs.Height, sleepDuration}:
		default:
		}

	}

	cs.scheduleTimeout(sleepDuration, rs.Height, 0, cstypes.RoundStepNewHeight)
}

//
func (cs *State) finishReceiveBlock(height int64) {
	cs.mtx.Unlock()
	cs.mtx.Lock()

	if cs.prerunTx {
		cs.blockExec.NotifyPrerun(cs.ProposalBlock)
	}
	if !cs.ProposalBlockParts.IsComplete() {
		cs.ProposalBlockParts = cs.ProposalBlock.MakePartSet(types.BlockPartSizeBytes)
	}
	// NOTE: it's possible to receive complete proposal blocks for future rounds without having the proposal
	cs.Logger.Info("Received complete proposal block", "height", cs.ProposalBlock.Height, "hash", cs.ProposalBlock.Hash())
	cs.eventBus.PublishEventCompleteProposal(cs.CompleteProposalEvent())
	cs.handleCompleteProposal(height)
}

// requestForProposer FireEvent to broadcast ProposeRequestMessage
func (cs *State) requestForProposer(prMsg ProposeRequestMessage) {
	if signature, err := cs.privValidator.SignBytes(prMsg.SignBytes()); err == nil {
		prMsg.Signature = signature
		cs.evsw.FireEvent(types.EventProposeRequest, &prMsg)
	} else {
		cs.Logger.Error("requestForProposer", "err", err)
	}
}

// Attempt to schedule a timeout (by sending timeoutInfo on the tickChan)
func (cs *State) scheduleTimeout(duration time.Duration, height int64, round int, step cstypes.RoundStepType) {
	cs.timeoutTicker.ScheduleTimeout(timeoutInfo{Duration: duration, Height: height, Round: round, Step: step})
}

// send a msg into the receiveRoutine regarding our own proposal, block part, or vote
func (cs *State) sendInternalMessage(mi msgInfo) {
	select {
	case cs.internalMsgQueue <- mi:
	default:
		// NOTE: using the go-routine means our votes can
		// be processed out of order.
		// TODO: use CList here for strict determinism and
		// attempt push to internalMsgQueue in receiveRoutine
		cs.Logger.Info("Internal msg queue is full. Using a go-routine")
		go func() { cs.internalMsgQueue <- mi }()
	}
}

func (cs *State) handleTxsAvailable() {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	// We only need to do this for round 0.
	if cs.Round != 0 {
		return
	}

	switch cs.Step {
	case cstypes.RoundStepNewHeight: // timeoutCommit phase
		if cs.needProofBlock(cs.Height) {
			// enterPropose will be called by enterNewRound
			return
		}

		// +1ms to ensure RoundStepNewRound timeout always happens after RoundStepNewHeight
		timeoutCommit := cs.StartTime.Sub(tmtime.Now()) + 1*time.Millisecond
		cs.scheduleTimeout(timeoutCommit, cs.Height, 0, cstypes.RoundStepNewRound)
	case cstypes.RoundStepNewRound: // after timeoutCommit
		cs.enterPropose(cs.Height, 0)
	}
}

func (cs *State) preMakeBlockRoutine() {
	for {
		select {
		case task := <-cs.preBlockTaskChan:
			if task.height == cs.Height {
				cs.preMakeBlock(task.height, task.duration)
			}
		case <-cs.Quit():
			return
		}
	}
}
