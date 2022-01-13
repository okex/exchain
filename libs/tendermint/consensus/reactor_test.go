package consensus

import (
	bytes2 "bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/okex/exchain/libs/tendermint/libs/automation"
	tmpubsub "github.com/okex/exchain/libs/tendermint/libs/pubsub"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbm "github.com/tendermint/tm-db"

	abcicli "github.com/okex/exchain/libs/tendermint/abci/client"
	"github.com/okex/exchain/libs/tendermint/abci/example/kvstore"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	cfg "github.com/okex/exchain/libs/tendermint/config"
	cstypes "github.com/okex/exchain/libs/tendermint/consensus/types"
	"github.com/okex/exchain/libs/tendermint/crypto/tmhash"
	"github.com/okex/exchain/libs/tendermint/libs/bits"
	"github.com/okex/exchain/libs/tendermint/libs/bytes"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	mempl "github.com/okex/exchain/libs/tendermint/mempool"
	"github.com/okex/exchain/libs/tendermint/p2p"
	"github.com/okex/exchain/libs/tendermint/p2p/mock"
	sm "github.com/okex/exchain/libs/tendermint/state"
	"github.com/okex/exchain/libs/tendermint/store"
	"github.com/okex/exchain/libs/tendermint/types"
)

//----------------------------------------------
// in-process testnets

func startConsensusNet(t *testing.T, css []*State, n int) (
	[]*Reactor,
	[]types.Subscription,
	[]*types.EventBus,
) {
	reactors := make([]*Reactor, n)
	blocksSubs := make([]types.Subscription, 0)
	eventBuses := make([]*types.EventBus, n)
	for i := 0; i < n; i++ {
		/*logger, err := tmflags.ParseLogLevel("consensus:info,*:error", logger, "info")
		if err != nil {	t.Fatal(err)}*/
		reactors[i] = NewReactor(css[i], true, false) // so we dont start the consensus states
		reactors[i].SetLogger(css[i].Logger)

		// eventBus is already started with the cs
		eventBuses[i] = css[i].eventBus
		reactors[i].SetEventBus(eventBuses[i])

		blocksSub, err := eventBuses[i].Subscribe(context.Background(), testSubscriber, types.EventQueryNewBlock)
		require.NoError(t, err)
		blocksSubs = append(blocksSubs, blocksSub)

		if css[i].state.LastBlockHeight == 0 { //simulate handle initChain in handshake
			sm.SaveState(css[i].blockExec.DB(), css[i].state)
		}
	}
	// make connected switches and start all reactors
	p2p.MakeConnectedSwitches(config.P2P, n, func(i int, s *p2p.Switch) *p2p.Switch {
		s.AddReactor("CONSENSUS", reactors[i])
		s.SetLogger(reactors[i].conS.Logger.With("module", "p2p"))
		return s
	}, p2p.Connect2Switches)

	// now that everyone is connected,  start the state machines
	// If we started the state machines before everyone was connected,
	// we'd block when the cs fires NewBlockEvent and the peers are trying to start their reactors
	// TODO: is this still true with new pubsub?
	for i := 0; i < n; i++ {
		s := reactors[i].conS.GetState()
		reactors[i].SwitchToConsensus(s, 0)
	}
	return reactors, blocksSubs, eventBuses
}

func stopConsensusNet(logger log.Logger, reactors []*Reactor, eventBuses []*types.EventBus) {
	logger.Info("stopConsensusNet", "n", len(reactors))
	for i, r := range reactors {
		logger.Info("stopConsensusNet: Stopping Reactor", "i", i)
		r.Switch.Stop()
	}
	for i, b := range eventBuses {
		logger.Info("stopConsensusNet: Stopping eventBus", "i", i)
		b.Stop()
	}
	logger.Info("stopConsensusNet: DONE", "n", len(reactors))
}

// Ensure a testnet makes blocks
func TestReactorBasic(t *testing.T) {
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter)
	defer cleanup()
	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)
	// wait till everyone makes the first new block
	timeoutWaitGroup(t, N, func(j int) {
		<-blocksSubs[j].Out()
	}, css)
}

