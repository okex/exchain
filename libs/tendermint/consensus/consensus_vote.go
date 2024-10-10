package consensus

import (
	"bytes"
	"encoding/hex"
	"fmt"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/p2p"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
	"time"
)

// Attempt to add the vote. if its a duplicate signature, dupeout the validator
func (cs *State) tryAddVote(vote *types.Vote, peerID p2p.ID) (bool, error) {
	added, err := cs.addVote(vote, peerID)
	if err != nil {
		// If the vote height is off, we'll just ignore it,
		// But if it's a conflicting sig, add it to the cs.evpool.
		// If it's otherwise invalid, punish peer.
		// nolint: gocritic
		if err == ErrVoteHeightMismatch {
			return added, err
		} else if voteErr, ok := err.(*types.ErrVoteConflictingVotes); ok {
			if cs.privValidatorPubKey == nil {
				return false, errPubKeyIsNotSet
			}

			if bytes.Equal(vote.ValidatorAddress, cs.privValidatorPubKey.Address()) {
				cs.Logger.Error(
					"Found conflicting vote from ourselves. Did you unsafe_reset a validator?",
					"height",
					vote.Height,
					"round",
					vote.Round,
					"type",
					vote.Type)
				return added, err
			}
			cs.evpool.AddEvidence(voteErr.DuplicateVoteEvidence)
			return added, err
		} else if err == types.ErrVoteNonDeterministicSignature {
			cs.Logger.Debug("Vote has non-deterministic signature", "err", err)
		} else {
			// Either
			// 1) bad peer OR
			// 2) not a bad peer? this can also err sometimes with "Unexpected step" OR
			// 3) tmkms use with multiple validators connecting to a single tmkms instance
			// 		(https://github.com/tendermint/tendermint/issues/3839).
			cs.Logger.Info("Error attempting to add vote", "err", err)
			return added, ErrAddingVote
		}
	}
	return added, nil
}

//-----------------------------------------------------------------------------

