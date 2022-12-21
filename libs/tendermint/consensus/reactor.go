package consensus

import (
	"bytes"
	"fmt"
	"github.com/okex/exchain/libs/tendermint/config"
	"github.com/okex/exchain/libs/tendermint/crypto"
	"github.com/okex/exchain/libs/tendermint/libs/automation"
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"

	amino "github.com/tendermint/go-amino"

	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/libs/bits"
	tmevents "github.com/okex/exchain/libs/tendermint/libs/events"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	"github.com/okex/exchain/libs/tendermint/p2p"
	sm "github.com/okex/exchain/libs/tendermint/state"
	"github.com/okex/exchain/libs/tendermint/types"
	tmtime "github.com/okex/exchain/libs/tendermint/types/time"
)

type bpType int

const (
	StateChannel       = byte(0x20)
	DataChannel        = byte(0x21)
	VoteChannel        = byte(0x22)
	VoteSetBitsChannel = byte(0x23)
	ViewChangeChannel  = byte(0x24)

	maxPartSize = 1048576 // 1MB; NOTE/TODO: keep in sync with types.PartSet sizes.
	maxMsgSize  = maxPartSize

	blocksToContributeToBecomeGoodPeer = 10000
	votesToContributeToBecomeGoodPeer  = 10000

	BP_SEND bpType = iota
	BP_RECV
	BP_ACK
	BP_CATCHUP
)

//-----------------------------------------------------------------------------

type blockchainReactor interface {
	// CheckFastSyncCondition called when we're hanging in a height for some time during consensus
	CheckFastSyncCondition()
}

// Reactor defines a reactor for the consensus service.
type Reactor struct {
	p2p.BaseReactor // BaseService + p2p.Switch

	conS *State

	mtx                   sync.RWMutex
	fastSync              bool
	autoFastSync          bool
	eventBus              *types.EventBus
	switchToFastSyncTimer *time.Timer
	conHeight             int64

	metrics *Metrics

	rs *cstypes.RoundState

	hasViewChanged int64
	avcWhiteList   map[string]bool
}

type ReactorOption func(*Reactor)

// NewReactor returns a new Reactor with the given
// consensusState.
func NewReactor(consensusState *State, fastSync bool, autoFastSync bool, options ...ReactorOption) *Reactor {
	conR := &Reactor{
		conS:                  consensusState,
		fastSync:              fastSync,
		autoFastSync:          autoFastSync,
		switchToFastSyncTimer: time.NewTimer(0),
		conHeight:             consensusState.Height,
		rs:                    consensusState.GetRoundState(),
		metrics:               NopMetrics(),
		avcWhiteList:          map[string]bool{},
	}
	conR.updateFastSyncingMetric()
	conR.BaseReactor = *p2p.NewBaseReactor("Consensus", conR)

	conR.stopSwitchToFastSyncTimer()

	for _, option := range options {
		option(conR)
	}

	for _, nodeKey := range config.DynamicConfig.GetAVCWhiteList() {
		conR.avcWhiteList[nodeKey] = true
	}

	return conR
}

// OnStart implements BaseService by subscribing to events, which later will be
// broadcasted to other peers and starting state if we're not in fast sync.
func (conR *Reactor) OnStart() error {
	conR.Logger.Info("Reactor ", "fastSync", conR.FastSync())

	// start routine that computes peer statistics for evaluating peer quality
	go conR.peerStatsRoutine()

	conR.subscribeToBroadcastEvents()

	if !conR.FastSync() {
		err := conR.conS.Start()
		if err != nil {
			return err
		}
	}

	go conR.updateRoundStateRoutine()

	return nil
}

func (conR *Reactor) updateRoundStateRoutine() {
	t := time.NewTicker(100 * time.Microsecond)
	defer t.Stop()

	for _ = range t.C {
		rs := conR.conS.GetRoundState()
		conR.mtx.Lock()
		conR.rs = rs
		conR.mtx.Unlock()
	}

}

func (conR *Reactor) getRoundState() *cstypes.RoundState {
	conR.mtx.RLock()
	defer conR.mtx.RUnlock()
	return conR.rs
}

// OnStop implements BaseService by unsubscribing from events and stopping
// state.
func (conR *Reactor) OnStop() {
	conR.unsubscribeFromBroadcastEvents()
	conR.conS.Stop()
	if !conR.FastSync() {
		conR.conS.Wait()
	}
	conR.stopSwitchToFastSyncTimer()
}

// SwitchToConsensus switches from fast_sync mode to consensus mode.
// It resets the state, turns off fast_sync, and starts the consensus state-machine
func (conR *Reactor) SwitchToConsensus(state sm.State, blocksSynced uint64) bool {
	if conR.conS.IsRunning() {
		return false
	}

	defer func() {
		conR.setFastSyncFlag(false, 0)
	}()

	conR.Logger.Info("SwitchToConsensus")
	if state.LastBlockHeight > types.GetStartBlockHeight() {
		conR.conS.reconstructLastCommit(state)
	}
	// NOTE: The line below causes broadcastNewRoundStepRoutine() to
	// broadcast a NewRoundStepMessage.
	conR.conS.updateToState(state)

	if blocksSynced > 0 {
		// dont bother with the WAL if we fast synced
		conR.conS.doWALCatchup = false
	}
	conR.conS.Stop()
	//if !conR.FastSync() {
	//	conR.conS.Wait()
	//}
	conR.conS.Reset()
	conR.conS.Start()

	go conR.peerStatsRoutine()
	conR.subscribeToBroadcastEvents()

	return true
}

func (conR *Reactor) SwitchToFastSync() (sm.State, error) {
	conR.Logger.Info("SwitchToFastSync")

	defer func() {
		conR.setFastSyncFlag(true, 1)
	}()

	if !conR.conS.IsRunning() {
		return conR.conS.GetState(), errors.New("state is not running")
	}

	err := conR.conS.Stop()
	if err != nil {
		panic(fmt.Sprintf(`Failed to stop consensus state: %v

conS:
%+v

conR:
%+v`, err, conR.conS, conR))
	}

	conR.stopSwitchToFastSyncTimer()
	conR.conS.Wait()

	cState := conR.conS.GetState()
	return cState, nil
}

func (conR *Reactor) setFastSyncFlag(f bool, v float64) {
	conR.mtx.Lock()
	defer conR.mtx.Unlock()

	conR.fastSync = f
	conR.metrics.FastSyncing.Set(v)
	conR.conS.blockExec.SetIsFastSyncing(f)
}

// Attempt to schedule a timer for checking whether consensus machine is hanged.
func (conR *Reactor) resetSwitchToFastSyncTimer() {
	if conR.autoFastSync {
		conR.Logger.Info("Reset SwitchToFastSyncTimeout.")
		conR.stopSwitchToFastSyncTimer()
		conR.switchToFastSyncTimer.Reset(conR.conS.config.TimeoutToFastSync)
	}
}

func (conR *Reactor) stopSwitchToFastSyncTimer() {
	if !conR.switchToFastSyncTimer.Stop() { // Stop() returns false if it was already fired or was stopped
		select {
		case <-conR.switchToFastSyncTimer.C:
		default:
			conR.Logger.Debug("Timer already stopped")
		}
	}
}

// GetChannels implements Reactor
func (conR *Reactor) GetChannels() []*p2p.ChannelDescriptor {
	// TODO optimize
	return []*p2p.ChannelDescriptor{
		{
			ID:                  StateChannel,
			Priority:            5,
			SendQueueCapacity:   100,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID: DataChannel, // maybe split between gossiping current block and catchup stuff
			// once we gossip the whole block there's nothing left to send until next height or round
			Priority:            10,
			SendQueueCapacity:   100,
			RecvBufferCapacity:  50 * 4096,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  VoteChannel,
			Priority:            5,
			SendQueueCapacity:   100,
			RecvBufferCapacity:  100 * 100,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  VoteSetBitsChannel,
			Priority:            1,
			SendQueueCapacity:   2,
			RecvBufferCapacity:  1024,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  ViewChangeChannel,
			Priority:            5,
			SendQueueCapacity:   100,
			RecvMessageCapacity: maxMsgSize,
		},
	}
}

// InitPeer implements Reactor by creating a state for the peer.
func (conR *Reactor) InitPeer(peer p2p.Peer) p2p.Peer {
	peerState := NewPeerState(peer).SetLogger(conR.Logger)
	peer.Set(types.PeerStateKey, peerState)
	return peer
}

