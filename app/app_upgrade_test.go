package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	ibccommon "github.com/okex/exchain/libs/ibc-go/modules/core/common"

	"github.com/okex/exchain/libs/tendermint/libs/cli"
	"github.com/okex/exchain/libs/tm-db/common"
	"github.com/okex/exchain/x/wasm"
	wasmkeeper "github.com/okex/exchain/x/wasm/keeper"
	"github.com/spf13/viper"

	"github.com/gorilla/mux"
	"github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/store/mpt"
	cosmost "github.com/okex/exchain/libs/cosmos-sdk/store/types"
	capabilityModule "github.com/okex/exchain/libs/cosmos-sdk/x/capability"
	"github.com/okex/exchain/libs/cosmos-sdk/x/genutil"
	"github.com/okex/exchain/libs/system/trace"
	commonversion "github.com/okex/exchain/x/common/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/okex/exchain/app/ante"
	okexchaincodec "github.com/okex/exchain/app/codec"
	"github.com/okex/exchain/app/refund"
	okexchain "github.com/okex/exchain/app/types"
	bam "github.com/okex/exchain/libs/cosmos-sdk/baseapp"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/types/module"
	upgradetypes "github.com/okex/exchain/libs/cosmos-sdk/types/upgrade"
	"github.com/okex/exchain/libs/cosmos-sdk/version"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	"github.com/okex/exchain/libs/cosmos-sdk/x/bank"
	capabilitykeeper "github.com/okex/exchain/libs/cosmos-sdk/x/capability/keeper"
	capabilitytypes "github.com/okex/exchain/libs/cosmos-sdk/x/capability/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/crisis"
	"github.com/okex/exchain/libs/cosmos-sdk/x/mint"
	"github.com/okex/exchain/libs/cosmos-sdk/x/supply"
	"github.com/okex/exchain/libs/cosmos-sdk/x/upgrade"
	"github.com/okex/exchain/libs/iavl"
	ibctransfer "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer"
	ibctransferkeeper "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer/types"
	ibc "github.com/okex/exchain/libs/ibc-go/modules/core"
	ibcclient "github.com/okex/exchain/libs/ibc-go/modules/core/02-client"
	ibcporttypes "github.com/okex/exchain/libs/ibc-go/modules/core/05-port/types"
	ibchost "github.com/okex/exchain/libs/ibc-go/modules/core/24-host"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmos "github.com/okex/exchain/libs/tendermint/libs/os"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/okex/exchain/x/ammswap"
	"github.com/okex/exchain/x/dex"
	distr "github.com/okex/exchain/x/distribution"
	"github.com/okex/exchain/x/erc20"
	"github.com/okex/exchain/x/evidence"
	"github.com/okex/exchain/x/evm"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/okex/exchain/x/farm"
	"github.com/okex/exchain/x/gov"
	"github.com/okex/exchain/x/gov/keeper"
	"github.com/okex/exchain/x/order"
	"github.com/okex/exchain/x/params"
	"github.com/okex/exchain/x/slashing"
	"github.com/okex/exchain/x/staking"
	"github.com/okex/exchain/x/token"
)

var (
	_ upgradetypes.UpgradeModule = (*SimpleBaseUpgradeModule)(nil)

	test_prefix = "upgrade_module_"
)

type SimpleBaseUpgradeModule struct {
	t                  *testing.T
	h                  int64
	taskExecuteHeight  int64
	taskExecutedNotify func()
	appModule          module.AppModuleBasic
	storeKey           *sdk.KVStoreKey
}

func (b *SimpleBaseUpgradeModule) CommitFilter() *cosmost.StoreFilter {
	var ret cosmost.StoreFilter
	ret = func(module string, h int64, store cosmost.CommitKVStore) bool {
		if b.appModule.Name() != module {
			return false
		}
		if b.UpgradeHeight() == 0 {
			return true
		}
		if b.h == h {
			store.SetUpgradeVersion(h)
			return false
		}
		if b.h > h {
			return false
		}

		return true
	}
	return &ret
}

func (b *SimpleBaseUpgradeModule) PruneFilter() *cosmost.StoreFilter {
	var ret cosmost.StoreFilter
	ret = func(module string, h int64, store cosmost.CommitKVStore) bool {
		if b.appModule.Name() != module {
			return false
		}
		if b.UpgradeHeight() == 0 {
			return true
		}
		if b.h >= h {
			return false
		}

		return true
	}
	return &ret
}

func (b *SimpleBaseUpgradeModule) VersionFilter() *cosmost.VersionFilter {
	//todo ywmet
	return nil
}

