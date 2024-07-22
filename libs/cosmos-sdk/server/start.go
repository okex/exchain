package server

// DONTCOVER

import (
	"os"
	"runtime/pprof"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/okex/exchain/libs/cosmos-sdk/baseapp"
	"github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/client/lcd"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	"github.com/okex/exchain/libs/cosmos-sdk/server/grpc"
	app2 "github.com/okex/exchain/libs/cosmos-sdk/server/types"
	"github.com/okex/exchain/libs/cosmos-sdk/store/iavl"
	"github.com/okex/exchain/libs/cosmos-sdk/store/mpt"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	tmiavl "github.com/okex/exchain/libs/iavl"
	"github.com/okex/exchain/libs/system"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	bcv0 "github.com/okex/exchain/libs/tendermint/blockchain/v0"
	tcmd "github.com/okex/exchain/libs/tendermint/cmd/tendermint/commands"
	"github.com/okex/exchain/libs/tendermint/consensus"
	"github.com/okex/exchain/libs/tendermint/libs/cli"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmos "github.com/okex/exchain/libs/tendermint/libs/os"
	"github.com/okex/exchain/libs/tendermint/mempool"
	"github.com/okex/exchain/libs/tendermint/node"
	"github.com/okex/exchain/libs/tendermint/p2p"
	pvm "github.com/okex/exchain/libs/tendermint/privval"
	"github.com/okex/exchain/libs/tendermint/proxy"
	"github.com/okex/exchain/libs/tendermint/rpc/client"
	"github.com/okex/exchain/libs/tendermint/rpc/client/local"
	"github.com/okex/exchain/libs/tendermint/state"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
)

// Tendermint full-node start flags
const (
	flagAddress            = "address"
	flagTraceStore         = "trace-store"
	flagCPUProfile         = "cpu-profile"
	FlagMinGasPrices       = "minimum-gas-prices"
	FlagHaltHeight         = "halt-height"
	FlagHaltTime           = "halt-time"
	FlagInterBlockCache    = "inter-block-cache"
	FlagUnsafeSkipUpgrades = "unsafe-skip-upgrades"
	FlagTrace              = "trace"

	FlagPruning           = "pruning"
	FlagPruningKeepRecent = "pruning-keep-recent"
	FlagPruningKeepEvery  = "pruning-keep-every"
	FlagPruningInterval   = "pruning-interval"
	FlagLocalRpcPort      = "local-rpc-port"
	FlagPortMonitor       = "netstat"
	FlagEvmImportPath     = "evm-import-path"
	FlagEvmImportMode     = "evm-import-mode"
	FlagGoroutineNum      = "goroutine-num"

	FlagPruningMaxWsNum = "pruning-max-worldstate-num"
	FlagExportKeystore  = "export-keystore"
	FlagLogServerUrl    = "log-server"

	FlagActiveViewChange = "active-view-change"
	FlagCommitGapHeight  = "commit-gap-height"

	FlagBlockPartSizeBytes = "block-part-size"

	FlagFastSyncGap = "fastsync-gap"

	FlagEventBlockTime = "event-block-time"
)

// StartCmd runs the service passed in, either stand-alone or in-process with
// Tendermint.
func StartCmd(ctx *Context,
	cdc *codec.CodecProxy,
	registry jsonpb.AnyResolver,
	appCreator AppCreator,
	appStop AppStop,
	registerRoutesFn func(restServer *lcd.RestServer),
	registerAppFlagFn func(cmd *cobra.Command),
	appPreRun func(ctx *Context, cmd *cobra.Command) error,
	subFunc func(logger log.Logger) log.Subscriber,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the full node",
		Long: `Run the full node application with Tendermint in or out of process. By
default, the application will run with Tendermint in process.

Pruning options can be provided via the '--pruning' flag or alternatively with '--pruning-keep-recent',
'pruning-keep-every', and 'pruning-interval' together.

For '--pruning' the options are as follows:

default: the last 100 states are kept in addition to every 500th state; pruning at 10 block intervals
nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
everything: all saved states will be deleted, storing only the current state; pruning at 10 block intervals
custom: allow pruning options to be manually specified through 'pruning-keep-recent', 'pruning-keep-every', and 'pruning-interval'

Node halting configurations exist in the form of two flags: '--halt-height' and '--halt-time'. During
the ABCI Commit phase, the node will check if the current block height is greater than or equal to
the halt-height or if the current block time is greater than or equal to the halt-time. If so, the
node will attempt to gracefully shutdown and the block will not be committed. In addition, the node
will not be able to commit subsequent blocks.

For profiling and benchmarking purposes, CPU profiling can be enabled via the '--cpu-profile' flag
which accepts a path for the resulting pprof file.
`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// app pre run
			if err := appPreRun(ctx, cmd); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			ctx.Logger.Info("Starting ABCI with Tendermint")

			sub := subFunc(ctx.Logger)
			log.SetSubscriber(sub)

			setPID(ctx)
			_, err := startInProcess(ctx, cdc, registry, appCreator, appStop, registerRoutesFn)
			if err != nil {
				tmos.Exit(err.Error())
			}
			return nil
		},
	}
	RegisterServerFlags(cmd)
	registerAppFlagFn(cmd)
	// add support for all Tendermint-specific command line options
	tcmd.AddNodeFlags(cmd)
	cmd.AddCommand(nodeModeCmd(ctx))
	return cmd
}