// AddPeer implements Reactor by spawning multiple gossiping goroutines for the
// peer.
func (conR *Reactor) AddPeer(peer p2p.Peer) {
	if !conR.IsRunning() {
		return
	}

	peerState, ok := peer.Get(types.PeerStateKey).(*PeerState)
	if !ok {
		panic(fmt.Sprintf("peer %v has no state", peer))
	}
	// Begin routines for this peer.
	go conR.gossipDataRoutine(peer, peerState)
	go conR.gossipVotesRoutine(peer, peerState)
	//	go conR.gossipVCRoutine(peer, peerState)
	go conR.queryMaj23Routine(peer, peerState)

	// Send our state to peer.
	// If we're fast_syncing, broadcast a RoundStepMessage later upon SwitchToConsensus().
	if !conR.FastSync() {
		conR.sendNewRoundStepMessage(peer)
	}
}

// RemovePeer is a noop.
func (conR *Reactor) RemovePeer(peer p2p.Peer, reason interface{}) {
	if !conR.IsRunning() {
		return
	}
	// TODO
	// ps, ok := peer.Get(PeerStateKey).(*PeerState)
	// if !ok {
	// 	panic(fmt.Sprintf("Peer %v has no state", peer))
	// }
	// ps.Disconnect()
}

// Receive implements Reactor
// NOTE: We process these messages even when we're fast_syncing.
// Messages affect either a peer state or the consensus state.
// Peer state updates can happen in parallel, but processing of
// proposals, block parts, and votes are ordered by the receiveRoutine
// NOTE: blocks on consensus state for proposals, block parts, and votes
func (conR *Reactor) Receive(chID byte, src p2p.Peer, msgBytes []byte) {
	if automation.NetworkDisconnect(conR.conS.Height, conR.conS.Round) {
		return
	}

	if !conR.IsRunning() {
		conR.Logger.Debug("Receive", "src", src, "chId", chID, "bytes", msgBytes)
		return
	}

	msg, err := decodeMsg(msgBytes)
	if err != nil {
		conR.Logger.Error("Error decoding message", "src", src, "chId", chID, "msg", msg, "err", err, "bytes", msgBytes)
		conR.Switch.StopPeerForError(src, err)
		return
	}

	if err = msg.ValidateBasic(); err != nil {
		conR.Logger.Error("Peer sent us invalid msg", "peer", src, "msg", msg, "err", err)
		conR.Switch.StopPeerForError(src, err)
		return
	}

	conR.Logger.Debug("Receive", "src", src, "chId", chID, "msg", msg)

	// Get peer states
	ps, ok := src.Get(types.PeerStateKey).(*PeerState)
	if !ok {
		panic(fmt.Sprintf("Peer %v has no state", src))
	}

	switch chID {
	case ViewChangeChannel:
		if !GetActiveVC() {
			return
		}
		switch msg := msg.(type) {
		case *ViewChangeMessage:
			// verify the signature of vcMsg
			_, val := conR.conS.Validators.GetByAddress(msg.CurrentProposer)
			if err := msg.Verify(val.PubKey); err != nil {
				conR.Logger.Error("reactor Verify Signature of ViewChangeMessage", "err", err)
				return
			}
			conR.conS.peerMsgQueue <- msgInfo{msg, ""}
		case *ProposeResponseMessage:
			conR.conS.peerMsgQueue <- msgInfo{msg, ""}
		case *ProposeRequestMessage:
			if !conR.avcWhiteList[string(src.ID())] {
				return
			}
			conR.conS.stateMtx.Lock()
			defer conR.conS.stateMtx.Unlock()
			height := conR.conS.Height
			// this peer has received a prMsg before
			// or this peer is not proposer
			// or only then proposer ApplyBlock(height) has finished, do not handle prMsg
			// or prMsg.height != prMsg.proposal.Height
			if msg.Height <= conR.hasViewChanged ||
				!bytes.Equal(conR.conS.privValidatorPubKey.Address(), msg.CurrentProposer) ||
				msg.Height <= height ||
				msg.Height != msg.Proposal.Height {
				return
			}

			// verify the signature of prMsg
			_, val := conR.conS.Validators.GetByAddress(msg.NewProposer)
			if val == nil {
				return
			}
			if err := msg.Verify(val.PubKey); err != nil {
				conR.Logger.Error("reactor Verify Signature of ProposeRequestMessage", "err", err)
				return
			}

			// sign proposal
			proposal := msg.Proposal
			signBytes := proposal.SignBytes(conR.conS.state.ChainID)
			sig, err := conR.conS.privValidator.SignBytes(signBytes)
			if err != nil {
				return
			}
			proposal.Signature = sig
			// tell newProposer
			prspMsg := &ProposeResponseMessage{Height: proposal.Height, Proposal: proposal}
			ps.peer.Send(ViewChangeChannel, cdc.MustMarshalBinaryBare(prspMsg))
			// broadcast the proposal
			conR.Switch.Broadcast(DataChannel, cdc.MustMarshalBinaryBare(&ProposalMessage{Proposal: proposal}))

			conR.hasViewChanged = msg.Height

			// mark the height no need to be proposer in msg.Height
			conR.conS.vcHeight[msg.Height] = msg.NewProposer.String()
			conR.Logger.Info("receive prMsg", "height", height, "prMsg", msg, "vcMsg", conR.conS.vcMsg)
		}
	case StateChannel:
		switch msg := msg.(type) {
		case *NewRoundStepMessage:
			ps.ApplyNewRoundStepMessage(msg)
		case *NewValidBlockMessage:
			ps.ApplyNewValidBlockMessage(msg)
		case *HasBlockPartMessage:
			ps.SetHasProposalBlockPart(msg.Height, msg.Round, msg.Index, BP_ACK, conR.conS.bt)
		case *HasVoteMessage:
			ps.ApplyHasVoteMessage(msg)
		case *VoteSetMaj23Message:
			cs := conR.conS
			cs.stateMtx.RLock()
			height, votes := cs.Height, cs.Votes
			cs.stateMtx.RUnlock()
			if height != msg.Height {
				return
			}
			// Peer claims to have a maj23 for some BlockID at H,R,S,
			err := votes.SetPeerMaj23(msg.Round, msg.Type, ps.peer.ID(), msg.BlockID)
			if err != nil {
				conR.Switch.StopPeerForError(src, err)
				return
			}
			// Respond with a VoteSetBitsMessage showing which votes we have.
			// (and consequently shows which we don't have)
			var ourVotes *bits.BitArray
			switch msg.Type {
			case types.PrevoteType:
				ourVotes = votes.Prevotes(msg.Round).BitArrayByBlockID(msg.BlockID)
			case types.PrecommitType:
				ourVotes = votes.Precommits(msg.Round).BitArrayByBlockID(msg.BlockID)
			default:
				panic("Bad VoteSetBitsMessage field Type. Forgot to add a check in ValidateBasic?")
			}
			src.TrySend(VoteSetBitsChannel, cdc.MustMarshalBinaryBare(&VoteSetBitsMessage{
				Height:  msg.Height,
				Round:   msg.Round,
				Type:    msg.Type,
				BlockID: msg.BlockID,
				Votes:   ourVotes,
			}))
		default:
			conR.Logger.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	case DataChannel:
		if conR.FastSync() {
			conR.Logger.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *ProposalMessage:
			ps.SetHasProposal(msg.Proposal)
			conR.conS.peerMsgQueue <- msgInfo{msg, src.ID()}
		case *ProposalPOLMessage:
			ps.ApplyProposalPOLMessage(msg)
		case *BlockPartMessage:
			ps.SetHasProposalBlockPart(msg.Height, msg.Round, msg.Part.Index, BP_RECV, conR.conS.bt)
			conR.metrics.BlockParts.With("peer_id", string(src.ID())).Add(1)
			conR.conS.peerMsgQueue <- msgInfo{msg, src.ID()}
		default:
			conR.Logger.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	case VoteChannel:
		if conR.FastSync() {
			conR.Logger.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *VoteMessage:
			cs := conR.conS
			cs.stateMtx.RLock()
			height, valSize, lastCommitSize := cs.Height, cs.Validators.Size(), cs.LastCommit.Size()
			cs.stateMtx.RUnlock()
			ps.EnsureVoteBitArrays(height, valSize)
			ps.EnsureVoteBitArrays(height-1, lastCommitSize)
			ps.SetHasVote(msg.Vote)

			cs.peerMsgQueue <- msgInfo{msg, src.ID()}

		default:
			// don't punish (leave room for soft upgrades)
			conR.Logger.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	case VoteSetBitsChannel:
		if conR.FastSync() {
			conR.Logger.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *VoteSetBitsMessage:
			cs := conR.conS
			cs.stateMtx.RLock()
			height, votes := cs.Height, cs.Votes
			cs.stateMtx.RUnlock()

			if height == msg.Height {
				var ourVotes *bits.BitArray
				switch msg.Type {
				case types.PrevoteType:
					ourVotes = votes.Prevotes(msg.Round).BitArrayByBlockID(msg.BlockID)
				case types.PrecommitType:
					ourVotes = votes.Precommits(msg.Round).BitArrayByBlockID(msg.BlockID)
				default:
					panic("Bad VoteSetBitsMessage field Type. Forgot to add a check in ValidateBasic?")
				}
				ps.ApplyVoteSetBitsMessage(msg, ourVotes)
			} else {
				ps.ApplyVoteSetBitsMessage(msg, nil)
			}
		default:
			// don't punish (leave room for soft upgrades)
			conR.Logger.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	default:
		conR.Logger.Error(fmt.Sprintf("Unknown chId %X", chID))
	}
}

// SetEventBus sets event bus.
func (conR *Reactor) SetEventBus(b *types.EventBus) {
	conR.eventBus = b
	conR.conS.SetEventBus(b)
}

// FastSync returns whether the consensus reactor is in fast-sync mode.
func (conR *Reactor) FastSync() bool {
	conR.mtx.RLock()
	defer conR.mtx.RUnlock()
	return conR.fastSync
}

//--------------------------------------

// subscribeToBroadcastEvents subscribes for new round steps and votes
// using internal pubsub defined on state to broadcast
// them to peers upon receiving.
func (conR *Reactor) subscribeToBroadcastEvents() {
	const subscriber = "consensus-reactor"
	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventNewRoundStep,
		func(data tmevents.EventData) {
			conR.broadcastNewRoundStepMessage(data.(*cstypes.RoundState))

		})

	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventValidBlock,
		func(data tmevents.EventData) {
			conR.broadcastNewValidBlockMessage(data.(*cstypes.RoundState))
		})

	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventVote,
		func(data tmevents.EventData) {
			conR.broadcastHasVoteMessage(data.(*types.Vote))
		})

	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventSignVote,
		func(data tmevents.EventData) {
			conR.broadcastSignVoteMessage(data.(*types.Vote))
		})

	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventBlockPart,
		func(data tmevents.EventData) {
			conR.broadcastHasBlockPartMessage(data.(*HasBlockPartMessage))
		})
	conR.conS.evsw.AddListenerForEvent(subscriber, types.EventProposeRequest,
		func(data tmevents.EventData) {
			conR.broadcastProposeRequestMessage(data.(*ProposeRequestMessage))
		})
}