// Ensure we can process blocks with evidence
func TestReactorWithEvidence(t *testing.T) {
	types.RegisterMockEvidences(cdc)
	types.RegisterMockEvidences(types.GetCodec())

	nValidators := 4
	testName := "consensus_reactor_test"
	tickerFunc := newMockTickerFunc(true)
	appFunc := newCounter

	// heed the advice from https://www.sandimetz.com/blog/2016/1/20/the-wrong-abstraction
	// to unroll unwieldy abstractions. Here we duplicate the code from:
	// css := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter)

	genDoc, privVals := randGenesisDoc(nValidators, false, 30)
	css := make([]*State, nValidators)
	logger := consensusLogger()
	for i := 0; i < nValidators; i++ {
		stateDB := dbm.NewMemDB() // each state needs its own db
		state, _ := sm.LoadStateFromDBOrGenesisDoc(stateDB, genDoc)
		thisConfig := ResetConfig(fmt.Sprintf("%s_%d", testName, i))
		defer os.RemoveAll(thisConfig.RootDir)
		ensureDir(path.Dir(thisConfig.Consensus.WalFile()), 0700) // dir for wal
		app := appFunc()
		vals := types.TM2PB.ValidatorUpdates(state.Validators)
		app.InitChain(abci.RequestInitChain{Validators: vals})

		pv := privVals[i]
		// duplicate code from:
		// css[i] = newStateWithConfig(thisConfig, state, privVals[i], app)

		blockDB := dbm.NewMemDB()
		deltaDB := dbm.NewMemDB()
		blockStore := store.NewBlockStore(blockDB)
		deltaStore := store.NewDeltaStore(deltaDB)

		// one for mempool, one for consensus
		mtx := new(sync.Mutex)
		proxyAppConnMem := abcicli.NewLocalClient(mtx, app)
		proxyAppConnCon := abcicli.NewLocalClient(mtx, app)

		// Make Mempool
		mempool := mempl.NewCListMempool(thisConfig.Mempool, proxyAppConnMem, 0)
		mempool.SetLogger(log.TestingLogger().With("module", "mempool"))
		if thisConfig.Consensus.WaitForTxs() {
			mempool.EnableTxsAvailable()
		}

		// mock the evidence pool
		// everyone includes evidence of another double signing
		vIdx := (i + 1) % nValidators
		pubKey, err := privVals[vIdx].GetPubKey()
		require.NoError(t, err)
		evpool := newMockEvidencePool(pubKey.Address())

		// Make State
		blockExec := sm.NewBlockExecutor(stateDB, log.TestingLogger(), proxyAppConnCon, mempool, evpool)
		cs := NewState(thisConfig.Consensus, state, blockExec, blockStore, deltaStore, mempool, evpool)
		cs.SetLogger(log.TestingLogger().With("module", "consensus"))
		cs.SetPrivValidator(pv)

		eventBus := types.NewEventBus()
		eventBus.SetLogger(log.TestingLogger().With("module", "events"))
		eventBus.Start()
		cs.SetEventBus(eventBus)

		cs.SetTimeoutTicker(tickerFunc())
		cs.SetLogger(logger.With("validator", i, "module", "consensus"))

		css[i] = cs
	}

	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, nValidators)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	// wait till everyone makes the first new block with no evidence
	timeoutWaitGroup(t, nValidators, func(j int) {
		msg := <-blocksSubs[j].Out()
		block := msg.Data().(types.EventDataNewBlock).Block
		assert.True(t, len(block.Evidence.Evidence) == 0)
	}, css)

	// second block should have evidence
	timeoutWaitGroup(t, nValidators, func(j int) {
		msg := <-blocksSubs[j].Out()
		block := msg.Data().(types.EventDataNewBlock).Block
		assert.True(t, len(block.Evidence.Evidence) > 0)
	}, css)
}

// mock evidence pool returns no evidence for block 1,
// and returnes one piece for all higher blocks. The one piece
// is for a given validator at block 1.
type mockEvidencePool struct {
	height int
	ev     []types.Evidence
}

func newMockEvidencePool(val []byte) *mockEvidencePool {
	return &mockEvidencePool{
		ev: []types.Evidence{types.NewMockEvidence(1, time.Now().UTC(), 1, val)},
	}
}

// NOTE: maxBytes is ignored
func (m *mockEvidencePool) PendingEvidence(maxBytes int64) []types.Evidence {
	if m.height > 0 {
		return m.ev
	}
	return nil
}
func (m *mockEvidencePool) AddEvidence(types.Evidence) error { return nil }
func (m *mockEvidencePool) Update(block *types.Block, state sm.State) {
	if m.height > 0 {
		if len(block.Evidence.Evidence) == 0 {
			panic("block has no evidence")
		}
	}
	m.height++
}
func (m *mockEvidencePool) IsCommitted(types.Evidence) bool { return false }

//------------------------------------

// Ensure a testnet makes blocks when there are txs
func TestReactorCreatesBlockWhenEmptyBlocksFalse(t *testing.T) {
	localTest()
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter,
		func(c *cfg.Config) {
			c.Consensus.CreateEmptyBlocks = false
		})
	defer cleanup()
	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	f := func(data []byte) {
		p := css[0].GetState().Validators.GetProposer()
		index := 0
		for i := 0; i < len(css); i++ {
			k, _ := css[i].privValidator.GetPubKey()
			if p.PubKey.Equals(k) {
				index = i
				break
			}
		}
		// send a tx
		if err := assertMempool(css[index].txNotifier).CheckTx(data, nil, mempl.TxInfo{}); err != nil {
			t.Error(err)
		}
		// wait till everyone makes the first new block
		timeoutWaitGroup(t, N, func(j int) {
			<-blocksSubs[j].Out()
		}, css)
	}
	f([]byte{1, 2, 3})
	f([]byte{4, 5, 6})
}

