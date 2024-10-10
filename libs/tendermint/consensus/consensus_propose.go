package consensus

import (
	"bytes"
	"fmt"
	"strings"

	cfg "github.com/okex/exchain/libs/tendermint/config"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/libs/automation"
	"github.com/okex/exchain/libs/tendermint/p2p"
	"github.com/okex/exchain/libs/tendermint/types"
)

// SetProposal inputs a proposal.
func (cs *State) SetProposal(proposal *types.Proposal, peerID p2p.ID) error {

	if peerID == "" {
		cs.internalMsgQueue <- msgInfo{&ProposalMessage{proposal}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&ProposalMessage{proposal}, peerID}
	}

	// TODO: wait for event?!
	return nil
}

// AddProposalBlockPart inputs a part of the proposal block.
func (cs *State) AddProposalBlockPart(height int64, round int, part *types.Part, peerID p2p.ID) error {
	if peerID == "" {
		cs.internalMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, peerID}
	}

	// TODO: wait for event?!
	return nil
}

// SetProposalAndBlock inputs the proposal and all block parts.
func (cs *State) SetProposalAndBlock(
	proposal *types.Proposal,
	block *types.Block,
	parts *types.PartSet,
	peerID p2p.ID,
) error {
	if err := cs.SetProposal(proposal, peerID); err != nil {
		return err
	}
	for i := 0; i < parts.Total(); i++ {
		part := parts.GetPart(i)
		if err := cs.AddProposalBlockPart(proposal.Height, proposal.Round, part, peerID); err != nil {
			return err
		}
	}
	return nil
}

func (cs *State) isBlockProducer() (string, string) {
	const len2display int = 6
	bpAddr := cs.Validators.GetProposer().Address
	bpStr := bpAddr.String()
	if len(bpStr) > len2display {
		bpStr = bpStr[:len2display]
	}
	isBlockProducer := "n"
	if cs.privValidator != nil && cs.privValidatorPubKey != nil {
		address := cs.privValidatorPubKey.Address()

		if bytes.Equal(bpAddr, address) {
			isBlockProducer = "y"
		}
	}

	return isBlockProducer, strings.ToLower(bpStr)
}

