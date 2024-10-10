package consensus

import (
	"fmt"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
)

//-----------------------------------------------------------------------------
// State functions
// Used internally by handleTimeout and handleMsg to make state transitions

// Enter: `timeoutNewHeight` by startTime (R0PrevoteTime+timeoutCommit),
//
//	or, if SkipTimeoutCommit==true, after receiving all precommits from (height,round-1)
//
// Enter: `timeoutPrecommits` after any +2/3 precommits from (height,round-1)
// Enter: +2/3 precommits for nil at (height,round-1)
// Enter: +2/3 prevotes any or +2/3 precommits for block or any from (height, round)
// NOTE: cs.StartTime was already set for height.
func (cs *State) enterNewRound(height int64, round int) {
	logger := cs.Logger.With("height", height, "round", round)
	if cs.Height != height || round < cs.Round || (cs.Round == round && cs.Step != cstypes.RoundStepNewHeight) {
		logger.Debug(fmt.Sprintf(
			"enterNewRound(%v/%v): Invalid args. Current step: %v/%v/%v",
			height,
			round,
			cs.Height,
			cs.Round,
			cs.Step))
		return
	}

	cs.doNewRound(height, round)
}

func (cs *State) doNewRound(height int64, round int) {
	logger := cs.Logger.With("height", height, "round", round)
	cs.initNewHeight()
	if now := tmtime.Now(); cs.StartTime.After(now) {
		logger.Info("Need to set a buffer and log message here for sanity.", "startTime", cs.StartTime, "now", now)
	}
	logger.Info(fmt.Sprintf("enterNewRound(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	// Increment validators if necessary
	validators := cs.Validators
	if cs.Round < round {
		validators = validators.Copy()
		validators.IncrementProposerPriority(round - cs.Round)
	}
	cs.Validators = validators
	cs.Votes.SetRound(round + 1) // also track next round (round+1) to allow round-skipping

	// Setup new round
	// we don't fire newStep for this step,
	// but we fire an event, so update the round step first
	cs.updateRoundStep(round, cstypes.RoundStepNewRound)
	if round == 0 {
		// We've already reset these upon new height,
		// and meanwhile we might have received a proposal
		// for round 0.
	} else {
		logger.Info("Resetting Proposal info")
		cs.Proposal = nil
		cs.ProposalBlock = nil
		cs.ProposalBlockParts = nil
	}

	cs.TriggeredTimeoutPrecommit = false
	cs.eventBus.PublishEventNewRound(cs.NewRoundEvent())
	cs.metrics.Rounds.Set(float64(round))

	// Wait for txs to be available in the mempool
	// before we enterPropose in round 0. If the last block changed the app hash,
	// we may need an empty "proof" block, and enterPropose immediately.
	waitForTxs := cs.config.WaitForTxs() && round == 0 && !cs.needProofBlock(height)
	if waitForTxs {
		if cs.config.CreateEmptyBlocksInterval > 0 {
			cs.scheduleTimeout(cs.config.CreateEmptyBlocksInterval, height, round,
				cstypes.RoundStepNewRound)
		}
	} else {
		cs.enterPropose(height, round)
	}
}

// Enter: `timeoutNewHeight` by startTime (after timeoutCommit),
func (cs *State) enterNewHeight(height int64) {
	cs.enterNewRound(height, 0)
}