func NewSimpleBaseUpgradeModule(t *testing.T, h int64, appModule module.AppModuleBasic, taskExecutedNotify func()) *SimpleBaseUpgradeModule {
	return &SimpleBaseUpgradeModule{t: t, h: h, appModule: appModule, taskExecutedNotify: taskExecutedNotify, taskExecuteHeight: h + 1}
}

func (b *SimpleBaseUpgradeModule) ModuleName() string {
	return b.appModule.Name()
}

func (b *SimpleBaseUpgradeModule) RegisterTask() upgradetypes.HeightTask {
	return upgradetypes.NewHeightTask(0, func(ctx sdk.Context) error {
		b.taskExecutedNotify()
		store := ctx.KVStore(b.storeKey)
		height := ctx.BlockHeight()
		require.Equal(b.t, b.taskExecuteHeight, height)
		store.Set([]byte(test_prefix+b.ModuleName()), []byte(strconv.Itoa(int(height))))
		return nil
	})
}

func (b *SimpleBaseUpgradeModule) UpgradeHeight() int64 {
	return b.h
}

func (b *SimpleBaseUpgradeModule) RegisterParam() params.ParamSet {
	return nil
}

var (
	_ module.AppModuleBasic = (*simpleDefaultAppModuleBasic)(nil)
)

type simpleDefaultAppModuleBasic struct {
	name string
}

func (s *simpleDefaultAppModuleBasic) Name() string {
	return s.name
}

func (s *simpleDefaultAppModuleBasic) RegisterCodec(c *codec.Codec) {}

func (s *simpleDefaultAppModuleBasic) DefaultGenesis() json.RawMessage { return nil }

func (s *simpleDefaultAppModuleBasic) ValidateGenesis(message json.RawMessage) error { return nil }

func (s *simpleDefaultAppModuleBasic) RegisterRESTRoutes(context context.CLIContext, router *mux.Router) {
	return
}

func (s *simpleDefaultAppModuleBasic) GetTxCmd(c *codec.Codec) *cobra.Command { return nil }

func (s *simpleDefaultAppModuleBasic) GetQueryCmd(c *codec.Codec) *cobra.Command { return nil }

var (
	_ module.AppModule = (*simpleAppModule)(nil)
)

type simpleAppModule struct {
	*SimpleBaseUpgradeModule
	*simpleDefaultAppModuleBasic
}

func newSimpleAppModule(t *testing.T, hh int64, name string, notify func()) *simpleAppModule {
	ret := &simpleAppModule{}
	ret.simpleDefaultAppModuleBasic = &simpleDefaultAppModuleBasic{name: name}
	ret.SimpleBaseUpgradeModule = NewSimpleBaseUpgradeModule(t, hh, ret, notify)
	return ret
}

func (s2 *simpleAppModule) InitGenesis(s sdk.Context, message json.RawMessage) []abci.ValidatorUpdate {
	return nil
}

func (s2 *simpleAppModule) ExportGenesis(s sdk.Context) json.RawMessage {
	return nil
}

func (s2 *simpleAppModule) RegisterInvariants(registry sdk.InvariantRegistry) { return }

func (s2 *simpleAppModule) Route() string {
	return ""
}

func (s2 *simpleAppModule) NewHandler() sdk.Handler { return nil }

func (s2 *simpleAppModule) QuerierRoute() string {
	return ""
}

func (s2 *simpleAppModule) NewQuerierHandler() sdk.Querier {
	return nil
}

func (s2 *simpleAppModule) BeginBlock(s sdk.Context, block abci.RequestBeginBlock) {
}

func (s2 *simpleAppModule) EndBlock(s sdk.Context, block abci.RequestEndBlock) []abci.ValidatorUpdate {
	return nil
}

func setupModuleBasics(bs ...module.AppModule) *module.Manager {
	basis := []module.AppModule{}
	basis = append(basis, bs...)
	return module.NewManager(
		basis...,
	)
}

type testSimApp struct {
	*OKExChainApp
	// the module manager
}

type TestSimAppOption func(a *testSimApp)
type MangerOption func(m *module.Manager)

func newTestOkcChainApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	skipUpgradeHeights map[int64]bool,
	invCheckPeriod uint,
	keys map[string]*sdk.KVStoreKey,
	ops ...TestSimAppOption,
) *testSimApp {
	logger.Info("Starting OEC",
		"GenesisHeight", tmtypes.GetStartBlockHeight(),
		"MercuryHeight", tmtypes.GetMercuryHeight(),
		"VenusHeight", tmtypes.GetVenusHeight(),
	)
	onceLog.Do(func() {
		iavl.SetLogger(logger.With("module", "iavl"))
		logStartingFlags(logger)
	})

	codecProxy, interfaceReg := okexchaincodec.MakeCodecSuit(ModuleBasics)

	// NOTE we use custom OKExChain transaction decoder that supports the sdk.Tx interface instead of sdk.StdTx
	bApp := bam.NewBaseApp(appName, logger, db, evm.TxDecoder(codecProxy))

	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetAppVersion(version.Version)
	bApp.SetStartLogHandler(trace.StartTxLog)
	bApp.SetEndLogHandler(trace.StopTxLog)

	bApp.SetInterfaceRegistry(interfaceReg)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)
	memKeys := sdk.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)

	ret := &testSimApp{}
	app := &OKExChainApp{
		BaseApp:        bApp,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
		subspaces:      make(map[string]params.Subspace),
		heightTasks:    make(map[int64]*upgradetypes.HeightTasks),
	}
	ret.OKExChainApp = app
	bApp.SetInterceptors(makeInterceptors())

	// init params keeper and subspaces
	app.ParamsKeeper = params.NewKeeper(codecProxy.GetCdc(), keys[params.StoreKey], tkeys[params.TStoreKey], logger)
	app.subspaces[auth.ModuleName] = app.ParamsKeeper.Subspace(auth.DefaultParamspace)
	app.subspaces[bank.ModuleName] = app.ParamsKeeper.Subspace(bank.DefaultParamspace)
	app.subspaces[staking.ModuleName] = app.ParamsKeeper.Subspace(staking.DefaultParamspace)
	app.subspaces[mint.ModuleName] = app.ParamsKeeper.Subspace(mint.DefaultParamspace)
	app.subspaces[distr.ModuleName] = app.ParamsKeeper.Subspace(distr.DefaultParamspace)
	app.subspaces[slashing.ModuleName] = app.ParamsKeeper.Subspace(slashing.DefaultParamspace)
	app.subspaces[gov.ModuleName] = app.ParamsKeeper.Subspace(gov.DefaultParamspace)
	app.subspaces[crisis.ModuleName] = app.ParamsKeeper.Subspace(crisis.DefaultParamspace)
	app.subspaces[evidence.ModuleName] = app.ParamsKeeper.Subspace(evidence.DefaultParamspace)
	app.subspaces[evm.ModuleName] = app.ParamsKeeper.Subspace(evm.DefaultParamspace)
	app.subspaces[token.ModuleName] = app.ParamsKeeper.Subspace(token.DefaultParamspace)
	app.subspaces[dex.ModuleName] = app.ParamsKeeper.Subspace(dex.DefaultParamspace)
	app.subspaces[order.ModuleName] = app.ParamsKeeper.Subspace(order.DefaultParamspace)
	app.subspaces[ammswap.ModuleName] = app.ParamsKeeper.Subspace(ammswap.DefaultParamspace)
	app.subspaces[farm.ModuleName] = app.ParamsKeeper.Subspace(farm.DefaultParamspace)
	app.subspaces[ibchost.ModuleName] = app.ParamsKeeper.Subspace(ibchost.ModuleName)
	app.subspaces[ibctransfertypes.ModuleName] = app.ParamsKeeper.Subspace(ibctransfertypes.ModuleName)
	app.subspaces[erc20.ModuleName] = app.ParamsKeeper.Subspace(erc20.DefaultParamspace)
	app.subspaces[wasm.ModuleName] = app.ParamsKeeper.Subspace(wasm.ModuleName)

	//proxy := codec.NewMarshalProxy(cc, cdc)
	app.marshal = codecProxy
	// use custom OKExChain account for contracts
	app.AccountKeeper = auth.NewAccountKeeper(
		codecProxy.GetCdc(), keys[auth.StoreKey], keys[mpt.StoreKey], app.subspaces[auth.ModuleName], okexchain.ProtoAccount,
	)

	bankKeeper := bank.NewBaseKeeperWithMarshal(
		&app.AccountKeeper, codecProxy, app.subspaces[bank.ModuleName], app.ModuleAccountAddrs(),
	)
	app.BankKeeper = &bankKeeper
	app.ParamsKeeper.SetBankKeeper(app.BankKeeper)
	app.SupplyKeeper = supply.NewKeeper(
		codecProxy.GetCdc(), keys[supply.StoreKey], &app.AccountKeeper, bank.NewBankKeeperAdapter(app.BankKeeper), maccPerms,
	)

	stakingKeeper := staking.NewKeeper(
		codecProxy, keys[staking.StoreKey], app.SupplyKeeper, app.subspaces[staking.ModuleName],
	)
	app.ParamsKeeper.SetStakingKeeper(stakingKeeper)
	app.MintKeeper = mint.NewKeeper(
		codecProxy.GetCdc(), keys[mint.StoreKey], app.subspaces[mint.ModuleName], &stakingKeeper,
		app.SupplyKeeper, auth.FeeCollectorName, farm.MintFarmingAccount,
	)
	app.DistrKeeper = distr.NewKeeper(
		codecProxy.GetCdc(), keys[distr.StoreKey], app.subspaces[distr.ModuleName], &stakingKeeper,
		app.SupplyKeeper, auth.FeeCollectorName, app.ModuleAccountAddrs(),
	)
	app.SlashingKeeper = slashing.NewKeeper(
		codecProxy.GetCdc(), keys[slashing.StoreKey], &stakingKeeper, app.subspaces[slashing.ModuleName],
	)
	app.CrisisKeeper = crisis.NewKeeper(
		app.subspaces[crisis.ModuleName], invCheckPeriod, app.SupplyKeeper, auth.FeeCollectorName,
	)
	app.UpgradeKeeper = upgrade.NewKeeper(skipUpgradeHeights, keys[upgrade.StoreKey], app.marshal.GetCdc())
	app.ParamsKeeper.RegisterSignal(evmtypes.SetEvmParamsNeedUpdate)
	app.EvmKeeper = evm.NewKeeper(
		app.marshal.GetCdc(), keys[evm.StoreKey], app.subspaces[evm.ModuleName], &app.AccountKeeper, app.SupplyKeeper, app.BankKeeper, &stakingKeeper, logger)
	(&bankKeeper).SetInnerTxKeeper(app.EvmKeeper)

	app.TokenKeeper = token.NewKeeper(app.BankKeeper, app.subspaces[token.ModuleName], auth.FeeCollectorName, app.SupplyKeeper,
		keys[token.StoreKey], keys[token.KeyLock], app.marshal.GetCdc(), false, &app.AccountKeeper)

	app.DexKeeper = dex.NewKeeper(auth.FeeCollectorName, app.SupplyKeeper, app.subspaces[dex.ModuleName], app.TokenKeeper, &stakingKeeper,
		app.BankKeeper, app.keys[dex.StoreKey], app.keys[dex.TokenPairStoreKey], app.marshal.GetCdc())

	app.OrderKeeper = order.NewKeeper(
		app.TokenKeeper, app.SupplyKeeper, app.DexKeeper, app.subspaces[order.ModuleName], auth.FeeCollectorName,
		app.keys[order.OrderStoreKey], app.marshal.GetCdc(), false, orderMetrics)

	app.SwapKeeper = ammswap.NewKeeper(app.SupplyKeeper, app.TokenKeeper, app.marshal.GetCdc(), app.keys[ammswap.StoreKey], app.subspaces[ammswap.ModuleName])

	app.FarmKeeper = farm.NewKeeper(auth.FeeCollectorName, app.SupplyKeeper, app.TokenKeeper, app.SwapKeeper, *app.EvmKeeper, app.subspaces[farm.StoreKey],
		app.keys[farm.StoreKey], app.marshal.GetCdc())

	// create evidence keeper with router
	evidenceKeeper := evidence.NewKeeper(
		codecProxy.GetCdc(), keys[evidence.StoreKey], app.subspaces[evidence.ModuleName], &app.StakingKeeper, app.SlashingKeeper,
	)
	evidenceRouter := evidence.NewRouter()
	evidenceKeeper.SetRouter(evidenceRouter)
	app.EvidenceKeeper = *evidenceKeeper

	// add capability keeper and ScopeToModule for ibc module
	app.CapabilityKeeper = capabilitykeeper.NewKeeper(codecProxy, keys[capabilitytypes.StoreKey], memKeys[capabilitytypes.MemStoreKey])
	scopedIBCKeeper := app.CapabilityKeeper.ScopeToModule(ibchost.ModuleName)
	scopedTransferKeeper := app.CapabilityKeeper.ScopeToModule(ibctransfertypes.ModuleName)
	// NOTE: the IBC mock keeper and application module is used only for testing core IBC. Do
	// note replicate if you do not need to test core IBC or light clients.
	scopedIBCMockKeeper := app.CapabilityKeeper.ScopeToModule("mock")

	v2keeper := ibc.NewKeeper(
		codecProxy, keys[ibchost.StoreKey], app.GetSubspace(ibchost.ModuleName), &stakingKeeper, app.UpgradeKeeper, &scopedIBCKeeper, interfaceReg,
	)
	v4Keeper := ibc.NewV4Keeper(v2keeper)
	facadedKeeper := ibc.NewFacadedKeeper(v2keeper)
	facadedKeeper.RegisterKeeper(ibccommon.DefaultFactory(tmtypes.HigherThanVenus4, ibc.IBCV4, v4Keeper))
	app.IBCKeeper = facadedKeeper

	// Create Transfer Keepers
	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		codecProxy, keys[ibctransfertypes.StoreKey], app.GetSubspace(ibctransfertypes.ModuleName),
		app.IBCKeeper.V2Keeper.ChannelKeeper, &app.IBCKeeper.V2Keeper.PortKeeper,
		app.SupplyKeeper, supply.NewSupplyKeeperAdapter(app.SupplyKeeper), scopedTransferKeeper, interfaceReg,
	)
	ibctransfertypes.SetMarshal(codecProxy)

	app.Erc20Keeper = erc20.NewKeeper(app.marshal.GetCdc(), app.keys[erc20.ModuleName], app.subspaces[erc20.ModuleName],
		app.AccountKeeper, app.SupplyKeeper, app.BankKeeper, app.EvmKeeper, app.TransferKeeper)

	// register the proposal types
	// 3.register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(params.RouterKey, params.NewParamChangeProposalHandler(&app.ParamsKeeper)).
		AddRoute(distr.RouterKey, distr.NewDistributionProposalHandler(app.DistrKeeper)).
		AddRoute(dex.RouterKey, dex.NewProposalHandler(&app.DexKeeper)).
		AddRoute(farm.RouterKey, farm.NewManageWhiteListProposalHandler(&app.FarmKeeper)).
		AddRoute(evm.RouterKey, evm.NewManageContractDeploymentWhitelistProposalHandler(app.EvmKeeper)).
		AddRoute(mint.RouterKey, mint.NewManageTreasuresProposalHandler(&app.MintKeeper)).
		AddRoute(ibchost.RouterKey, ibcclient.NewClientUpdateProposalHandler(app.IBCKeeper.V2Keeper.ClientKeeper)).
		AddRoute(erc20.RouterKey, erc20.NewProposalHandler(&app.Erc20Keeper))
	govProposalHandlerRouter := keeper.NewProposalHandlerRouter()
	govProposalHandlerRouter.AddRoute(params.RouterKey, &app.ParamsKeeper).
		AddRoute(dex.RouterKey, &app.DexKeeper).
		AddRoute(farm.RouterKey, &app.FarmKeeper).
		AddRoute(evm.RouterKey, app.EvmKeeper).
		AddRoute(mint.RouterKey, &app.MintKeeper).
		AddRoute(erc20.RouterKey, &app.Erc20Keeper)
	app.GovKeeper = gov.NewKeeper(
		app.marshal.GetCdc(), app.keys[gov.StoreKey], app.ParamsKeeper, app.subspaces[gov.DefaultParamspace],
		app.SupplyKeeper, &stakingKeeper, gov.DefaultParamspace, govRouter,
		app.BankKeeper, govProposalHandlerRouter, auth.FeeCollectorName,
	)
	app.ParamsKeeper.SetGovKeeper(app.GovKeeper)
	app.DexKeeper.SetGovKeeper(app.GovKeeper)
	app.FarmKeeper.SetGovKeeper(app.GovKeeper)
	app.EvmKeeper.SetGovKeeper(app.GovKeeper)
	app.MintKeeper.SetGovKeeper(app.GovKeeper)
	app.Erc20Keeper.SetGovKeeper(app.GovKeeper)

	// Set EVM hooks
	app.EvmKeeper.SetHooks(evm.NewLogProcessEvmHook(erc20.NewSendToIbcEventHandler(app.Erc20Keeper),
		erc20.NewSendNative20ToIbcEventHandler(app.Erc20Keeper)))
	// Set IBC hooks
	app.TransferKeeper = *app.TransferKeeper.SetHooks(erc20.NewIBCTransferHooks(app.Erc20Keeper))
	transferModule := ibctransfer.NewAppModule(app.TransferKeeper, codecProxy)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := ibcporttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transferModule)
	//ibcRouter.AddRoute(ibcmock.ModuleName, mockModule)
	app.IBCKeeper.V2Keeper.SetRouter(ibcRouter)

	// register the staking hooks
	// NOTE: stakingKeeper above is passed by reference, so that it will contain these hooks
	app.StakingKeeper = *stakingKeeper.SetHooks(
		staking.NewMultiStakingHooks(app.DistrKeeper.Hooks(), app.SlashingKeeper.Hooks()),
	)

	homeDir := viper.GetString(cli.HomeFlag)
	wasmDir := filepath.Join(homeDir, "wasm")
	wasmConfig, err := wasm.ReadWasmConfig()
	if err != nil {
		panic(fmt.Sprintf("error while reading wasm config: %s", err))
	}

	// The last arguments can contain custom message handlers, and custom query handlers,
	// if we want to allow any custom callbacks
	supportedFeatures := wasm.SupportedFeatures
	app.WasmKeeper = wasm.NewKeeper(
		app.marshal,
		keys[wasm.StoreKey],
		app.subspaces[wasm.ModuleName],
		&app.AccountKeeper,
		bank.NewBankKeeperAdapter(app.BankKeeper),
		app.IBCKeeper.V2Keeper.ChannelKeeper,
		&app.IBCKeeper.V2Keeper.PortKeeper,
		nil,
		app.TransferKeeper,
		app.MsgServiceRouter(),
		app.GRPCQueryRouter(),
		wasmDir,
		wasmConfig,
		supportedFeatures,
	)

	app.mm = module.NewManager(
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app.BaseApp.DeliverTx),
		auth.NewAppModule(app.AccountKeeper),
		bank.NewAppModule(app.BankKeeper, app.AccountKeeper, app.SupplyKeeper),
		crisis.NewAppModule(&app.CrisisKeeper),
		supply.NewAppModule(app.SupplyKeeper, app.AccountKeeper),
		gov.NewAppModule(app.GovKeeper, app.SupplyKeeper),
		mint.NewAppModule(app.MintKeeper),
		slashing.NewAppModule(app.SlashingKeeper, app.AccountKeeper, app.StakingKeeper),
		distr.NewAppModule(app.DistrKeeper, app.SupplyKeeper),
		staking.NewAppModule(app.StakingKeeper, app.AccountKeeper, app.SupplyKeeper),
		evidence.NewAppModule(app.EvidenceKeeper),
		evm.NewAppModule(app.EvmKeeper, &app.AccountKeeper),
		token.NewAppModule(commonversion.ProtocolVersionV0, app.TokenKeeper, app.SupplyKeeper),
		dex.NewAppModule(commonversion.ProtocolVersionV0, app.DexKeeper, app.SupplyKeeper),
		order.NewAppModule(commonversion.ProtocolVersionV0, app.OrderKeeper, app.SupplyKeeper),
		ammswap.NewAppModule(app.SwapKeeper),
		farm.NewAppModule(app.FarmKeeper),
		params.NewAppModule(app.ParamsKeeper),
		// ibc
		ibc.NewAppModule(app.IBCKeeper),
		capabilityModule.NewAppModule(codecProxy, *app.CapabilityKeeper),
		transferModule,
		erc20.NewAppModule(app.Erc20Keeper),
		wasm.NewAppModule(*app.marshal, &app.WasmKeeper),
	)

	for _, opt := range ops {
		opt(ret)
	}

	// create the simulation manager and define the order of the modules for deterministic simulations
	//
	// NOTE: this is not required apps that don't use the simulator for fuzz testing
	// transactions
	app.sm = module.NewSimulationManager(
		auth.NewAppModule(app.AccountKeeper),
		bank.NewAppModule(app.BankKeeper, app.AccountKeeper, app.SupplyKeeper),
		supply.NewAppModule(app.SupplyKeeper, app.AccountKeeper),
		gov.NewAppModule(app.GovKeeper, app.SupplyKeeper),
		mint.NewAppModule(app.MintKeeper),
		staking.NewAppModule(app.StakingKeeper, app.AccountKeeper, app.SupplyKeeper),
		distr.NewAppModule(app.DistrKeeper, app.SupplyKeeper),
		slashing.NewAppModule(app.SlashingKeeper, app.AccountKeeper, app.StakingKeeper),
		params.NewAppModule(app.ParamsKeeper), // NOTE: only used for simulation to generate randomized param change proposals
		ibc.NewAppModule(app.IBCKeeper),
		wasm.NewAppModule(*app.marshal, &app.WasmKeeper),
	)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)
	app.MountMemoryStores(memKeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetAnteHandler(ante.NewAnteHandler(app.AccountKeeper, app.EvmKeeper, app.SupplyKeeper, validateMsgHook(app.OrderKeeper), wasmkeeper.HandlerOption{
		WasmConfig:        &wasmConfig,
		TXCounterStoreKey: keys[wasm.StoreKey],
	}, app.IBCKeeper, app.StakingKeeper, app.ParamsKeeper))
	app.SetEndBlocker(app.EndBlocker)
	app.SetGasRefundHandler(refund.NewGasRefundHandler(app.AccountKeeper, app.SupplyKeeper, app.EvmKeeper))
	app.SetAccNonceHandler(NewAccNonceHandler(app.AccountKeeper))
	app.SetEvmSysContractAddressHandler(NewEvmSysContractAddressHandler(app.EvmKeeper))
	app.SetUpdateFeeCollectorAccHandler(updateFeeCollectorHandler(app.BankKeeper, app.SupplyKeeper))
	app.SetParallelTxLogHandlers(fixLogForParallelTxHandler(app.EvmKeeper))
	app.SetEvmWatcherCollector(app.EvmKeeper.Watcher.Collect)

	if loadLatest {
		err := app.LoadLatestVersion(app.keys[bam.MainStoreKey])
		if err != nil {
			tmos.Exit(err.Error())
		}
	}

	app.ScopedIBCKeeper = scopedIBCKeeper
	app.ScopedTransferKeeper = scopedTransferKeeper

	// NOTE: the IBC mock keeper and application module is used only for testing core IBC. Do
	// note replicate if you do not need to test core IBC or light clients.
	app.ScopedIBCMockKeeper = scopedIBCMockKeeper

	return ret
}