func TestResetProposalBlock(t *testing.T) {
	localTest()
	oneHeightPreRunCount := 0 // except 2
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter,
		func(c *cfg.Config) {
			c.Consensus.CreateEmptyBlocks = false
		})
	defer cleanup()

	blockBHexBytes, _ := hex.DecodeString("0aaf020a02080a120f74656e6465726d696e745f746573741802220c0888a9868f0610b0c9af95032a480a20d0fb69f398456529d5a9fe05551984997ee7c2347d857678dcd538f3ded36965122408011220e4bf838131a30d85d1d26d54e653aa48943d619cf3b3ceda870a4d7b565117fe322090cc57a68da8797fc4a9230e76056d0cb4db5e6515ea59fa5afdb826ea81e59d3a20e6a75f9df009d7fed203c3a641be78bae896ce01dd084e64e64b511bd678c21a4220c267c03255271c3f5e897bdb59d16694dc0ea60fc34868b39d4cd25a37bd107f4a20c267c03255271c3f5e897bdb59d16694dc0ea60fc34868b39d4cd25a37bd107f5220048091bc7ddc283f77bfbf91d73c44da58c3df8a9cbc867405d8b7f3daada22f7214139eba7c5da73e1b857e7edc754cc3bca3b7e02812050a0304050622f40308011a480a20d0fb69f398456529d5a9fe05551984997ee7c2347d857678dcd538f3ded36965122408011220e4bf838131a30d85d1d26d54e653aa48943d619cf3b3ceda870a4d7b565117fe2268080212141335d2144a4d06dc7e696ed551eed0dc0955b8141a0c0888a9868f0610b0c9af95032240404542d9e9d6f4aaa1668fefeaf7f6fe9dc98c3f6b67c2ae9279ab7ed6dc2bed0f16bd55a85a8f64f277b4f6ba59bc4bfced361171bb485fcd4730f8f2cb5a0c226808021214139eba7c5da73e1b857e7edc754cc3bca3b7e0281a0c0888a9868f0610b0c9af9503224018bc193b527a4cb4d798c8216f7d008fbff165eedab70312fbf5dc3e30743c7eff98c4ce942f3634ecc214891a69600230f6b6b33037299f96675cefa87ced072268080212149911771be341ef93cd0489c0890dc292b399581e1a0c0888a9868f0610b0c9af95032240c45ffdd5e78e70f17383853c2a67684f77aba6ef188630c1dbf309e48842cef719588415f3f46158fe843eef33b38d01e59c7a9cba563ee5904b0ca44e659f0e226808021214cca3e2bc46ac13f8666ebb1fc80bf0c71f3dedfa1a0c0888a9868f0610b0c9af95032240e5633a959fd9aebe7e2dbaa7fc07438c6d76340cf3c5cb2b979d76c04c274efa1a67f2ea1109aea7209501c1e34bc946654af7de2769d965a041bd364fa8cd0c")
	bb := getBlock(blockBHexBytes)
	blocksSubs := make([]types.Subscription, 0)
	for _, v := range css {
		blocksSub, _ := v.eventBus.Subscribe(context.Background(), testSubscriber, types.EventQueryNewBlock)
		blocksSubs = append(blocksSubs, blocksSub)
	}

	findV := func() int {
		p := css[0].Validators.GetProposer()
		for index, vs := range css {
			pk, _ := vs.privValidator.GetPubKey()
			if bytes2.Equal(pk.Address(), p.Address) {
				return index
			}
		}
		panic(-1)
	}
	proposalChs := make([]<-chan tmpubsub.Message, len(css))
	newRoundChs := make([]<-chan tmpubsub.Message, len(css))
	prerunChs := make([]<-chan tmpubsub.Message, len(css))
	validBlockEvent := make([]<-chan tmpubsub.Message, len(css))
	for i := 0; i < len(css); i++ {
		eventBus := css[i].eventBus
		proposalChs[i] = subscribe(eventBus, types.EventQueryCompleteProposal)
		newRoundChs[i] = subscribe(eventBus, types.EventQueryNewRound)
		prerunChs[i] = subscribe(eventBus, types.EventQueryNewPreRun)
		validBlockEvent[i] = subscribe(eventBus, types.EventQueryValidBlock)
	}

	round := 0
	height := int64(1)
	for _, v := range css {
		startTestRound(v, height, round)
	}
	ensureNewRound(newRoundChs[0], height, round)
	ensureNewProposal(proposalChs[0], height, round)
	rs := css[0].GetRoundState()
	hash := rs.ProposalBlock.Hash()
	header := rs.ProposalBlockParts.Header()

	collectVotes(height, round, types.PrecommitType, hash, header, css...)
	// send blockPart to other nodes
	bc := make(chan *types.Block, 1)
	go func() {
		b := <-bc
		set := b.MakePartSet(types.BlockPartSizeBytes)
		for j, vs := range css {
			if j == 0 {
				continue
			}
			m := &BlockPartMessage{
				Height: height,
				Round:  round,
			}
			m.Part = set.GetPart(0)
			vs.peerMsgQueue <- msgInfo{Msg: m}
		}
	}()
	once := sync.Once{}
	timeoutWaitGroup(t, N, func(j int) {
		blockEvent := <-blocksSubs[j].Out()
		b := blockEvent.Data().(types.EventDataNewBlock)
		once.Do(func() {
			bc <- b.Block
			bb.LastBlockID = types.BlockID{
				Hash:        b.Block.Hash(),
				PartsHeader: b.Block.MakePartSet(types.BlockPartSizeBytes).Header(),
			}
		})
	}, css)
	// flush
	for i := 0; i < len(css); i++ {
		f := func(index int, c <-chan tmpubsub.Message) {
			for {
				select {
				case <-c:
				default:
					return
				}
			}
		}
		f(i, prerunChs[i])
		f(i, newRoundChs[i])
		f(i, proposalChs[i])
		f(i, validBlockEvent[i])
	}
	automation.EnableRoleTest(true)
	// all the previous prerun task will be blocked
	blockC := make(chan struct{})
	automation.RegisterActionCallBack(2, 0, func(data ...interface{}) {
		<-blockC
	})
	f := func(index int, height int64, data []byte) {
		cs1 := css[index]
		proposalCh := proposalChs[index]
		assertMempool(cs1.txNotifier).CheckTx(data, nil, mempl.TxInfo{})
		ensureNewProposal(proposalCh, height, round)
		rs := cs1.GetRoundState()
		originHash := rs.ProposalBlock.Hash()
		// because we dont use switch , so we will just wait one single
		ensureNewPreRun(prerunChs[index], height, originHash, true)
		oneHeightPreRunCount++

		blockBHash := bb.Hash()
		blockBHeader := bb.MakePartSet(types.BlockPartSizeBytes).Header()
		// then  all nodes vote the blockB
		logger := log.TestingLogger()
		logger.Info("vote another block", "hash", hex.EncodeToString(blockBHash), "originHash", hex.EncodeToString(originHash), "index", index)

		sm.IgnoreSmbCheck = true
		collectVotes(height, round, types.PrecommitType, blockBHash, blockBHeader, css...)

		// it is excepted to see ,nodes reset the proposalBlock because of  wrong commit hash(but actually it is valid block)
		timeoutWaitGroup(t, N, func(i int) {
			<-validBlockEvent[i]
		}, css)

		// send
		set := bb.MakePartSet(types.BlockPartSizeBytes)
		for _, vs := range css {
			m := &BlockPartMessage{
				Height: height,
				Round:  round,
			}
			m.Part = set.GetPart(0)
			vs.peerMsgQueue <- msgInfo{Msg: m}
		}
		// and then ,we will receive one stop event (which event's hash belongs to previous proposalBlock)
		ensureNewPreRun(prerunChs[index], height, originHash, false)
		// after we receive stop event, now we can close blockC to let new task keep going
		close(blockC)

		// now we will meet new prerun task (blockB proposal)
		timeoutWaitGroup(t, N, func(j int) {
			ensureNewPreRun(prerunChs[j], height, blockBHash, true)
		}, css)
		oneHeightPreRunCount++

		// wait till everyone makes the  block
		timeoutWaitGroup(t, N, func(j int) {
			v := <-blocksSubs[j].Out()
			b := v.Data().(types.EventDataNewBlock)
			require.Equal(t, b.Block.Hash(), blockBHash)
		}, css)
	}
	f(findV(), 2, []byte{4, 5, 6})
	require.Equal(t, 2, oneHeightPreRunCount)
}