func startInProcess(ctx *Context, cdc *codec.CodecProxy, registry jsonpb.AnyResolver, appCreator AppCreator, appStop AppStop,
	registerRoutesFn func(restServer *lcd.RestServer)) (*node.Node, error) {

	cfg := ctx.Config
	home := cfg.RootDir
	//startInProcess hooker
	callHooker(FlagHookstartInProcess, ctx)

	traceWriterFile := viper.GetString(flagTraceStore)
	db, err := openDB(home)
	if err != nil {
		return nil, err
	}

	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return nil, err
	}

	app := appCreator(ctx.Logger, db, traceWriter)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil, err
	}

	// create & start tendermint node
	tmNode, err := node.NewNode(
		cfg,
		pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(app),
		node.DefaultGenesisDocProviderFunc(cfg),
		node.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		ctx.Logger.With("module", "node"),
	)
	if err != nil {
		return nil, err
	}

	app.SetOption(abci.RequestSetOption{
		Key:   "CheckChainID",
		Value: tmNode.ConsensusState().GetState().ChainID,
	})

	if clientSetter, ok := app.(interface {
		SetTmClient(client client.Client)
	}); ok {
		clientSetter.SetTmClient(local.New(tmNode))
	}

	ctx.Logger.Info("startInProcess",
		"ConsensusStateChainID", tmNode.ConsensusState().GetState().ChainID,
		"GenesisDocChainID", tmNode.GenesisDoc().ChainID,
	)
	if err := tmNode.Start(); err != nil {
		return nil, err
	}

	var cpuProfileCleanup func()

	if cpuProfile := viper.GetString(flagCPUProfile); cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			return nil, err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return nil, err
		}

		cpuProfileCleanup = func() {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			f.Close()
		}
	}

	TrapSignal(func() {
		if tmNode.IsRunning() {
			_ = tmNode.Stop()
		}
		appStop(app)

		if cpuProfileCleanup != nil {
			cpuProfileCleanup()
		}

		ctx.Logger.Info("exiting...")
	})

	if registerRoutesFn != nil {
		go lcd.StartRestServer(cdc, registry, registerRoutesFn, tmNode, viper.GetString(FlagListenAddr))
	}

	if cfg.GRPC.Enable {
		go grpc.StartGRPCServer(cdc, registry, app.(app2.ApplicationAdapter), cfg.GRPC, tmNode)
	}

	baseapp.SetGlobalMempool(tmNode.Mempool(), cfg.Mempool.SortTxByGp, cfg.Mempool.EnablePendingPool)

	if cfg.Mempool.EnablePendingPool {
		cliCtx := context.NewCLIContext().WithProxy(cdc)
		cliCtx.Client = local.New(tmNode)
		cliCtx.TrustNode = true
		accRetriever := types.NewAccountRetriever(cliCtx)
		tmNode.Mempool().SetAccountRetriever(accRetriever)
	}

	if parser, ok := app.(mempool.TxInfoParser); ok {
		tmNode.Mempool().SetTxInfoParser(parser)
	}

	// run forever (the node will not be returned)
	select {}
}