func newTestSimApp(name string, logger log.Logger, db dbm.DB, txDecoder sdk.TxDecoder, keys map[string]*sdk.KVStoreKey, ops ...TestSimAppOption) *testSimApp {
	return newTestOkcChainApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0, keys, ops...)
}

type UpgradeCase struct {
	name     string
	upgradeH int64
}

func createCases(moduleCount int, beginHeight int) []UpgradeCase {
	ret := make([]UpgradeCase, moduleCount)
	for i := 0; i < moduleCount; i++ {
		ret[i] = UpgradeCase{
			name:     "m_" + strconv.Itoa(i),
			upgradeH: int64(beginHeight + i),
		}
	}
	return ret
}

func newRecordMemDB() *RecordMemDB {
	ret := &RecordMemDB{}
	ret.db = dbm.NewMemDB()
	return ret
}

func TestUpgradeWithConcreteHeight(t *testing.T) {
	db := newRecordMemDB()

	cases := createCases(5, 10)
	m := make(map[string]int)
	count := 0
	maxHeight := int64(0)

	modules := make([]*simpleAppModule, 0)
	for _, ca := range cases {
		c := ca
		m[c.name] = 0
		if maxHeight < c.upgradeH {
			maxHeight = c.upgradeH
		}
		modules = append(modules, newSimpleAppModule(t, c.upgradeH, c.name, func() {
			m[c.name]++
			count++
		}))
	}

	app := setupTestApp(db, cases, modules)

	genesisState := ModuleBasics.DefaultGenesis()
	stateBytes, err := codec.MarshalJSONIndent(app.Codec(), genesisState)
	require.NoError(t, err)
	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app.Commit(abci.RequestCommit{})

	for i := int64(2); i < maxHeight+5; i++ {
		header := abci.Header{Height: i}
		app.BeginBlock(abci.RequestBeginBlock{Header: header})
		app.Commit(abci.RequestCommit{})
	}
	for _, v := range m {
		require.Equal(t, 1, v)
	}
	require.Equal(t, count, len(cases))
}