func TestReactorReceiveDoesNotPanicIfAddPeerHasntBeenCalledYet(t *testing.T) {
	N := 1
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter)
	defer cleanup()
	reactors, _, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
		msg     = cdc.MustMarshalBinaryBare(&HasVoteMessage{Height: 1, Round: 1, Index: 1, Type: types.PrevoteType})
	)

	reactor.InitPeer(peer)

	// simulate switch calling Receive before AddPeer
	assert.NotPanics(t, func() {
		reactor.Receive(StateChannel, peer, msg)
		reactor.AddPeer(peer)
	})
}

func TestReactorReceivePanicsIfInitPeerHasntBeenCalledYet(t *testing.T) {
	N := 1
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter)
	defer cleanup()
	reactors, _, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
		msg     = cdc.MustMarshalBinaryBare(&HasVoteMessage{Height: 1, Round: 1, Index: 1, Type: types.PrevoteType})
	)

	// we should call InitPeer here

	// simulate switch calling Receive before AddPeer
	assert.Panics(t, func() {
		reactor.Receive(StateChannel, peer, msg)
	})
}

// Test we record stats about votes and block parts from other peers.
func TestReactorRecordsVotesAndBlockParts(t *testing.T) {
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newMockTickerFunc(true), newCounter)
	defer cleanup()
	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	// wait till everyone makes the first new block
	timeoutWaitGroup(t, N, func(j int) {
		<-blocksSubs[j].Out()
	}, css)

	// Get peer
	peer := reactors[1].Switch.Peers().List()[0]
	// Get peer state
	ps := peer.Get(types.PeerStateKey).(*PeerState)

	assert.Equal(t, true, ps.VotesSent() > 0, "number of votes sent should have increased")
	assert.Equal(t, true, ps.BlockPartsSent() > 0, "number of votes sent should have increased")
}

//-------------------------------------------------------------
// ensure we can make blocks despite cycling a validator set