// Enter (CreateEmptyBlocks): from enterNewRound(height,round)
// Enter (CreateEmptyBlocks, CreateEmptyBlocksInterval > 0 ):
// 		after enterNewRound(height,round), after timeout of CreateEmptyBlocksInterval
// Enter (!CreateEmptyBlocks) : after enterNewRound(height,round), once txs are in the mempool
func (cs *State) enterPropose(height int64, round int) {
	logger := cs.Logger.With("height", height, "round", round)
	if cs.Height != height || round < cs.Round || (cs.Round == round && cstypes.RoundStepPropose <= cs.Step) {
		logger.Debug(fmt.Sprintf(
			"enterPropose(%v/%v): Invalid args. Current step: %v/%v/%v",
			height,
			round,
			cs.Height,
			cs.Round,
			cs.Step))
		return
	}

	cs.initNewHeight()
	isBlockProducer, bpAddr := cs.isBlockProducer()

	cs.stateMtx.RLock()
	cs.updateRoundStep(round, cstypes.RoundStepPropose)
	newProposer := ""
	if cs.vcHeight[height] != "" && cs.Round == 0 {
		newProposer = "-avc-" + cs.vcHeight[height][:6]
	}
	cs.stateMtx.RUnlock()
	cs.trc.Pin("enterPropose-%d-%s-%s%s", round, isBlockProducer, bpAddr, newProposer)

	logger.Info(fmt.Sprintf("enterPropose(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterPropose:
		cs.newStep()

		// If we have the whole proposal + POL, then goto Prevote now.
		// else, we'll enterPrevote when the rest of the proposal is received (in AddProposalBlockPart),
		// or else after timeoutPropose
		if cs.isProposalComplete() {
			cs.enterPrevote(height, cs.Round)
		}
	}()

	// If we don't get the proposal and all block parts quick enough, enterPrevote
	cs.timeoutTicker.ScheduleTimeout(timeoutInfo{Duration: cs.config.Propose(round), Height: height, Round: round, Step: cstypes.RoundStepPropose, ActiveViewChange: cs.HasVC})

	if isBlockProducer == "y" {
		logger.Info("enterPropose: Our turn to propose",
			"proposer",
			bpAddr,
			"privValidator",
			cs.privValidator)
		if newProposer == "" || cs.Round != 0 {
			cs.decideProposal(height, round)
		}
	} else {
		logger.Info("enterPropose: Not our turn to propose",
			"proposer",
			cs.Validators.GetProposer().Address,
			"privValidator",
			cs.privValidator)
	}
}

func (cs *State) isProposer(address []byte) bool {
	return bytes.Equal(cs.Validators.GetProposer().Address, address)
}

func (cs *State) defaultDecideProposal(height int64, round int) {
	var block *types.Block
	var blockParts *types.PartSet

	// Decide on block
	if cs.ValidBlock != nil {
		// If there is valid block, choose that.
		block, blockParts = cs.ValidBlock, cs.ValidBlockParts
	} else {
		// Create a new proposal block from state/txs from the mempool.
		if res := cs.getPreBlockResult(height); res != nil {
			block, blockParts = res.block, res.blockParts
		} else {
			block, blockParts = cs.createProposalBlock()
		}
		if block == nil {
			return
		}
	}

	// Flush the WAL. Otherwise, we may not recompute the same proposal to sign,
	// and the privValidator will refuse to sign anything.
	cs.wal.FlushAndSync()

	// Make proposal
	propBlockID := types.BlockID{Hash: block.Hash(), PartsHeader: blockParts.Header()}
	proposal := types.NewProposal(height, round, cs.ValidRound, propBlockID)
	proposal.HasVC = cs.HasVC
	if err := cs.privValidator.SignProposal(cs.state.ChainID, proposal); err == nil {
		pi := ProposalBlockMessage{proposal, block}
		cs.blockCtx.deltaBroker.Pub(pi.Marshal())

		// send proposal and block parts on internal msg queue
		cs.sendInternalMessage(msgInfo{&ProposalMessage{proposal}, ""})
		for i := 0; i < blockParts.Total(); i++ {
			part := blockParts.GetPart(i)
			cs.sendInternalMessage(msgInfo{&BlockPartMessage{cs.Height, cs.Round, part}, ""})
		}
		cs.Logger.Info("Signed proposal", "height", height, "round", round, "proposal", proposal)
		cs.Logger.Debug(fmt.Sprintf("Signed proposal block: %v", block))
	} else if !cs.replayMode {
		cs.Logger.Error("enterPropose: Error signing proposal", "height", height, "round", round, "err", err)
	}
}

// Returns true if the proposal block is complete &&
// (if POLRound was proposed, we have +2/3 prevotes from there).
func (cs *State) isProposalComplete() bool {
	if cs.Proposal == nil || cs.ProposalBlock == nil {
		return false
	}
	// we have the proposal. if there's a POLRound,
	// make sure we have the prevotes from it too
	if cs.Proposal.POLRound < 0 {
		return true
	}
	// if this is false the proposer is lying or we haven't received the POL yet
	return cs.Votes.Prevotes(cs.Proposal.POLRound).HasTwoThirdsMajority()

}

// Create the next block to propose and return it. Returns nil block upon error.
//
// We really only need to return the parts, but the block is returned for
// convenience so we can log the proposal block.
//
// NOTE: keep it side-effect free for clarity.
// CONTRACT: cs.privValidator is not nil.
func (cs *State) createProposalBlock() (block *types.Block, blockParts *types.PartSet) {
	if cs.privValidator == nil {
		panic("entered createProposalBlock with privValidator being nil")
	}

	var commit *types.Commit
	switch {
	case cs.Height == types.GetStartBlockHeight()+1:
		// We're creating a proposal for the first block.
		// The commit is empty, but not nil.
		commit = types.NewCommit(0, 0, types.BlockID{}, nil)
	case cs.LastCommit.HasTwoThirdsMajority():
		// Make the commit from LastCommit
		commit = cs.LastCommit.MakeCommit()
	default: // This shouldn't happen.
		cs.Logger.Error("enterPropose: Cannot propose anything: No commit for the previous block")
		return
	}

	if cs.privValidatorPubKey == nil {
		// If this node is a validator & proposer in the current round, it will
		// miss the opportunity to create a block.
		cs.Logger.Error(fmt.Sprintf("enterPropose: %v", errPubKeyIsNotSet))
		return
	}
	proposerAddr := cs.privValidatorPubKey.Address()

	return cs.blockExec.CreateProposalBlock(cs.Height, cs.state, commit, proposerAddr)
}

//-----------------------------------------------------------------------------

func (cs *State) defaultSetProposal(proposal *types.Proposal) (bool, error) {
	// Already have one
	// TODO: possibly catch double proposals
	if cs.Proposal != nil {
		return false, nil
	}

	// Does not apply
	if proposal.Height != cs.Height || proposal.Round != cs.Round {
		return false, nil
	}

	// Verify POLRound, which must be -1 or in range [0, proposal.Round).
	if proposal.POLRound < -1 ||
		(proposal.POLRound >= 0 && proposal.POLRound >= proposal.Round) {
		return false, ErrInvalidProposalPOLRound
	}

	// Verify signature
	if !cs.Validators.GetProposer().PubKey.VerifyBytes(proposal.SignBytes(cs.state.ChainID), proposal.Signature) {
		return false, ErrInvalidProposalSignature
	}

	cs.Proposal = proposal
	// We don't update cs.ProposalBlockParts if it is already set.
	// This happens if we're already in cstypes.RoundStepCommit or if there is a valid block in the current round.
	// TODO: We can check if Proposal is for a different block as this is a sign of misbehavior!
	if cs.ProposalBlockParts == nil {
		cs.ProposalBlockParts = types.NewPartSetFromHeader(proposal.BlockID.PartsHeader)
	}
	cs.Logger.Info("Received proposal", "proposal", proposal)
	cs.bt.onProposal(proposal.Height)
	cs.trc.Pin("recvProposal")
	return true, nil
}

func (cs *State) unmarshalBlock() error {
	// uncompress blockParts bytes if necessary
	pbpReader, err := types.UncompressBlockFromReader(cs.ProposalBlockParts.GetReader())
	if err != nil {
		return err
	}

	// Added and completed!
	_, err = cdc.UnmarshalBinaryLengthPrefixedReader(
		pbpReader,
		&cs.ProposalBlock,
		cs.state.ConsensusParams.Block.MaxBytes,
	)
	return err
}
func (cs *State) onBlockPartAdded(height int64, round, index int, added bool, err error) {

	if err != nil {
		cs.bt.droppedDue2Error++
	}

	if added {
		if cs.ProposalBlockParts.Count() == 1 {
			cs.trc.Pin("1stPart")
			cs.bt.on1stPart(height)
		}
		// event to decrease blockpart transport
		if cfg.DynamicConfig.GetEnableHasBlockPartMsg() {
			cs.evsw.FireEvent(types.EventBlockPart, &HasBlockPartMessage{height, round, index})
		}
	} else {
		cs.bt.droppedDue2NotAdded++
	}

}

func (cs *State) addBlockPart(height int64, round int, part *types.Part, peerID p2p.ID) (added bool, err error) {
	// Blocks might be reused, so round mismatch is OK
	if cs.Height != height {
		cs.bt.droppedDue2WrongHeight++
		cs.Logger.Debug("Received block part from wrong height", "height", height, "round", round)
		return
	}
	// We're not expecting a block part.
	if cs.ProposalBlockParts == nil {
		// NOTE: this can happen when we've gone to a higher round and
		// then receive parts from the previous round - not necessarily a bad peer.
		cs.Logger.Info("Received a block part when we're not expecting any",
			"height", height, "round", round, "index", part.Index, "peer", peerID)
		cs.bt.droppedDue2NotExpected++
		return
	}
	added, err = cs.ProposalBlockParts.AddPart(part)
	cs.onBlockPartAdded(height, round, part.Index, added, err)
	return
}

// NOTE: block is not necessarily valid.
// Asynchronously triggers either enterPrevote (before we timeout of propose) or tryFinalizeCommit,
// once we have the full block.
func (cs *State) addProposalBlockPart(msg *BlockPartMessage, peerID p2p.ID) (added bool, err error) {
	height, round, part := msg.Height, msg.Round, msg.Part
	if automation.BlockIsNotCompleted(height, round) {
		return
	}
	automation.AddBlockTimeOut(height, round)
	return cs.addBlockPart(height, round, part, peerID)
}

func (cs *State) handleCompleteProposal(height int64) {
	cs.Logger.Info("handleCompleteProposal", "height", cs.Height, "round", cs.Round, "step", cs.Step)
	// Update Valid* if we can.
	prevotes := cs.Votes.Prevotes(cs.Round)
	blockID, hasTwoThirds := prevotes.TwoThirdsMajority()
	prevoteBlockValid := hasTwoThirds && !blockID.IsZero() && (cs.ValidRound < cs.Round) && cs.ProposalBlock.HashesTo(blockID.Hash)
	if prevoteBlockValid {
		cs.Logger.Info("Updating valid block to new proposal block",
			"valid_round", cs.Round, "valid_block_hash", cs.ProposalBlock.Hash())
		cs.ValidRound = cs.Round
		cs.ValidBlock = cs.ProposalBlock
		cs.ValidBlockParts = cs.ProposalBlockParts
		// TODO: In case there is +2/3 majority in Prevotes set for some
		// if !cs.ProposalBlock.HashesTo(blockID.Hash) {}
		// block and cs.ProposalBlock contains different block, either
		// proposer is faulty or voting power of faulty processes is more
		// than 1/3. We should trigger in the future accountability
		// procedure at this point.
	}

	if cs.Step <= cstypes.RoundStepPropose && cs.isProposalComplete() {
		// Move onto the next step
		cs.enterPrevote(height, cs.Round)
		if hasTwoThirds { // this is optimisation as this will be triggered when prevote is added
			cs.enterPrecommit(height, cs.Round)
		}
	}
	if cs.HasVC && cs.Round == 0 {
		blockID, hasTwoThirds := cs.Votes.Precommits(cs.Round).TwoThirdsMajority()
		cs.Logger.Info("avc and handleCompleteProposal", "2/3Precommit", hasTwoThirds, "proposal", cs.ProposalBlock.Hash(), "block", blockID.Hash)
		if hasTwoThirds && !blockID.IsZero() && cs.ProposalBlock.HashesTo(blockID.Hash) {
			cs.updateRoundStep(cs.Round, cstypes.RoundStepCommit)
		}
	}
	if cs.Step == cstypes.RoundStepCommit {
		// If we're waiting on the proposal block...
		cs.tryFinalizeCommit(height)
	}
}