func (conR *Reactor) unsubscribeFromBroadcastEvents() {
	const subscriber = "consensus-reactor"
	conR.conS.evsw.RemoveListener(subscriber)
}

func (conR *Reactor) broadcastProposeRequestMessage(prMsg *ProposeRequestMessage) {
	conR.Switch.Broadcast(ViewChangeChannel, cdc.MustMarshalBinaryBare(prMsg))
}

func (conR *Reactor) broadcastViewChangeMessage(prMsg *ProposeRequestMessage) *ViewChangeMessage {
	vcMsg := ViewChangeMessage{Height: prMsg.Height, CurrentProposer: prMsg.CurrentProposer, NewProposer: prMsg.NewProposer}
	signature, err := conR.conS.privValidator.SignBytes(vcMsg.SignBytes())
	if err != nil {
		conR.Logger.Error("broadcastViewChangeMessage", "err", err)
		return nil
	}
	vcMsg.Signature = signature
	conR.Switch.Broadcast(ViewChangeChannel, cdc.MustMarshalBinaryBare(vcMsg))
	return &vcMsg
}

func (conR *Reactor) broadcastNewRoundStepMessage(rs *cstypes.RoundState) {
	nrsMsg := makeRoundStepMessage(rs)
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(nrsMsg))
}

func (conR *Reactor) broadcastNewValidBlockMessage(rs *cstypes.RoundState) {
	csMsg := &NewValidBlockMessage{
		Height:           rs.Height,
		Round:            rs.Round,
		BlockPartsHeader: rs.ProposalBlockParts.Header(),
		BlockParts:       rs.ProposalBlockParts.BitArray(),
		IsCommit:         rs.Step == cstypes.RoundStepCommit,
	}
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(csMsg))
}

// Broadcasts HasBlockPartMessage to peers that care.
func (conR *Reactor) broadcastHasBlockPartMessage(msg *HasBlockPartMessage) {
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(msg))
}

// Broadcasts HasVoteMessage to peers that care.
func (conR *Reactor) broadcastHasVoteMessage(vote *types.Vote) {
	msg := &HasVoteMessage{
		Height: vote.Height,
		Round:  vote.Round,
		Type:   vote.Type,
		Index:  vote.ValidatorIndex,
	}
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(msg))
	/*
		// TODO: Make this broadcast more selective.
		for _, peer := range conR.Switch.Peers().List() {
			ps, ok := peer.Get(PeerStateKey).(*PeerState)
			if !ok {
				panic(fmt.Sprintf("Peer %v has no state", peer))
			}
			prs := ps.GetRoundState()
			if prs.Height == vote.Height {
				// TODO: Also filter on round?
				peer.TrySend(StateChannel, struct{ ConsensusMessage }{msg})
			} else {
				// Height doesn't match
				// TODO: check a field, maybe CatchupCommitRound?
				// TODO: But that requires changing the struct field comment.
			}
		}
	*/
}
func (conR *Reactor) broadcastSignVoteMessage(vote *types.Vote) {
	msg := &VoteMessage{vote}
	conR.Switch.Broadcast(VoteChannel, cdc.MustMarshalBinaryBare(msg))
}
func makeRoundStepMessage(rs *cstypes.RoundState) (nrsMsg *NewRoundStepMessage) {
	nrsMsg = &NewRoundStepMessage{
		Height:                rs.Height,
		Round:                 rs.Round,
		Step:                  rs.Step,
		SecondsSinceStartTime: int(time.Since(rs.StartTime).Seconds()),
		LastCommitRound:       rs.LastCommit.GetRound(),
		HasVC:                 rs.HasVC,
	}
	return
}

func (conR *Reactor) sendNewRoundStepMessage(peer p2p.Peer) {
	rs := conR.getRoundState()
	nrsMsg := makeRoundStepMessage(rs)
	peer.Send(StateChannel, cdc.MustMarshalBinaryBare(nrsMsg))
}

func (conR *Reactor) gossipDataRoutine(peer p2p.Peer, ps *PeerState) {
	logger := conR.Logger.With("peer", peer)

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			logger.Info("Stopping gossipDataRoutine for peer")
			return
		}
		rs := conR.getRoundState()
		prs := ps.GetRoundState()

		// Send proposal Block parts?
		if rs.ProposalBlockParts.HasHeader(prs.ProposalBlockPartsHeader) {
			if index, ok := rs.ProposalBlockParts.BitArray().Sub(prs.ProposalBlockParts.Copy()).PickRandom(); ok {
				part := rs.ProposalBlockParts.GetPart(index)
				msg := &BlockPartMessage{
					Height: rs.Height, // This tells peer that this part applies to us.
					Round:  rs.Round,  // This tells peer that this part applies to us.
					Part:   part,
				}
				logger.Debug("Sending block part", "height", prs.Height, "round", prs.Round)
				if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
					ps.SetHasProposalBlockPart(prs.Height, prs.Round, index, BP_SEND, conR.conS.bt)
				}
				continue OUTER_LOOP
			}
		}

		// If the peer is on a previous height that we have, help catch up.
		if (0 < prs.Height) && (prs.Height < rs.Height) && (prs.Height >= conR.conS.blockStore.Base()) {
			heightLogger := logger.With("height", prs.Height)

			// if we never received the commit message from the peer, the block parts wont be initialized
			if prs.ProposalBlockParts == nil {
				blockMeta := conR.conS.blockStore.LoadBlockMeta(prs.Height)
				if blockMeta == nil {
					heightLogger.Error("Failed to load block meta",
						"blockstoreBase", conR.conS.blockStore.Base(), "blockstoreHeight", conR.conS.blockStore.Height())
					time.Sleep(conR.conS.config.PeerGossipSleepDuration)
				} else {
					ps.InitProposalBlockParts(blockMeta.BlockID.PartsHeader)
				}
				// continue the loop since prs is a copy and not effected by this initialization
				continue OUTER_LOOP
			}
			conR.gossipDataForCatchup(heightLogger, rs, prs, ps, peer)
			continue OUTER_LOOP
		}

		// If height and round don't match, sleep.
		if (rs.Height != prs.Height) || (rs.Round != prs.Round) {
			//logger.Info("Peer Height|Round mismatch, sleeping", "peerHeight", prs.Height, "peerRound", prs.Round, "peer", peer)
			time.Sleep(conR.conS.config.PeerGossipSleepDuration)
			continue OUTER_LOOP
		}

		// By here, height and round match.
		// Proposal block parts were already matched and sent if any were wanted.
		// (These can match on hash so the round doesn't matter)
		// Now consider sending other things, like the Proposal itself.

		// Send Proposal && ProposalPOL BitArray?
		if rs.Proposal != nil && !prs.Proposal {
			// Proposal: share the proposal metadata with peer.
			{
				msg := &ProposalMessage{Proposal: rs.Proposal}
				logger.Debug("Sending proposal", "height", prs.Height, "round", prs.Round)
				if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
					// NOTE[ZM]: A peer might have received different proposal msg so this Proposal msg will be rejected!
					ps.SetHasProposal(rs.Proposal)
				}
			}
			// ProposalPOL: lets peer know which POL votes we have so far.
			// Peer must receive ProposalMessage first.
			// rs.Proposal was validated, so rs.Proposal.POLRound <= rs.Round,
			// so we definitely have rs.Votes.Prevotes(rs.Proposal.POLRound).
			if 0 <= rs.Proposal.POLRound {
				msg := &ProposalPOLMessage{
					Height:           rs.Height,
					ProposalPOLRound: rs.Proposal.POLRound,
					ProposalPOL:      rs.Votes.Prevotes(rs.Proposal.POLRound).BitArray(),
				}
				logger.Debug("Sending POL", "height", prs.Height, "round", prs.Round)
				peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg))
			}
			continue OUTER_LOOP
		}

		// Nothing to do. Sleep.
		time.Sleep(conR.conS.config.PeerGossipSleepDuration)
		continue OUTER_LOOP
	}
}