func TestReactorVotingPowerChange(t *testing.T) {
	nVals := 4
	logger := log.TestingLogger()
	css, cleanup := randConsensusNet(
		nVals,
		"consensus_voting_power_changes_test",
		newMockTickerFunc(true),
		newPersistentKVStore)
	defer cleanup()
	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, nVals)
	defer stopConsensusNet(logger, reactors, eventBuses)

	// map of active validators
	activeVals := make(map[string]struct{})
	for i := 0; i < nVals; i++ {
		pubKey, err := css[i].privValidator.GetPubKey()
		require.NoError(t, err)
		addr := pubKey.Address()
		activeVals[string(addr)] = struct{}{}
	}

	// wait till everyone makes block 1
	timeoutWaitGroup(t, nVals, func(j int) {
		<-blocksSubs[j].Out()
	}, css)

	//---------------------------------------------------------------------------
	logger.Debug("---------------------------- Testing changing the voting power of one validator a few times")

	val1PubKey, err := css[0].privValidator.GetPubKey()
	require.NoError(t, err)
	val1PubKeyABCI := types.TM2PB.PubKey(val1PubKey)
	updateValidatorTx := kvstore.MakeValSetChangeTx(val1PubKeyABCI, 25)
	previousTotalVotingPower := css[0].GetRoundState().LastValidators.TotalVotingPower()

	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlockWithTx(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)

	if css[0].GetRoundState().LastValidators.TotalVotingPower() == previousTotalVotingPower {
		t.Fatalf(
			"expected voting power to change (before: %d, after: %d)",
			previousTotalVotingPower,
			css[0].GetRoundState().LastValidators.TotalVotingPower())
	}

	updateValidatorTx = kvstore.MakeValSetChangeTx(val1PubKeyABCI, 2)
	previousTotalVotingPower = css[0].GetRoundState().LastValidators.TotalVotingPower()

	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlockWithTx(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)

	if css[0].GetRoundState().LastValidators.TotalVotingPower() == previousTotalVotingPower {
		t.Fatalf(
			"expected voting power to change (before: %d, after: %d)",
			previousTotalVotingPower,
			css[0].GetRoundState().LastValidators.TotalVotingPower())
	}

	updateValidatorTx = kvstore.MakeValSetChangeTx(val1PubKeyABCI, 26)
	previousTotalVotingPower = css[0].GetRoundState().LastValidators.TotalVotingPower()

	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlockWithTx(t, nVals, activeVals, blocksSubs, css, updateValidatorTx)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)
	waitForAndValidateBlock(t, nVals, activeVals, blocksSubs, css)

	if css[0].GetRoundState().LastValidators.TotalVotingPower() == previousTotalVotingPower {
		t.Fatalf(
			"expected voting power to change (before: %d, after: %d)",
			previousTotalVotingPower,
			css[0].GetRoundState().LastValidators.TotalVotingPower())
	}
}

func TestReactorValidatorSetChanges(t *testing.T) {
	return
	nPeers := 7
	nVals := 4
	css, _, _, cleanup := randConsensusNetWithPeers(
		nVals,
		nPeers,
		"consensus_val_set_changes_test",
		newMockTickerFunc(true),
		newPersistentKVStoreWithPath)

	defer cleanup()
	logger := log.TestingLogger()

	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, nPeers)
	defer stopConsensusNet(logger, reactors, eventBuses)

	// map of active validators
	activeVals := make(map[string]struct{})
	for i := 0; i < nVals; i++ {
		pubKey, err := css[i].privValidator.GetPubKey()
		require.NoError(t, err)
		activeVals[string(pubKey.Address())] = struct{}{}
	}

	// wait till everyone makes block 1
	timeoutWaitGroup(t, nPeers, func(j int) {
		<-blocksSubs[j].Out()
	}, css)

	//---------------------------------------------------------------------------
	logger.Info("---------------------------- Testing adding one validator")

	newValidatorPubKey1, err := css[nVals].privValidator.GetPubKey()
	require.NoError(t, err)
	valPubKey1ABCI := types.TM2PB.PubKey(newValidatorPubKey1)
	newValidatorTx1 := kvstore.MakeValSetChangeTx(valPubKey1ABCI, testMinPower)

	// wait till everyone makes block 2
	// ensure the commit includes all validators
	// send newValTx to change vals in block 3
	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css, newValidatorTx1)

	// wait till everyone makes block 3.
	// it includes the commit for block 2, which is by the original validator set
	waitForAndValidateBlockWithTx(t, nPeers, activeVals, blocksSubs, css, newValidatorTx1)

	// wait till everyone makes block 4.
	// it includes the commit for block 3, which is by the original validator set
	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css)

	// the commits for block 4 should be with the updated validator set
	activeVals[string(newValidatorPubKey1.Address())] = struct{}{}

	// wait till everyone makes block 5
	// it includes the commit for block 4, which should have the updated validator set
	waitForBlockWithUpdatedValsAndValidateIt(t, nPeers, activeVals, blocksSubs, css)

	//---------------------------------------------------------------------------
	logger.Info("---------------------------- Testing changing the voting power of one validator")

	updateValidatorPubKey1, err := css[nVals].privValidator.GetPubKey()
	require.NoError(t, err)
	updatePubKey1ABCI := types.TM2PB.PubKey(updateValidatorPubKey1)
	updateValidatorTx1 := kvstore.MakeValSetChangeTx(updatePubKey1ABCI, 25)
	previousTotalVotingPower := css[nVals].GetRoundState().LastValidators.TotalVotingPower()

	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css, updateValidatorTx1)
	waitForAndValidateBlockWithTx(t, nPeers, activeVals, blocksSubs, css, updateValidatorTx1)
	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css)
	waitForBlockWithUpdatedValsAndValidateIt(t, nPeers, activeVals, blocksSubs, css)

	if css[nVals].GetRoundState().LastValidators.TotalVotingPower() == previousTotalVotingPower {
		t.Errorf(
			"expected voting power to change (before: %d, after: %d)",
			previousTotalVotingPower,
			css[nVals].GetRoundState().LastValidators.TotalVotingPower())
	}

	//---------------------------------------------------------------------------
	logger.Info("---------------------------- Testing adding two validators at once")

	newValidatorPubKey2, err := css[nVals+1].privValidator.GetPubKey()
	require.NoError(t, err)
	newVal2ABCI := types.TM2PB.PubKey(newValidatorPubKey2)
	newValidatorTx2 := kvstore.MakeValSetChangeTx(newVal2ABCI, testMinPower)

	newValidatorPubKey3, err := css[nVals+2].privValidator.GetPubKey()
	require.NoError(t, err)
	newVal3ABCI := types.TM2PB.PubKey(newValidatorPubKey3)
	newValidatorTx3 := kvstore.MakeValSetChangeTx(newVal3ABCI, testMinPower)

	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css, newValidatorTx2, newValidatorTx3)
	waitForAndValidateBlockWithTx(t, nPeers, activeVals, blocksSubs, css, newValidatorTx2, newValidatorTx3)
	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css)
	activeVals[string(newValidatorPubKey2.Address())] = struct{}{}
	activeVals[string(newValidatorPubKey3.Address())] = struct{}{}
	waitForBlockWithUpdatedValsAndValidateIt(t, nPeers, activeVals, blocksSubs, css)

	//---------------------------------------------------------------------------
	logger.Info("---------------------------- Testing removing two validators at once")

	removeValidatorTx2 := kvstore.MakeValSetChangeTx(newVal2ABCI, 0)
	removeValidatorTx3 := kvstore.MakeValSetChangeTx(newVal3ABCI, 0)

	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css, removeValidatorTx2, removeValidatorTx3)
	waitForAndValidateBlockWithTx(t, nPeers, activeVals, blocksSubs, css, removeValidatorTx2, removeValidatorTx3)
	waitForAndValidateBlock(t, nPeers, activeVals, blocksSubs, css)
	delete(activeVals, string(newValidatorPubKey2.Address()))
	delete(activeVals, string(newValidatorPubKey3.Address()))
	waitForBlockWithUpdatedValsAndValidateIt(t, nPeers, activeVals, blocksSubs, css)
}