func setupTestApp(db dbm.DB, cases []UpgradeCase, modules []*simpleAppModule) *testSimApp {
	keys := createKeysByCases(cases)
	for _, m := range modules {
		m.storeKey = keys[m.Name()]
	}
	app := newTestSimApp("demo", log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, func(txBytes []byte, height ...int64) (sdk.Tx, error) {
		return nil, nil
	}, keys, func(a *testSimApp) {
		for _, m := range modules {
			a.mm.Modules[m.Name()] = m
			a.mm.OrderBeginBlockers = append(a.mm.OrderEndBlockers, m.Name())
			a.mm.OrderEndBlockers = append(a.mm.OrderEndBlockers, m.Name())
			a.mm.OrderInitGenesis = append(a.mm.OrderInitGenesis, m.Name())
			a.mm.OrderExportGenesis = append(a.mm.OrderExportGenesis, m.Name())
		}
	}, func(a *testSimApp) {
		a.setupUpgradeModules(false)
	})
	return app
}

func createKeysByCases(caseas []UpgradeCase) map[string]*sdk.KVStoreKey {
	caseKeys := make([]string, 0)
	for _, c := range caseas {
		caseKeys = append(caseKeys, c.name)
	}
	caseKeys = append(caseKeys, bam.MainStoreKey, auth.StoreKey, staking.StoreKey,
		supply.StoreKey, mint.StoreKey, distr.StoreKey, slashing.StoreKey,
		gov.StoreKey, params.StoreKey, upgrade.StoreKey, evidence.StoreKey,
		evm.StoreKey, token.StoreKey, token.KeyLock, dex.StoreKey, dex.TokenPairStoreKey,
		order.OrderStoreKey, ammswap.StoreKey, farm.StoreKey, ibctransfertypes.StoreKey, capabilitytypes.StoreKey,
		ibchost.StoreKey,
		erc20.StoreKey, wasm.StoreKey)
	keys := sdk.NewKVStoreKeys(
		caseKeys...,
	)
	return keys
}