func StartRestWithNode(ctx *Context, cdc *codec.CodecProxy, blockStoreDir string,
	registry jsonpb.AnyResolver, appCreator AppCreator,
	registerRoutesFn func(restServer *lcd.RestServer)) (*node.Node, error) {

	cfg := ctx.Config
	home := cfg.RootDir
	////startInProcess hooker
	//callHooker(FlagHookstartInProcess, ctx)

	traceWriterFile := viper.GetString(flagTraceStore)
	db, err := openDB(home)
	if err != nil {
		return nil, err
	}

	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return nil, err
	}

	app := appCreator(ctx.Logger, db, traceWriter)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil, err
	}

	// create & start tendermint node
	tmNode, err := node.NewLRPNode(
		cfg,
		pvm.LoadFilePVEmptyState(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(app),
		node.DefaultGenesisDocProviderFunc(cfg),
		node.DefaultDBProvider,
		blockStoreDir,
		ctx.Logger.With("module", "node"),
	)
	if err != nil {
		return nil, err
	}

	app.SetOption(abci.RequestSetOption{
		Key:   "CheckChainID",
		Value: tmNode.ConsensusState().GetState().ChainID,
	})

	if registerRoutesFn != nil {
		go lcd.StartRestServer(cdc, registry, registerRoutesFn, tmNode, viper.GetString(FlagListenAddr))
	}

	// run forever (the node will not be returned)
	//select {}
	return tmNode, nil
}

// Use SetExternalPackageValue to set external package config value.
func SetExternalPackageValue(cmd *cobra.Command) {
	iavl.IavlCacheSize = viper.GetInt(iavl.FlagIavlCacheSize)
	tmiavl.IavlCacheInitRatio = viper.GetFloat64(tmiavl.FlagIavlCacheInitRatio)
	tmiavl.OutputModules, _ = cmd.Flags().GetStringToInt(tmiavl.FlagOutputModules)
	tmiavl.CommitIntervalHeight = viper.GetInt64(tmiavl.FlagIavlCommitIntervalHeight)
	tmiavl.MinCommitItemCount = viper.GetInt64(tmiavl.FlagIavlMinCommitItemCount)
	tmiavl.HeightOrphansCacheSize = viper.GetInt(tmiavl.FlagIavlHeightOrphansCacheSize)
	tmiavl.MaxCommittedHeightNum = viper.GetInt(tmiavl.FlagIavlMaxCommittedHeightNum)
	tmiavl.EnableAsyncCommit = viper.GetBool(tmiavl.FlagIavlEnableAsyncCommit)
	if viper.GetBool(tmiavl.FlagIavlDiscardFastStorage) {
		tmiavl.SetEnableFastStorage(false)
		viper.Set(tmiavl.FlagIavlEnableFastStorage, false)
	}
	system.EnableGid = viper.GetBool(system.FlagEnableGid)

	state.ApplyBlockPprofTime = viper.GetInt(state.FlagApplyBlockPprofTime)
	state.HomeDir = viper.GetString(cli.HomeFlag)

	abci.SetDisableABCIQueryMutex(viper.GetBool(abci.FlagDisableABCIQueryMutex))
	abci.SetDisableCheckTx(viper.GetBool(abci.FlagDisableCheckTx))

	tmtypes.DownloadDelta = viper.GetBool(tmtypes.FlagDownloadDDS)
	tmtypes.UploadDelta = viper.GetBool(tmtypes.FlagUploadDDS)
	tmtypes.FastQuery = viper.GetBool(tmtypes.FlagFastQuery)
	tmtypes.DeltaVersion = viper.GetInt(tmtypes.FlagDeltaVersion)
	tmtypes.BlockCompressType = viper.GetInt(tmtypes.FlagBlockCompressType)
	tmtypes.BlockCompressFlag = viper.GetInt(tmtypes.FlagBlockCompressFlag)
	tmtypes.BlockCompressThreshold = viper.GetInt(tmtypes.FlagBlockCompressThreshold)

	mpt.TrieCommitGap = viper.GetInt64(FlagCommitGapHeight)

	bcv0.MaxIntervalForFastSync = viper.GetInt64(FlagFastSyncGap)

	consensus.SetActiveVC(viper.GetBool(FlagActiveViewChange))

	tmtypes.EnableEventBlockTime = viper.GetBool(FlagEventBlockTime)
}