// Check we can make blocks with skip_timeout_commit=false
func TestReactorWithTimeoutCommit(t *testing.T) {
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_with_timeout_commit_test", newMockTickerFunc(false), newCounter)
	defer cleanup()
	// override default SkipTimeoutCommit == true for tests
	for i := 0; i < N; i++ {
		css[i].config.SkipTimeoutCommit = false
	}

	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, N-1)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)

	// wait till everyone makes the first new block
	timeoutWaitGroup(t, N-1, func(j int) {
		<-blocksSubs[j].Out()
	}, css)
}

func waitForAndValidateBlock(
	t *testing.T,
	n int,
	activeVals map[string]struct{},
	blocksSubs []types.Subscription,
	css []*State,
	txs ...[]byte,
) {
	timeoutWaitGroup(t, n, func(j int) {
		css[j].Logger.Debug("waitForAndValidateBlock")
		msg := <-blocksSubs[j].Out()
		newBlock := msg.Data().(types.EventDataNewBlock).Block
		css[j].Logger.Debug("waitForAndValidateBlock: Got block", "height", newBlock.Height)
		err := validateBlock(newBlock, activeVals)
		assert.Nil(t, err)
		for _, tx := range txs {
			err := assertMempool(css[j].txNotifier).CheckTx(tx, nil, mempl.TxInfo{})
			assert.Nil(t, err)
		}
	}, css)
}

func waitForAndValidateBlockWithTx(
	t *testing.T,
	n int,
	activeVals map[string]struct{},
	blocksSubs []types.Subscription,
	css []*State,
	txs ...[]byte,
) {
	timeoutWaitGroup(t, n, func(j int) {
		ntxs := 0
	BLOCK_TX_LOOP:
		for {
			css[j].Logger.Debug("waitForAndValidateBlockWithTx", "ntxs", ntxs)
			msg := <-blocksSubs[j].Out()
			newBlock := msg.Data().(types.EventDataNewBlock).Block
			css[j].Logger.Debug("waitForAndValidateBlockWithTx: Got block", "height", newBlock.Height)
			err := validateBlock(newBlock, activeVals)
			assert.Nil(t, err)

			// check that txs match the txs we're waiting for.
			// note they could be spread over multiple blocks,
			// but they should be in order.
			for _, tx := range newBlock.Data.Txs {
				assert.EqualValues(t, txs[ntxs], tx)
				ntxs++
			}

			if ntxs == len(txs) {
				break BLOCK_TX_LOOP
			}
		}

	}, css)
}

func waitForBlockWithUpdatedValsAndValidateIt(
	t *testing.T,
	n int,
	updatedVals map[string]struct{},
	blocksSubs []types.Subscription,
	css []*State,
) {
	timeoutWaitGroup(t, n, func(j int) {

		var newBlock *types.Block
	LOOP:
		for {
			css[j].Logger.Debug("waitForBlockWithUpdatedValsAndValidateIt")
			msg := <-blocksSubs[j].Out()
			newBlock = msg.Data().(types.EventDataNewBlock).Block
			if newBlock.LastCommit.Size() == len(updatedVals) {
				css[j].Logger.Debug("waitForBlockWithUpdatedValsAndValidateIt: Got block", "height", newBlock.Height)
				break LOOP
			} else {
				css[j].Logger.Debug(
					"waitForBlockWithUpdatedValsAndValidateIt: Got block with no new validators. Skipping",
					"height",
					newBlock.Height)
			}
		}

		err := validateBlock(newBlock, updatedVals)
		assert.Nil(t, err)
	}, css)
}