func (conR *Reactor) gossipDataForCatchup(logger log.Logger, rs *cstypes.RoundState,
	prs *cstypes.PeerRoundState, ps *PeerState, peer p2p.Peer) {

	if index, ok := prs.ProposalBlockParts.Not().PickRandom(); ok {
		// Ensure that the peer's PartSetHeader is correct
		blockMeta := conR.conS.blockStore.LoadBlockMeta(prs.Height)
		if blockMeta == nil {
			logger.Error("Failed to load block meta", "ourHeight", rs.Height,
				"blockstoreBase", conR.conS.blockStore.Base(), "blockstoreHeight", conR.conS.blockStore.Height())
			time.Sleep(conR.conS.config.PeerGossipSleepDuration)
			return
		} else if !blockMeta.BlockID.PartsHeader.Equals(prs.ProposalBlockPartsHeader) {
			logger.Info("Peer ProposalBlockPartsHeader mismatch, sleeping",
				"blockPartsHeader", blockMeta.BlockID.PartsHeader, "peerBlockPartsHeader", prs.ProposalBlockPartsHeader)
			time.Sleep(conR.conS.config.PeerGossipSleepDuration)
			return
		}
		// Load the part
		part := conR.conS.blockStore.LoadBlockPart(prs.Height, index)
		if part == nil {
			logger.Error("Could not load part", "index", index,
				"blockPartsHeader", blockMeta.BlockID.PartsHeader, "peerBlockPartsHeader", prs.ProposalBlockPartsHeader)
			time.Sleep(conR.conS.config.PeerGossipSleepDuration)
			return
		}

		// Send the part
		msg := &BlockPartMessage{
			Height: prs.Height, // Not our height, so it doesn't matter.
			Round:  prs.Round,  // Not our height, so it doesn't matter.
			Part:   part,
		}
		logger.Debug("Sending block part for catchup", "round", prs.Round, "index", index)
		if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
			ps.SetHasProposalBlockPart(prs.Height, prs.Round, index, BP_CATCHUP, conR.conS.bt)
		} else {
			logger.Debug("Sending block part for catchup failed")
		}
		return
	}
	//logger.Info("No parts to send in catch-up, sleeping")
	time.Sleep(conR.conS.config.PeerGossipSleepDuration)
}

func (conR *Reactor) gossipVotesRoutine(peer p2p.Peer, ps *PeerState) {
	logger := conR.Logger.With("peer", peer)

	// Simple hack to throttle logs upon sleep.
	var sleeping = 0

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			logger.Info("Stopping gossipVotesRoutine for peer")
			return
		}
		rs := conR.getRoundState()
		prs := ps.GetRoundState()

		switch sleeping {
		case 1: // First sleep
			sleeping = 2
		case 2: // No more sleep
			sleeping = 0
		}

		//logger.Debug("gossipVotesRoutine", "rsHeight", rs.Height, "rsRound", rs.Round,
		//	"prsHeight", prs.Height, "prsRound", prs.Round, "prsStep", prs.Step)

		// If height matches, then send LastCommit, Prevotes, Precommits.
		if rs.Height == prs.Height {
			heightLogger := logger.With("height", prs.Height)
			if conR.gossipVotesForHeight(heightLogger, rs, prs, ps) {
				continue OUTER_LOOP
			}
		}

		// Special catchup logic.
		// If peer is lagging by height 1, send LastCommit.
		if prs.Height != 0 && rs.Height == prs.Height+1 {
			if ps.PickSendVote(rs.LastCommit) {
				logger.Debug("Picked rs.LastCommit to send", "height", prs.Height)
				continue OUTER_LOOP
			}
		}

		// Catchup logic
		// If peer is lagging by more than 1, send Commit.
		if prs.Height != 0 && rs.Height >= prs.Height+2 {
			// Load the block commit for prs.Height,
			// which contains precommit signatures for prs.Height.
			commit := conR.conS.blockStore.LoadBlockCommit(prs.Height)
			if ps.PickSendVote(commit) {
				logger.Debug("Picked Catchup commit to send", "height", prs.Height)
				continue OUTER_LOOP
			}
		}

		if sleeping == 0 {
			// We sent nothing. Sleep...
			sleeping = 1
			logger.Debug("No votes to send, sleeping", "rs.Height", rs.Height, "prs.Height", prs.Height,
				"localPV", rs.Votes.Prevotes(rs.Round).BitArray(), "peerPV", prs.Prevotes,
				"localPC", rs.Votes.Precommits(rs.Round).BitArray(), "peerPC", prs.Precommits)
		} else if sleeping == 2 {
			// Continued sleep...
			sleeping = 1
		}

		time.Sleep(conR.conS.config.PeerGossipSleepDuration)
		continue OUTER_LOOP
	}
}

func (conR *Reactor) gossipVotesForHeight(
	logger log.Logger,
	rs *cstypes.RoundState,
	prs *cstypes.PeerRoundState,
	ps *PeerState,
) bool {

	// If there are lastCommits to send...
	if prs.Step == cstypes.RoundStepNewHeight {
		if ps.PickSendVote(rs.LastCommit) {
			logger.Debug("Picked rs.LastCommit to send")
			return true
		}
	}
	// If there are POL prevotes to send...
	if prs.Step <= cstypes.RoundStepPropose && prs.Round != -1 && prs.Round <= rs.Round && prs.ProposalPOLRound != -1 {
		if polPrevotes := rs.Votes.Prevotes(prs.ProposalPOLRound); polPrevotes != nil {
			if ps.PickSendVote(polPrevotes) {
				logger.Debug("Picked rs.Prevotes(prs.ProposalPOLRound) to send",
					"round", prs.ProposalPOLRound)
				return true
			}
		}
	}
	// If there are prevotes to send...
	if prs.Step <= cstypes.RoundStepPrevoteWait && prs.Round != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Prevotes(prs.Round)) {
			logger.Debug("Picked rs.Prevotes(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are precommits to send...
	if prs.Step <= cstypes.RoundStepPrecommitWait && prs.Round != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Precommits(prs.Round)) {
			logger.Debug("Picked rs.Precommits(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are prevotes to send...Needed because of validBlock mechanism
	if prs.Round != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Prevotes(prs.Round)) {
			logger.Debug("Picked rs.Prevotes(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are POLPrevotes to send...
	if prs.ProposalPOLRound != -1 {
		if polPrevotes := rs.Votes.Prevotes(prs.ProposalPOLRound); polPrevotes != nil {
			if ps.PickSendVote(polPrevotes) {
				logger.Debug("Picked rs.Prevotes(prs.ProposalPOLRound) to send",
					"round", prs.ProposalPOLRound)
				return true
			}
		}
	}

	return false
}

func (conR *Reactor) gossipVCRoutine(peer p2p.Peer, ps *PeerState) {
	logger := conR.Logger.With("peer", peer)

OUTER_LOOP:
	for {
		time.Sleep(conR.conS.config.PeerGossipSleepDuration * 2)

		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			logger.Info("Stopping gossipDataRoutine for peer")
			return
		}

		if !GetActiveVC() {
			time.Sleep(time.Second)
			continue OUTER_LOOP
		}

		rs := conR.getRoundState()
		prs := ps.GetRoundState()
		vcMsg := conR.conS.vcMsg

		if vcMsg == nil || rs.Height > vcMsg.Height {
			continue OUTER_LOOP
		}
		// only in round0 send vcMsg
		if rs.Round != 0 || prs.Round != 0 {
			continue OUTER_LOOP
		}
		// send vcMsg
		if vcMsg.Height == prs.Height && prs.AVCHeight < vcMsg.Height {
			peer.Send(ViewChangeChannel, cdc.MustMarshalBinaryBare(vcMsg))
			//conR.Switch.Broadcast(ViewChangeChannel, cdc.MustMarshalBinaryBare(vcMsg))
			ps.SetAvcHeight(vcMsg.Height)
		}

		if rs.Height == vcMsg.Height {
			// send proposal
			if rs.Proposal != nil {
				msg := &ProposalMessage{Proposal: rs.Proposal}
				peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg))
			}
		}

		continue OUTER_LOOP
	}
}