// /
type RecordMemDB struct {
	db *dbm.MemDB
	common.PlaceHolder
}

func (d *RecordMemDB) Get(bytes []byte) ([]byte, error) {
	return d.db.Get(bytes)
}

func (d *RecordMemDB) GetUnsafeValue(key []byte, processor dbm.UnsafeValueProcessor) (interface{}, error) {
	return d.db.GetUnsafeValue(key, processor)
}

func (d *RecordMemDB) Has(key []byte) (bool, error) {
	return d.db.Has(key)
}

func (d *RecordMemDB) SetSync(bytes []byte, bytes2 []byte) error {
	return d.db.SetSync(bytes, bytes2)
}

func (d *RecordMemDB) Delete(bytes []byte) error {
	return d.db.Delete(bytes)
}

func (d *RecordMemDB) DeleteSync(bytes []byte) error {
	return d.db.DeleteSync(bytes)
}

func (d *RecordMemDB) Iterator(start, end []byte) (dbm.Iterator, error) {
	return d.db.Iterator(start, end)
}

func (d *RecordMemDB) ReverseIterator(start, end []byte) (dbm.Iterator, error) {
	return d.db.ReverseIterator(start, end)
}

func (d *RecordMemDB) Close() error {
	return d.db.Close()
}

func (d *RecordMemDB) NewBatch() dbm.Batch {
	return d.db.NewBatch()
}