// expects high synchrony!
func validateBlock(block *types.Block, activeVals map[string]struct{}) error {
	if block.LastCommit.Size() != len(activeVals) {
		return fmt.Errorf(
			"commit size doesn't match number of active validators. Got %d, expected %d",
			block.LastCommit.Size(),
			len(activeVals))
	}

	for _, commitSig := range block.LastCommit.Signatures {
		if _, ok := activeVals[string(commitSig.ValidatorAddress)]; !ok {
			return fmt.Errorf("found vote for inactive validator %X", commitSig.ValidatorAddress)
		}
	}
	return nil
}

func timeoutWaitGroup(t *testing.T, n int, f func(int), css []*State) {
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(j int) {
			f(j)
			wg.Done()
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// we're running many nodes in-process, possibly in in a virtual machine,
	// and spewing debug messages - making a block could take a while,
	timeout := time.Second * 300

	select {
	case <-done:
	case <-time.After(timeout):
		for i, cs := range css {
			t.Log("#################")
			t.Log("Validator", i)
			t.Log(cs.GetRoundState())
			t.Log("")
		}
		os.Stdout.Write([]byte("pprof.Lookup('goroutine'):\n"))
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		capture()
		panic("Timed out waiting for all validators to commit a block")
	}
}

func capture() {
	trace := make([]byte, 10240000)
	count := runtime.Stack(trace, true)
	fmt.Printf("Stack of %d bytes: %s\n", count, trace)
}

//-------------------------------------------------------------
// Ensure basic validation of structs is functioning

func TestNewRoundStepMessageValidateBasic(t *testing.T) {
	testCases := []struct { // nolint: maligned
		expectErr              bool
		messageRound           int
		messageLastCommitRound int
		messageHeight          int64
		testName               string
		messageStep            cstypes.RoundStepType
	}{
		{false, 0, 0, 0, "Valid Message", 0x01},
		{true, -1, 0, 0, "Invalid Message", 0x01},
		{true, 0, 0, -1, "Invalid Message", 0x01},
		{true, 0, 0, 1, "Invalid Message", 0x00},
		{true, 0, 0, 1, "Invalid Message", 0x00},
		{true, 0, -2, 2, "Invalid Message", 0x01},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			message := NewRoundStepMessage{
				Height:          tc.messageHeight,
				Round:           tc.messageRound,
				Step:            tc.messageStep,
				LastCommitRound: tc.messageLastCommitRound,
			}

			assert.Equal(t, tc.expectErr, message.ValidateBasic() != nil, "Validate Basic had an unexpected result")
		})
	}
}

func TestNewValidBlockMessageValidateBasic(t *testing.T) {
	testCases := []struct {
		malleateFn func(*NewValidBlockMessage)
		expErr     string
	}{
		{func(msg *NewValidBlockMessage) {}, ""},
		{func(msg *NewValidBlockMessage) { msg.Height = -1 }, "negative Height"},
		{func(msg *NewValidBlockMessage) { msg.Round = -1 }, "negative Round"},
		{
			func(msg *NewValidBlockMessage) { msg.BlockPartsHeader.Total = 2 },
			"blockParts bit array size 1 not equal to BlockPartsHeader.Total 2",
		},
		{
			func(msg *NewValidBlockMessage) { msg.BlockPartsHeader.Total = 0; msg.BlockParts = bits.NewBitArray(0) },
			"empty blockParts",
		},
		{
			func(msg *NewValidBlockMessage) { msg.BlockParts = bits.NewBitArray(types.MaxBlockPartsCount + 1) },
			"blockParts bit array size 1602 not equal to BlockPartsHeader.Total 1",
		},
	}

	for i, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			msg := &NewValidBlockMessage{
				Height: 1,
				Round:  0,
				BlockPartsHeader: types.PartSetHeader{
					Total: 1,
				},
				BlockParts: bits.NewBitArray(1),
			}

			tc.malleateFn(msg)
			err := msg.ValidateBasic()
			if tc.expErr != "" && assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.expErr)
			}
		})
	}
}

func TestProposalPOLMessageValidateBasic(t *testing.T) {
	testCases := []struct {
		malleateFn func(*ProposalPOLMessage)
		expErr     string
	}{
		{func(msg *ProposalPOLMessage) {}, ""},
		{func(msg *ProposalPOLMessage) { msg.Height = -1 }, "negative Height"},
		{func(msg *ProposalPOLMessage) { msg.ProposalPOLRound = -1 }, "negative ProposalPOLRound"},
		{func(msg *ProposalPOLMessage) { msg.ProposalPOL = bits.NewBitArray(0) }, "empty ProposalPOL bit array"},
		{func(msg *ProposalPOLMessage) { msg.ProposalPOL = bits.NewBitArray(types.MaxVotesCount + 1) },
			"ProposalPOL bit array is too big: 10001, max: 10000"},
	}

	for i, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			msg := &ProposalPOLMessage{
				Height:           1,
				ProposalPOLRound: 1,
				ProposalPOL:      bits.NewBitArray(1),
			}

			tc.malleateFn(msg)
			err := msg.ValidateBasic()
			if tc.expErr != "" && assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.expErr)
			}
		})
	}
}