// NOTE: `queryMaj23Routine` has a simple crude design since it only comes
// into play for liveness when there's a signature DDoS attack happening.
func (conR *Reactor) queryMaj23Routine(peer p2p.Peer, ps *PeerState) {
	logger := conR.Logger.With("peer", peer)

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			logger.Info("Stopping queryMaj23Routine for peer")
			return
		}

		// Maybe send Height/Round/Prevotes
		{
			rs := conR.getRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height {
				if maj23, ok := rs.Votes.Prevotes(prs.Round).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   prs.Round,
						Type:    types.PrevoteType,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23SleepDuration)
				}
			}
		}

		// Maybe send Height/Round/Precommits
		{
			rs := conR.getRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height {
				if maj23, ok := rs.Votes.Precommits(prs.Round).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   prs.Round,
						Type:    types.PrecommitType,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23SleepDuration)
				}
			}
		}

		// Maybe send Height/Round/ProposalPOL
		{
			rs := conR.getRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height && prs.ProposalPOLRound >= 0 {
				if maj23, ok := rs.Votes.Prevotes(prs.ProposalPOLRound).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   prs.ProposalPOLRound,
						Type:    types.PrevoteType,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23SleepDuration)
				}
			}
		}

		// Little point sending LastCommitRound/LastCommit,
		// These are fleeting and non-blocking.

		// Maybe send Height/CatchupCommitRound/CatchupCommit.
		{
			prs := ps.GetRoundState()
			if prs.CatchupCommitRound != -1 && prs.Height > 0 && prs.Height <= conR.conS.blockStore.Height() &&
				prs.Height >= conR.conS.blockStore.Base() {
				if commit := conR.conS.LoadCommit(prs.Height); commit != nil {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   commit.Round,
						Type:    types.PrecommitType,
						BlockID: commit.BlockID,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23SleepDuration)
				}
			}
		}

		time.Sleep(conR.conS.config.PeerQueryMaj23SleepDuration)

		continue OUTER_LOOP
	}
}

func (conR *Reactor) peerStatsRoutine() {
	conR.resetSwitchToFastSyncTimer()

	defer func() {
		conR.stopSwitchToFastSyncTimer()
	}()

	for {
		if !conR.IsRunning() {
			conR.Logger.Info("Stopping peerStatsRoutine")
			return
		}

		select {
		case msg := <-conR.conS.statsMsgQueue:
			// Get peer
			peer := conR.Switch.Peers().Get(msg.PeerID)
			if peer == nil {
				conR.Logger.Debug("Attempt to update stats for non-existent peer",
					"peer", msg.PeerID)
				continue
			}
			// Get peer state
			ps, ok := peer.Get(types.PeerStateKey).(*PeerState)
			if !ok {
				panic(fmt.Sprintf("Peer %v has no state", peer))
			}
			switch msg.Msg.(type) {
			case *VoteMessage:
				if numVotes := ps.RecordVote(); numVotes%votesToContributeToBecomeGoodPeer == 0 {
					conR.Switch.MarkPeerAsGood(peer)
				}
			case *BlockPartMessage:
				if numParts := ps.RecordBlockPart(); numParts%blocksToContributeToBecomeGoodPeer == 0 {
					conR.Switch.MarkPeerAsGood(peer)
				}
			}
		case <-conR.switchToFastSyncTimer.C:
			bcR, ok := conR.Switch.Reactor("BLOCKCHAIN").(blockchainReactor)
			if ok {
				bcR.CheckFastSyncCondition()
				conR.resetSwitchToFastSyncTimer()
			}

		case <-conR.conS.Quit():
			return

		case <-conR.Quit():
			return
		}
	}
}

// String returns a string representation of the Reactor.
// NOTE: For now, it is just a hard-coded string to avoid accessing unprotected shared variables.
// TODO: improve!
func (conR *Reactor) String() string {
	// better not to access shared variables
	return "ConsensusReactor" // conR.StringIndented("")
}

// StringIndented returns an indented string representation of the Reactor
func (conR *Reactor) StringIndented(indent string) string {
	s := "ConsensusReactor{\n"
	s += indent + "  " + conR.conS.StringIndented(indent+"  ") + "\n"
	for _, peer := range conR.Switch.Peers().List() {
		ps, ok := peer.Get(types.PeerStateKey).(*PeerState)
		if !ok {
			panic(fmt.Sprintf("Peer %v has no state", peer))
		}
		s += indent + "  " + ps.StringIndented(indent+"  ") + "\n"
	}
	s += indent + "}"
	return s
}

func (conR *Reactor) updateFastSyncingMetric() {
	var fastSyncing float64
	if conR.fastSync {
		fastSyncing = 1
	} else {
		fastSyncing = 0
	}
	conR.metrics.FastSyncing.Set(fastSyncing)
}

// ReactorMetrics sets the metrics
func ReactorMetrics(metrics *Metrics) ReactorOption {
	return func(conR *Reactor) { conR.metrics = metrics }
}

//-----------------------------------------------------------------------------

var (
	ErrPeerStateHeightRegression = errors.New("error peer state height regression")
	ErrPeerStateInvalidStartTime = errors.New("error peer state invalid startTime")
)

// PeerState contains the known state of a peer, including its connection and
// threadsafe access to its PeerRoundState.
// NOTE: THIS GETS DUMPED WITH rpc/core/consensus.go.
// Be mindful of what you Expose.
type PeerState struct {
	peer   p2p.Peer
	logger log.Logger

	mtx   sync.Mutex             // NOTE: Modify below using setters, never directly.
	PRS   cstypes.PeerRoundState `json:"round_state"` // Exposed.
	Stats *peerStateStats        `json:"stats"`       // Exposed.
}

// peerStateStats holds internal statistics for a peer.
type peerStateStats struct {
	Votes      int `json:"votes"`
	BlockParts int `json:"block_parts"`
}

func (pss peerStateStats) String() string {
	return fmt.Sprintf("peerStateStats{votes: %d, blockParts: %d}",
		pss.Votes, pss.BlockParts)
}

// NewPeerState returns a new PeerState for the given Peer
func NewPeerState(peer p2p.Peer) *PeerState {
	return &PeerState{
		peer:   peer,
		logger: log.NewNopLogger(),
		PRS: cstypes.PeerRoundState{
			Round:              -1,
			ProposalPOLRound:   -1,
			LastCommitRound:    -1,
			CatchupCommitRound: -1,
		},
		Stats: &peerStateStats{},
	}
}

// SetLogger allows to set a logger on the peer state. Returns the peer state
// itself.
func (ps *PeerState) SetLogger(logger log.Logger) *PeerState {
	ps.logger = logger
	return ps
}

// GetRoundState returns an shallow copy of the PeerRoundState.
// There's no point in mutating it since it won't change PeerState.
func (ps *PeerState) GetRoundState() *cstypes.PeerRoundState {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	prs := ps.PRS // copy
	return &prs
}

// ToJSON returns a json of PeerState, marshalled using go-amino.
func (ps *PeerState) ToJSON() ([]byte, error) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return cdc.MarshalJSON(ps)
}