func (cs *State) addVote(
	vote *types.Vote,
	peerID p2p.ID) (added bool, err error) {
	cs.Logger.Debug(
		"addVote",
		"voteHeight",
		vote.Height,
		"voteType",
		vote.Type,
		"valIndex",
		vote.ValidatorIndex,
		"csHeight",
		cs.Height,
	)

	// A precommit for the previous height?
	// These come in while we wait timeoutCommit
	if vote.Height+1 == cs.Height {
		if !(cs.Step == cstypes.RoundStepNewHeight && vote.Type == types.PrecommitType) {
			// TODO: give the reason ..
			// fmt.Errorf("tryAddVote: Wrong height, not a LastCommit straggler commit.")
			return added, ErrVoteHeightMismatch
		}
		added, err = cs.LastCommit.AddVote(vote)
		if !added {
			return added, err
		}

		cs.Logger.Info(fmt.Sprintf("Added to lastPrecommits: %v", cs.LastCommit.StringShort()))
		cs.eventBus.PublishEventVote(types.EventDataVote{Vote: vote})
		cs.evsw.FireEvent(types.EventVote, vote)

		// if we can skip timeoutCommit and have all the votes now,
		if cs.config.SkipTimeoutCommit && cs.LastCommit.HasAll() {
			// go straight to new round (skip timeout commit)
			// cs.scheduleTimeout(time.Duration(0), cs.Height, 0, cstypes.RoundStepNewHeight)
			cs.enterNewRound(cs.Height, 0)
		}

		return
	}

	// Height mismatch is ignored.
	// Not necessarily a bad peer, but not favourable behaviour.
	if vote.Height != cs.Height {
		err = ErrVoteHeightMismatch
		cs.Logger.Info("Vote ignored and not added", "voteHeight", vote.Height, "csHeight", cs.Height, "peerID", peerID)
		return
	}

	height := cs.Height
	added, err = cs.Votes.AddVote(vote, peerID)
	if !added {
		// Either duplicate, or error upon cs.Votes.AddByIndex()
		return
	}

	cs.eventBus.PublishEventVote(types.EventDataVote{Vote: vote})
	cs.evsw.FireEvent(types.EventVote, vote)

	switch vote.Type {
	case types.PrevoteType:
		prevotes := cs.Votes.Prevotes(vote.Round)
		cs.Logger.Info("Added to prevote", "vote", vote, "prevotes", prevotes.StringShort())

		// If +2/3 prevotes for a block or nil for *any* round:
		if blockID, ok := prevotes.TwoThirdsMajority(); ok {

			// There was a polka!
			// If we're locked but this is a recent polka, unlock.
			// If it matches our ProposalBlock, update the ValidBlock

			// Unlock if `cs.LockedRound < vote.Round <= cs.Round`
			// NOTE: If vote.Round > cs.Round, we'll deal with it when we get to vote.Round
			if (cs.LockedBlock != nil) &&
				(cs.LockedRound < vote.Round) &&
				(vote.Round <= cs.Round) &&
				!cs.LockedBlock.HashesTo(blockID.Hash) {

				cs.Logger.Info("Unlocking because of POL.", "lockedRound", cs.LockedRound, "POLRound", vote.Round)
				cs.LockedRound = -1
				cs.LockedBlock = nil
				cs.LockedBlockParts = nil
				cs.eventBus.PublishEventUnlock(cs.RoundStateEvent())
			}

			// Update Valid* if we can.
			// NOTE: our proposal block may be nil or not what received a polka..
			if len(blockID.Hash) != 0 && (cs.ValidRound < vote.Round) && (vote.Round == cs.Round) {

				if cs.ProposalBlock.HashesTo(blockID.Hash) {
					cs.Logger.Info(
						"Updating ValidBlock because of POL.", "validRound", cs.ValidRound, "POLRound", vote.Round)
					cs.ValidRound = vote.Round
					cs.ValidBlock = cs.ProposalBlock
					cs.ValidBlockParts = cs.ProposalBlockParts
				} else {
					cs.Logger.Info(
						"Valid block we don't know about. Set ProposalBlock=nil",
						"proposal", cs.ProposalBlock.Hash(), "blockID", blockID.Hash)
					// We're getting the wrong block.
					cs.ProposalBlock = nil
				}
				if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {
					cs.Logger.Info("addVote proposalBlockPart reset ,because of mismatch hash,",
						"origin", hex.EncodeToString(cs.ProposalBlockParts.Hash()), "after", blockID.Hash)
					cs.ProposalBlockParts = types.NewPartSetFromHeader(blockID.PartsHeader)
				}
				cs.evsw.FireEvent(types.EventValidBlock, &cs.RoundState)
				cs.eventBus.PublishEventValidBlock(cs.RoundStateEvent())
			}
		}

		// If +2/3 prevotes for *anything* for future round:
		switch {
		case cs.Round < vote.Round && prevotes.HasTwoThirdsAny():
			// Round-skip if there is any 2/3+ of votes ahead of us
			cs.enterNewRound(height, vote.Round)
		case cs.Round == vote.Round && cstypes.RoundStepPrevote <= cs.Step: // current round
			blockID, ok := prevotes.TwoThirdsMajority()
			if ok && (cs.isProposalComplete() || len(blockID.Hash) == 0) {
				cs.enterPrecommit(height, vote.Round)
			} else if prevotes.HasTwoThirdsAny() {
				cs.enterPrevoteWait(height, vote.Round)
			}
		case cs.Proposal != nil && 0 <= cs.Proposal.POLRound && cs.Proposal.POLRound == vote.Round:
			// If the proposal is now complete, enter prevote of cs.Round.
			if cs.isProposalComplete() {
				cs.enterPrevote(height, cs.Round)
			}
		}

	case types.PrecommitType:
		precommits := cs.Votes.Precommits(vote.Round)
		cs.Logger.Info("Added to precommit", "vote", vote, "precommits", precommits.StringShort())

		blockID, ok := precommits.TwoThirdsMajority()
		if ok {
			// Executed as TwoThirdsMajority could be from a higher round
			cs.enterNewRound(height, vote.Round)
			cs.enterPrecommit(height, vote.Round)
			if len(blockID.Hash) != 0 {
				cs.enterCommit(height, vote.Round)
				if cs.config.SkipTimeoutCommit && precommits.HasAll() {
					cs.enterNewRound(cs.Height, 0)
				}
			} else {
				cs.enterPrecommitWait(height, vote.Round)
			}
		} else if cs.Round <= vote.Round && precommits.HasTwoThirdsAny() {
			cs.enterNewRound(height, vote.Round)
			cs.enterPrecommitWait(height, vote.Round)
		}

	default:
		panic(fmt.Sprintf("Unexpected vote type %X", vote.Type)) // go-amino should prevent this.
	}

	return added, err
}