func TestBlockPartMessageValidateBasic(t *testing.T) {
	testPart := new(types.Part)
	testPart.Proof.LeafHash = tmhash.Sum([]byte("leaf"))
	testCases := []struct {
		testName      string
		messageHeight int64
		messageRound  int
		messagePart   *types.Part
		expectErr     bool
	}{
		{"Valid Message", 0, 0, testPart, false},
		{"Invalid Message", -1, 0, testPart, true},
		{"Invalid Message", 0, -1, testPart, true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			message := BlockPartMessage{
				Height: tc.messageHeight,
				Round:  tc.messageRound,
				Part:   tc.messagePart,
			}

			assert.Equal(t, tc.expectErr, message.ValidateBasic() != nil, "Validate Basic had an unexpected result")
		})
	}

	message := BlockPartMessage{Height: 0, Round: 0, Part: new(types.Part)}
	message.Part.Index = -1

	assert.Equal(t, true, message.ValidateBasic() != nil, "Validate Basic had an unexpected result")
}

func TestHasVoteMessageValidateBasic(t *testing.T) {
	const (
		validSignedMsgType   types.SignedMsgType = 0x01
		invalidSignedMsgType types.SignedMsgType = 0x03
	)

	testCases := []struct { // nolint: maligned
		expectErr     bool
		messageRound  int
		messageIndex  int
		messageHeight int64
		testName      string
		messageType   types.SignedMsgType
	}{
		{false, 0, 0, 0, "Valid Message", validSignedMsgType},
		{true, -1, 0, 0, "Invalid Message", validSignedMsgType},
		{true, 0, -1, 0, "Invalid Message", validSignedMsgType},
		{true, 0, 0, 0, "Invalid Message", invalidSignedMsgType},
		{true, 0, 0, -1, "Invalid Message", validSignedMsgType},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			message := HasVoteMessage{
				Height: tc.messageHeight,
				Round:  tc.messageRound,
				Type:   tc.messageType,
				Index:  tc.messageIndex,
			}

			assert.Equal(t, tc.expectErr, message.ValidateBasic() != nil, "Validate Basic had an unexpected result")
		})
	}
}

func TestVoteSetMaj23MessageValidateBasic(t *testing.T) {
	const (
		validSignedMsgType   types.SignedMsgType = 0x01
		invalidSignedMsgType types.SignedMsgType = 0x03
	)

	validBlockID := types.BlockID{}
	invalidBlockID := types.BlockID{
		Hash: bytes.HexBytes{},
		PartsHeader: types.PartSetHeader{
			Total: -1,
			Hash:  bytes.HexBytes{},
		},
	}

	testCases := []struct { // nolint: maligned
		expectErr      bool
		messageRound   int
		messageHeight  int64
		testName       string
		messageType    types.SignedMsgType
		messageBlockID types.BlockID
	}{
		{false, 0, 0, "Valid Message", validSignedMsgType, validBlockID},
		{true, -1, 0, "Invalid Message", validSignedMsgType, validBlockID},
		{true, 0, -1, "Invalid Message", validSignedMsgType, validBlockID},
		{true, 0, 0, "Invalid Message", invalidSignedMsgType, validBlockID},
		{true, 0, 0, "Invalid Message", validSignedMsgType, invalidBlockID},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.testName, func(t *testing.T) {
			message := VoteSetMaj23Message{
				Height:  tc.messageHeight,
				Round:   tc.messageRound,
				Type:    tc.messageType,
				BlockID: tc.messageBlockID,
			}

			assert.Equal(t, tc.expectErr, message.ValidateBasic() != nil, "Validate Basic had an unexpected result")
		})
	}
}

func TestVoteSetBitsMessageValidateBasic(t *testing.T) {
	testCases := []struct { // nolint: maligned
		malleateFn func(*VoteSetBitsMessage)
		expErr     string
	}{
		{func(msg *VoteSetBitsMessage) {}, ""},
		{func(msg *VoteSetBitsMessage) { msg.Height = -1 }, "negative Height"},
		{func(msg *VoteSetBitsMessage) { msg.Round = -1 }, "negative Round"},
		{func(msg *VoteSetBitsMessage) { msg.Type = 0x03 }, "invalid Type"},
		{func(msg *VoteSetBitsMessage) {
			msg.BlockID = types.BlockID{
				Hash: bytes.HexBytes{},
				PartsHeader: types.PartSetHeader{
					Total: -1,
					Hash:  bytes.HexBytes{},
				},
			}
		}, "wrong BlockID: wrong PartsHeader: negative Total"},
		{func(msg *VoteSetBitsMessage) { msg.Votes = bits.NewBitArray(types.MaxVotesCount + 1) },
			"votes bit array is too big: 10001, max: 10000"},
	}

	for i, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			msg := &VoteSetBitsMessage{
				Height:  1,
				Round:   0,
				Type:    0x01,
				Votes:   bits.NewBitArray(1),
				BlockID: types.BlockID{},
			}

			tc.malleateFn(msg)
			err := msg.ValidateBasic()
			if tc.expErr != "" && assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.expErr)
			}
		})
	}
}