// GetHeight returns an atomic snapshot of the PeerRoundState's height
// used by the mempool to ensure peers are caught up before broadcasting new txs
func (ps *PeerState) GetHeight() int64 {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.PRS.Height
}

// SetAvcHeight sets the given hasVC as known for the peer
func (ps *PeerState) SetAvcHeight(height int64) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.PRS.AVCHeight = height
}

// SetHasProposal sets the given proposal as known for the peer.
func (ps *PeerState) SetHasProposal(proposal *types.Proposal) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != proposal.Height || ps.PRS.Round != proposal.Round {
		return
	}

	if ps.PRS.Proposal {
		return
	}

	ps.PRS.Proposal = true

	// ps.PRS.ProposalBlockParts is set due to NewValidBlockMessage
	if ps.PRS.ProposalBlockParts != nil {
		return
	}

	ps.PRS.ProposalBlockPartsHeader = proposal.BlockID.PartsHeader
	ps.PRS.ProposalBlockParts = bits.NewBitArray(proposal.BlockID.PartsHeader.Total)
	ps.PRS.ProposalPOLRound = proposal.POLRound
	ps.PRS.ProposalPOL = nil // Nil until ProposalPOLMessage received.
}

// InitProposalBlockParts initializes the peer's proposal block parts header and bit array.
func (ps *PeerState) InitProposalBlockParts(partsHeader types.PartSetHeader) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.ProposalBlockParts != nil {
		return
	}

	ps.PRS.ProposalBlockPartsHeader = partsHeader
	ps.PRS.ProposalBlockParts = bits.NewBitArray(partsHeader.Total)
}

// SetHasProposalBlockPart sets the given block part index as known for the peer.
func (ps *PeerState) SetHasProposalBlockPart(height int64, round int, index int, t bpType, bt *BlockTransport) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != height || ps.PRS.Round != round {
		return
	}

	switch t {
	case BP_SEND:
		bt.onBPSend()
	case BP_ACK:
		if !ps.PRS.ProposalBlockParts.GetIndex(index) {
			bt.onBPACKHit()
		}
	case BP_RECV:
		if !ps.PRS.ProposalBlockParts.GetIndex(index) {
			bt.onBPDataHit()
		}
	}

	ps.PRS.ProposalBlockParts.SetIndex(index, true)
}

// PickSendVote picks a vote and sends it to the peer.
// Returns true if vote was sent.
func (ps *PeerState) PickSendVote(votes types.VoteSetReader) bool {
	if vote, ok := ps.PickVoteToSend(votes); ok {
		msg := &VoteMessage{vote}
		ps.logger.Debug("Sending vote message", "ps", ps, "vote", vote)
		if ps.peer.Send(VoteChannel, cdc.MustMarshalBinaryBare(msg)) {
			ps.SetHasVote(vote)
			return true
		}
		return false
	}
	return false
}

// PickVoteToSend picks a vote to send to the peer.
// Returns true if a vote was picked.
// NOTE: `votes` must be the correct Size() for the Height().
func (ps *PeerState) PickVoteToSend(votes types.VoteSetReader) (vote *types.Vote, ok bool) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if votes.Size() == 0 {
		return nil, false
	}

	height, round, votesType, size := votes.GetHeight(), votes.GetRound(), types.SignedMsgType(votes.Type()), votes.Size()

	// Lazily set data using 'votes'.
	if votes.IsCommit() {
		ps.ensureCatchupCommitRound(height, round, size)
	}
	ps.ensureVoteBitArrays(height, size)

	psVotes := ps.getVoteBitArray(height, round, votesType)
	if psVotes == nil {
		return nil, false // Not something worth sending
	}
	if index, ok := votes.BitArray().Sub(psVotes).PickRandom(); ok {
		return votes.GetByIndex(index), true
	}
	return nil, false
}

func (ps *PeerState) getVoteBitArray(height int64, round int, votesType types.SignedMsgType) *bits.BitArray {
	if !types.IsVoteTypeValid(votesType) {
		return nil
	}

	if ps.PRS.Height == height {
		if ps.PRS.Round == round {
			switch votesType {
			case types.PrevoteType:
				return ps.PRS.Prevotes
			case types.PrecommitType:
				return ps.PRS.Precommits
			}
		}
		if ps.PRS.CatchupCommitRound == round {
			switch votesType {
			case types.PrevoteType:
				return nil
			case types.PrecommitType:
				return ps.PRS.CatchupCommit
			}
		}
		if ps.PRS.ProposalPOLRound == round {
			switch votesType {
			case types.PrevoteType:
				return ps.PRS.ProposalPOL
			case types.PrecommitType:
				return nil
			}
		}
		return nil
	}
	if ps.PRS.Height == height+1 {
		if ps.PRS.LastCommitRound == round {
			switch votesType {
			case types.PrevoteType:
				return nil
			case types.PrecommitType:
				return ps.PRS.LastCommit
			}
		}
		return nil
	}
	return nil
}

// 'round': A round for which we have a +2/3 commit.
func (ps *PeerState) ensureCatchupCommitRound(height int64, round int, numValidators int) {
	if ps.PRS.Height != height {
		return
	}
	/*
		NOTE: This is wrong, 'round' could change.
		e.g. if orig round is not the same as block LastCommit round.
		if ps.CatchupCommitRound != -1 && ps.CatchupCommitRound != round {
			panic(fmt.Sprintf(
				"Conflicting CatchupCommitRound. Height: %v,
				Orig: %v,
				New: %v",
				height,
				ps.CatchupCommitRound,
				round))
		}
	*/
	if ps.PRS.CatchupCommitRound == round {
		return // Nothing to do!
	}
	ps.PRS.CatchupCommitRound = round
	if round == ps.PRS.Round {
		ps.PRS.CatchupCommit = ps.PRS.Precommits
	} else {
		ps.PRS.CatchupCommit = bits.NewBitArray(numValidators)
	}
}

// EnsureVoteBitArrays ensures the bit-arrays have been allocated for tracking
// what votes this peer has received.
// NOTE: It's important to make sure that numValidators actually matches
// what the node sees as the number of validators for height.
func (ps *PeerState) EnsureVoteBitArrays(height int64, numValidators int) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.ensureVoteBitArrays(height, numValidators)
}

func (ps *PeerState) ensureVoteBitArrays(height int64, numValidators int) {
	if ps.PRS.Height == height {
		if ps.PRS.Prevotes == nil {
			ps.PRS.Prevotes = bits.NewBitArray(numValidators)
		}
		if ps.PRS.Precommits == nil {
			ps.PRS.Precommits = bits.NewBitArray(numValidators)
		}
		if ps.PRS.CatchupCommit == nil {
			ps.PRS.CatchupCommit = bits.NewBitArray(numValidators)
		}
		if ps.PRS.ProposalPOL == nil {
			ps.PRS.ProposalPOL = bits.NewBitArray(numValidators)
		}
	} else if ps.PRS.Height == height+1 {
		if ps.PRS.LastCommit == nil {
			ps.PRS.LastCommit = bits.NewBitArray(numValidators)
		}
	}
}

// RecordVote increments internal votes related statistics for this peer.
// It returns the total number of added votes.
func (ps *PeerState) RecordVote() int {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.Stats.Votes++

	return ps.Stats.Votes
}

// VotesSent returns the number of blocks for which peer has been sending us
// votes.
func (ps *PeerState) VotesSent() int {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return ps.Stats.Votes
}

// RecordBlockPart increments internal block part related statistics for this peer.
// It returns the total number of added block parts.
func (ps *PeerState) RecordBlockPart() int {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.Stats.BlockParts++
	return ps.Stats.BlockParts
}

// BlockPartsSent returns the number of useful block parts the peer has sent us.
func (ps *PeerState) BlockPartsSent() int {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return ps.Stats.BlockParts
}

// SetHasVote sets the given vote as known by the peer
func (ps *PeerState) SetHasVote(vote *types.Vote) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.setHasVote(vote.Height, vote.Round, vote.Type, vote.ValidatorIndex)
}

func (ps *PeerState) setHasVote(height int64, round int, voteType types.SignedMsgType, index int) {
	ps.logger.Debug("setHasVote", "peerH", ps.PRS.Height, "peerR", ps.PRS.Round, "H", height, "R", round, "type", voteType, "index", index)

	// NOTE: some may be nil BitArrays -> no side effects.
	psVotes := ps.getVoteBitArray(height, round, voteType)
	if psVotes != nil {
		psVotes.SetIndex(index, true)
	}
}