func (d *RecordMemDB) Print() error {
	return d.db.Print()
}

func (d *RecordMemDB) Stats() map[string]string {
	return d.db.Stats()
}

func (d *RecordMemDB) Set(key []byte, value []byte) error {
	return d.db.Set(key, value)
}

func TestErc20InitGenesis(t *testing.T) {
	db := newRecordMemDB()

	cases := createCases(1, 1)
	m := make(map[string]int)
	count := 0
	maxHeight := int64(0)
	veneus1H := 10
	tmtypes.UnittestOnlySetMilestoneVenus1Height(10)

	modules := make([]*simpleAppModule, 0)
	for _, ca := range cases {
		c := ca
		m[c.name] = 0
		if maxHeight < c.upgradeH {
			maxHeight = c.upgradeH
		}
		modules = append(modules, newSimpleAppModule(t, c.upgradeH, c.name, func() {
			m[c.name]++
			count++
		}))
	}

	app := setupTestApp(db, cases, modules)

	genesisState := ModuleBasics.DefaultGenesis()
	stateBytes, err := codec.MarshalJSONIndent(app.Codec(), genesisState)
	require.NoError(t, err)
	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app.Commit(abci.RequestCommit{})

	for i := int64(2); i < int64(veneus1H+5); i++ {
		header := abci.Header{Height: i}
		app.BeginBlock(abci.RequestBeginBlock{Header: header})
		if i <= int64(veneus1H) {
			_, found := app.Erc20Keeper.GetImplementTemplateContract(app.GetDeliverStateCtx())
			require.Equal(t, found, false)
			_, found = app.Erc20Keeper.GetProxyTemplateContract(app.GetDeliverStateCtx())
			require.Equal(t, found, false)
		}
		if i >= int64(veneus1H+2) {
			_, found := app.Erc20Keeper.GetImplementTemplateContract(app.GetDeliverStateCtx())
			require.Equal(t, found, true)
			_, found = app.Erc20Keeper.GetProxyTemplateContract(app.GetDeliverStateCtx())
			require.Equal(t, found, true)
		}
		app.Commit(abci.RequestCommit{})

	}

}