// CONTRACT: cs.privValidator is not nil.
func (cs *State) signVote(
	msgType types.SignedMsgType,
	hash []byte,
	header types.PartSetHeader,
) (*types.Vote, error) {
	// Flush the WAL. Otherwise, we may not recompute the same vote to sign,
	// and the privValidator will refuse to sign anything.
	cs.wal.FlushAndSync()

	if cs.privValidatorPubKey == nil {
		return nil, errPubKeyIsNotSet
	}
	addr := cs.privValidatorPubKey.Address()
	valIdx, _ := cs.Validators.GetByAddress(addr)

	vote := &types.Vote{
		ValidatorAddress: addr,
		ValidatorIndex:   valIdx,
		Height:           cs.Height,
		Round:            cs.Round,
		Timestamp:        cs.voteTime(),
		Type:             msgType,
		BlockID:          types.BlockID{Hash: hash, PartsHeader: header},
	}

	err := cs.privValidator.SignVote(cs.state.ChainID, vote)
	return vote, err
}

func (cs *State) voteTime() time.Time {
	now := tmtime.Now()
	minVoteTime := now
	// TODO: We should remove next line in case we don't vote for v in case cs.ProposalBlock == nil,
	// even if cs.LockedBlock != nil. See https://docs.tendermint.com/master/spec/.
	timeIotaMs := time.Duration(cs.state.ConsensusParams.Block.TimeIotaMs) * time.Millisecond
	if cs.LockedBlock != nil {
		// See the BFT time spec https://docs.tendermint.com/master/spec/consensus/bft-time.html
		minVoteTime = cs.LockedBlock.Time.Add(timeIotaMs)
	} else if cs.ProposalBlock != nil {
		minVoteTime = cs.ProposalBlock.Time.Add(timeIotaMs)
	}

	if now.After(minVoteTime) {
		return now
	}
	return minVoteTime
}

// sign the vote and publish on internalMsgQueue
func (cs *State) signAddVote(msgType types.SignedMsgType, hash []byte, header types.PartSetHeader) *types.Vote {
	if cs.privValidator == nil { // the node does not have a key
		return nil
	}

	if cs.privValidatorPubKey == nil {
		// Vote won't be signed, but it's not critical.
		cs.Logger.Error(fmt.Sprintf("signAddVote: %v", errPubKeyIsNotSet))
		return nil
	}

	// If the node not in the validator set, do nothing.
	if !cs.Validators.HasAddress(cs.privValidatorPubKey.Address()) {
		return nil
	}

	// TODO: pass pubKey to signVote
	vote, err := cs.signVote(msgType, hash, header)
	if err == nil {
		//broadcast vote immediately
		cs.evsw.FireEvent(types.EventSignVote, vote)
		cs.sendInternalMessage(msgInfo{&VoteMessage{vote}, ""})
		cs.Logger.Info("Signed and pushed vote", "height", cs.Height, "round", cs.Round, "vote", vote, "err", err)
		return vote
	}
	if !cs.replayMode {
		cs.Logger.Error("Error signing vote", "height", cs.Height, "round", cs.Round, "vote", vote, "err", err)
	}
	return nil
}