// ApplyNewRoundStepMessage updates the peer state for the new round.
func (ps *PeerState) ApplyNewRoundStepMessage(msg *NewRoundStepMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	// Ignore duplicates or decreases
	if CompareHRS(msg.Height, msg.Round, msg.Step,
		ps.PRS.Height, ps.PRS.Round, ps.PRS.Step,
		msg.HasVC && msg.Step == cstypes.RoundStepPropose) <= 0 {
		return
	}

	// Just remember these values.
	psHeight := ps.PRS.Height
	psRound := ps.PRS.Round
	psCatchupCommitRound := ps.PRS.CatchupCommitRound
	psCatchupCommit := ps.PRS.CatchupCommit

	startTime := tmtime.Now().Add(-1 * time.Duration(msg.SecondsSinceStartTime) * time.Second)
	ps.PRS.Height = msg.Height
	ps.PRS.Round = msg.Round
	ps.PRS.Step = msg.Step
	ps.PRS.StartTime = startTime
	if psHeight != msg.Height || psRound != msg.Round {
		ps.PRS.Proposal = false
		ps.PRS.ProposalBlockPartsHeader = types.PartSetHeader{}
		ps.PRS.ProposalBlockParts = nil
		ps.PRS.ProposalPOLRound = -1
		ps.PRS.ProposalPOL = nil
		// We'll update the BitArray capacity later.
		ps.PRS.Prevotes = nil
		ps.PRS.Precommits = nil
	}
	if psHeight == msg.Height && psRound != msg.Round && msg.Round == psCatchupCommitRound {
		// Peer caught up to CatchupCommitRound.
		// Preserve psCatchupCommit!
		// NOTE: We prefer to use prs.Precommits if
		// pr.Round matches pr.CatchupCommitRound.
		ps.PRS.Precommits = psCatchupCommit
	}
	if psHeight != msg.Height {
		// Shift Precommits to LastCommit.
		if psHeight+1 == msg.Height && psRound == msg.LastCommitRound {
			ps.PRS.LastCommitRound = msg.LastCommitRound
			ps.PRS.LastCommit = ps.PRS.Precommits
		} else {
			ps.PRS.LastCommitRound = msg.LastCommitRound
			ps.PRS.LastCommit = nil
		}
		// We'll update the BitArray capacity later.
		ps.PRS.CatchupCommitRound = -1
		ps.PRS.CatchupCommit = nil
	}
}

// ApplyNewValidBlockMessage updates the peer state for the new valid block.
func (ps *PeerState) ApplyNewValidBlockMessage(msg *NewValidBlockMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}

	if ps.PRS.Round != msg.Round && !msg.IsCommit {
		return
	}

	ps.PRS.ProposalBlockPartsHeader = msg.BlockPartsHeader
	ps.PRS.ProposalBlockParts = msg.BlockParts
}

// ApplyProposalPOLMessage updates the peer state for the new proposal POL.
func (ps *PeerState) ApplyProposalPOLMessage(msg *ProposalPOLMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}
	if ps.PRS.ProposalPOLRound != msg.ProposalPOLRound {
		return
	}

	// TODO: Merge onto existing ps.PRS.ProposalPOL?
	// We might have sent some prevotes in the meantime.
	ps.PRS.ProposalPOL = msg.ProposalPOL
}

// ApplyHasVoteMessage updates the peer state for the new vote.
func (ps *PeerState) ApplyHasVoteMessage(msg *HasVoteMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}

	ps.setHasVote(msg.Height, msg.Round, msg.Type, msg.Index)
}

// ApplyVoteSetBitsMessage updates the peer state for the bit-array of votes
// it claims to have for the corresponding BlockID.
// `ourVotes` is a BitArray of votes we have for msg.BlockID
// NOTE: if ourVotes is nil (e.g. msg.Height < rs.Height),
// we conservatively overwrite ps's votes w/ msg.Votes.
func (ps *PeerState) ApplyVoteSetBitsMessage(msg *VoteSetBitsMessage, ourVotes *bits.BitArray) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	votes := ps.getVoteBitArray(msg.Height, msg.Round, msg.Type)
	if votes != nil {
		if ourVotes == nil {
			votes.Update(msg.Votes)
		} else {
			otherVotes := votes.Sub(ourVotes)
			hasVotes := otherVotes.Or(msg.Votes)
			votes.Update(hasVotes)
		}
	}
}

// String returns a string representation of the PeerState
func (ps *PeerState) String() string {
	return ps.StringIndented("")
}

// StringIndented returns a string representation of the PeerState
func (ps *PeerState) StringIndented(indent string) string {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return fmt.Sprintf(`PeerState{
%s  Key        %v
%s  RoundState %v
%s  Stats      %v
%s}`,
		indent, ps.peer.ID(),
		indent, ps.PRS.StringIndented(indent+"  "),
		indent, ps.Stats,
		indent)
}

//-----------------------------------------------------------------------------
// Messages

// Message is a message that can be sent and received on the Reactor
type Message interface {
	ValidateBasic() error
}

func RegisterMessages(cdc *amino.Codec) {
	cdc.RegisterInterface((*Message)(nil), nil)
	cdc.RegisterConcrete(&NewRoundStepMessage{}, "tendermint/NewRoundStepMessage", nil)
	cdc.RegisterConcrete(&NewValidBlockMessage{}, "tendermint/NewValidBlockMessage", nil)
	cdc.RegisterConcrete(&ProposalMessage{}, "tendermint/Proposal", nil)
	cdc.RegisterConcrete(&ProposalPOLMessage{}, "tendermint/ProposalPOL", nil)
	cdc.RegisterConcrete(&BlockPartMessage{}, "tendermint/BlockPart", nil)
	cdc.RegisterConcrete(&VoteMessage{}, "tendermint/Vote", nil)
	cdc.RegisterConcrete(&HasVoteMessage{}, "tendermint/HasVote", nil)
	cdc.RegisterConcrete(&VoteSetMaj23Message{}, "tendermint/VoteSetMaj23", nil)
	cdc.RegisterConcrete(&VoteSetBitsMessage{}, "tendermint/VoteSetBits", nil)
	cdc.RegisterConcrete(&HasBlockPartMessage{}, "tendermint/HasBlockPart", nil)
	cdc.RegisterConcrete(&ProposeRequestMessage{}, "tendermint/ProposeRequestMessage", nil)
	cdc.RegisterConcrete(&ProposeResponseMessage{}, "tendermint/ProposeResponseMessage", nil)
	cdc.RegisterConcrete(&ViewChangeMessage{}, "tendermint/ChangeValidator", nil)
}

func decodeMsg(bz []byte) (msg Message, err error) {
	if len(bz) > maxMsgSize {
		return msg, fmt.Errorf("msg exceeds max size (%d > %d)", len(bz), maxMsgSize)
	}
	err = cdc.UnmarshalBinaryBare(bz, &msg)
	return
}

//-------------------------------------

// NewRoundStepMessage is sent for every step taken in the ConsensusState.
// For every height/round/step transition
type NewRoundStepMessage struct {
	Height                int64
	Round                 int
	Step                  cstypes.RoundStepType
	SecondsSinceStartTime int
	LastCommitRound       int
	HasVC                 bool
}

// ValidateBasic performs basic validation.
func (m *NewRoundStepMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if !m.Step.IsValid() {
		return errors.New("invalid Step")
	}

	// NOTE: SecondsSinceStartTime may be negative

	if (m.Height == types.GetStartBlockHeight()+1 && m.LastCommitRound != -1) ||
		(m.Height > types.GetStartBlockHeight()+1 && m.LastCommitRound < -1) {
		// TODO: #2737 LastCommitRound should always be >= 0 for heights > 1
		return errors.New("invalid LastCommitRound (for 1st block: -1, for others: >= 0)")
	}
	return nil
}

// String returns a string representation.
func (m *NewRoundStepMessage) String() string {
	return fmt.Sprintf("[NewRoundStep H:%v R:%v S:%v LCR:%v]",
		m.Height, m.Round, m.Step, m.LastCommitRound)
}

//-------------------------------------

// NewValidBlockMessage is sent when a validator observes a valid block B in some round r,
// i.e., there is a Proposal for block B and 2/3+ prevotes for the block B in the round r.
// In case the block is also committed, then IsCommit flag is set to true.
type NewValidBlockMessage struct {
	Height           int64
	Round            int
	BlockPartsHeader types.PartSetHeader
	BlockParts       *bits.BitArray
	IsCommit         bool
}

// ValidateBasic performs basic validation.
func (m *NewValidBlockMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if err := m.BlockPartsHeader.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong BlockPartsHeader: %v", err)
	}
	if m.BlockParts.Size() == 0 {
		return errors.New("empty blockParts")
	}
	if m.BlockParts.Size() != m.BlockPartsHeader.Total {
		return fmt.Errorf("blockParts bit array size %d not equal to BlockPartsHeader.Total %d",
			m.BlockParts.Size(),
			m.BlockPartsHeader.Total)
	}
	if m.BlockParts.Size() > types.MaxBlockPartsCount {
		return errors.Errorf("blockParts bit array is too big: %d, max: %d", m.BlockParts.Size(), types.MaxBlockPartsCount)
	}
	return nil
}

// String returns a string representation.
func (m *NewValidBlockMessage) String() string {
	return fmt.Sprintf("[ValidBlockMessage H:%v R:%v BP:%v BA:%v IsCommit:%v]",
		m.Height, m.Round, m.BlockPartsHeader, m.BlockParts, m.IsCommit)
}

//-------------------------------------

// ProposalMessage is sent when a new block is proposed.
type ProposalMessage struct {
	Proposal *types.Proposal
}

// ValidateBasic performs basic validation.
func (m *ProposalMessage) ValidateBasic() error {
	return m.Proposal.ValidateBasic()
}

// String returns a string representation.
func (m *ProposalMessage) String() string {
	return fmt.Sprintf("[Proposal %v]", m.Proposal)
}

//-------------------------------------

// ProposalPOLMessage is sent when a previous proposal is re-proposed.
type ProposalPOLMessage struct {
	Height           int64
	ProposalPOLRound int
	ProposalPOL      *bits.BitArray
}

// ValidateBasic performs basic validation.
func (m *ProposalPOLMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.ProposalPOLRound < 0 {
		return errors.New("negative ProposalPOLRound")
	}
	if m.ProposalPOL.Size() == 0 {
		return errors.New("empty ProposalPOL bit array")
	}
	if m.ProposalPOL.Size() > types.MaxVotesCount {
		return errors.Errorf("ProposalPOL bit array is too big: %d, max: %d", m.ProposalPOL.Size(), types.MaxVotesCount)
	}
	return nil
}

// String returns a string representation.
func (m *ProposalPOLMessage) String() string {
	return fmt.Sprintf("[ProposalPOL H:%v POLR:%v POL:%v]", m.Height, m.ProposalPOLRound, m.ProposalPOL)
}

//-------------------------------------

// BlockPartMessage is sent when gossipping a piece of the proposed block.
type BlockPartMessage struct {
	Height int64
	Round  int
	Part   *types.Part
}

// ValidateBasic performs basic validation.
func (m *BlockPartMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if err := m.Part.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong Part: %v", err)
	}
	return nil
}

// String returns a string representation.
func (m *BlockPartMessage) String() string {
	return fmt.Sprintf("[BlockPart H:%v R:%v P:%v]", m.Height, m.Round, m.Part)
}

//-------------------------------------

// VoteMessage is sent when voting for a proposal (or lack thereof).
type VoteMessage struct {
	Vote *types.Vote
}

// ValidateBasic performs basic validation.
func (m *VoteMessage) ValidateBasic() error {
	return m.Vote.ValidateBasic()
}

// String returns a string representation.
func (m *VoteMessage) String() string {
	return fmt.Sprintf("[Vote %v]", m.Vote)
}

//-------------------------------------

// HasBlockPartMessage is sent to indicate that a particular vote has been received.
type HasBlockPartMessage struct {
	Height int64
	Round  int
	Index  int
}

// ValidateBasic performs basic validation.
func (m *HasBlockPartMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if m.Index < 0 {
		return errors.New("negative Index")
	}
	return nil
}

// String returns a string representation.
func (m *HasBlockPartMessage) String() string {
	return fmt.Sprintf("[HasBlockPart VI:%v V:{%v/%02d}]", m.Index, m.Height, m.Round)
}

// HasVoteMessage is sent to indicate that a particular vote has been received.
type HasVoteMessage struct {
	Height int64
	Round  int
	Type   types.SignedMsgType
	Index  int
}

// ValidateBasic performs basic validation.
func (m *HasVoteMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if !types.IsVoteTypeValid(m.Type) {
		return errors.New("invalid Type")
	}
	if m.Index < 0 {
		return errors.New("negative Index")
	}
	return nil
}

// String returns a string representation.
func (m *HasVoteMessage) String() string {
	return fmt.Sprintf("[HasVote VI:%v V:{%v/%02d/%v}]", m.Index, m.Height, m.Round, m.Type)
}

//-------------------------------------

// VoteSetMaj23Message is sent to indicate that a given BlockID has seen +2/3 votes.
type VoteSetMaj23Message struct {
	Height  int64
	Round   int
	Type    types.SignedMsgType
	BlockID types.BlockID
}

// ValidateBasic performs basic validation.
func (m *VoteSetMaj23Message) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if !types.IsVoteTypeValid(m.Type) {
		return errors.New("invalid Type")
	}
	if err := m.BlockID.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong BlockID: %v", err)
	}
	return nil
}

// String returns a string representation.
func (m *VoteSetMaj23Message) String() string {
	return fmt.Sprintf("[VSM23 %v/%02d/%v %v]", m.Height, m.Round, m.Type, m.BlockID)
}

//-------------------------------------

// VoteSetBitsMessage is sent to communicate the bit-array of votes seen for the BlockID.
type VoteSetBitsMessage struct {
	Height  int64
	Round   int
	Type    types.SignedMsgType
	BlockID types.BlockID
	Votes   *bits.BitArray
}

// ValidateBasic performs basic validation.
func (m *VoteSetBitsMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	if m.Round < 0 {
		return errors.New("negative Round")
	}
	if !types.IsVoteTypeValid(m.Type) {
		return errors.New("invalid Type")
	}
	if err := m.BlockID.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong BlockID: %v", err)
	}
	// NOTE: Votes.Size() can be zero if the node does not have any
	if m.Votes.Size() > types.MaxVotesCount {
		return fmt.Errorf("votes bit array is too big: %d, max: %d", m.Votes.Size(), types.MaxVotesCount)
	}
	return nil
}

// String returns a string representation.
func (m *VoteSetBitsMessage) String() string {
	return fmt.Sprintf("[VSB %v/%02d/%v %v %v]", m.Height, m.Round, m.Type, m.BlockID, m.Votes)
}

// ProposeRequestMessage from other peer for request the latest height of consensus block
type ProposeRequestMessage struct {
	Height          int64
	CurrentProposer types.Address
	NewProposer     types.Address
	Proposal        *types.Proposal
	Signature       []byte
}

func (m *ProposeRequestMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	return nil
}

func (m *ProposeRequestMessage) SignBytes() []byte {
	return cdc.MustMarshalBinaryBare(ProposeRequestMessage{Height: m.Height, CurrentProposer: m.CurrentProposer, NewProposer: m.NewProposer, Proposal: m.Proposal})
}

func (m *ProposeRequestMessage) Verify(pubKey crypto.PubKey) error {
	if !bytes.Equal(pubKey.Address(), m.NewProposer) {
		return errors.New("invalid validator address")
	}

	if !pubKey.VerifyBytes(m.SignBytes(), m.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

// ProposeResponseMessage is the response of prMsg
type ProposeResponseMessage struct {
	Height   int64
	Proposal *types.Proposal
}

func (m *ProposeResponseMessage) ValidateBasic() error {
	if m.Height < 0 {
		return errors.New("negative Height")
	}
	return nil
}

// ViewChangeMessage is sent for remind peer to do vc
type ViewChangeMessage struct {
	Height          int64
	CurrentProposer types.Address
	NewProposer     types.Address
	Signature       []byte
}

func (m *ViewChangeMessage) ValidateBasic() error {
	return nil
}

func (m *ViewChangeMessage) Validate(height int64, proposer types.Address) bool {
	if m.Height != height {
		return false
	}
	if !bytes.Equal(proposer, m.CurrentProposer) {
		return false
	}

	return true
}

func (m *ViewChangeMessage) SignBytes() []byte {
	return cdc.MustMarshalBinaryBare(ViewChangeMessage{Height: m.Height, CurrentProposer: m.CurrentProposer, NewProposer: m.NewProposer})
}

func (m *ViewChangeMessage) Verify(pubKey crypto.PubKey) error {
	if !bytes.Equal(pubKey.Address(), m.CurrentProposer) {
		return errors.New("invalid validator address")
	}

	if !pubKey.VerifyBytes(m.SignBytes(), m.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

//-------------------------------------
