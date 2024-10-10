package simapp

import (
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"

	"github.com/okex/exchain/app/ante"
	okexchaincodec "github.com/okex/exchain/app/codec"
	appconfig "github.com/okex/exchain/app/config"
	"github.com/okex/exchain/app/refund"
	ethermint "github.com/okex/exchain/app/types"
	okexchain "github.com/okex/exchain/app/types"
	"github.com/okex/exchain/app/utils/sanity"
	"github.com/okex/exchain/libs/cosmos-sdk/baseapp"
	bam "github.com/okex/exchain/libs/cosmos-sdk/baseapp"
	"github.com/okex/exchain/libs/cosmos-sdk/client"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	"github.com/okex/exchain/libs/cosmos-sdk/server"
	"github.com/okex/exchain/libs/cosmos-sdk/simapp"
	"github.com/okex/exchain/libs/cosmos-sdk/store/mpt"
	"github.com/okex/exchain/libs/cosmos-sdk/store/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/types/module"
	upgradetypes "github.com/okex/exchain/libs/cosmos-sdk/types/upgrade"
	"github.com/okex/exchain/libs/cosmos-sdk/version"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	authante "github.com/okex/exchain/libs/cosmos-sdk/x/auth/ante"
	authtypes "github.com/okex/exchain/libs/cosmos-sdk/x/auth/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/bank"
	capabilityModule "github.com/okex/exchain/libs/cosmos-sdk/x/capability"
	capabilitykeeper "github.com/okex/exchain/libs/cosmos-sdk/x/capability/keeper"
	capabilitytypes "github.com/okex/exchain/libs/cosmos-sdk/x/capability/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/crisis"
	"github.com/okex/exchain/libs/cosmos-sdk/x/mint"
	govclient "github.com/okex/exchain/libs/cosmos-sdk/x/mint/client"
	"github.com/okex/exchain/libs/cosmos-sdk/x/params/subspace"
	"github.com/okex/exchain/libs/cosmos-sdk/x/supply"
	"github.com/okex/exchain/libs/cosmos-sdk/x/upgrade"
	icacontroller "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/controller"
	icacontrollerkeeper "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/controller/keeper"
	icacontrollertypes "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/controller/types"
	icahost "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/host"
	icahostkeeper "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/host/keeper"
	icahosttypes "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/okex/exchain/libs/ibc-go/modules/apps/27-interchain-accounts/types"
	ibcfee "github.com/okex/exchain/libs/ibc-go/modules/apps/29-fee"
	ibcfeekeeper "github.com/okex/exchain/libs/ibc-go/modules/apps/29-fee/keeper"
	ibcfeetypes "github.com/okex/exchain/libs/ibc-go/modules/apps/29-fee/types"
	"github.com/okex/exchain/libs/ibc-go/modules/apps/common"
	ibctransfer "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer"
	ibctransferkeeper "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer/types"
	ibc "github.com/okex/exchain/libs/ibc-go/modules/core"
	ibcclient "github.com/okex/exchain/libs/ibc-go/modules/core/02-client"
	ibcclienttypes "github.com/okex/exchain/libs/ibc-go/modules/core/02-client/types"
	ibcporttypes "github.com/okex/exchain/libs/ibc-go/modules/core/05-port/types"
	ibchost "github.com/okex/exchain/libs/ibc-go/modules/core/24-host"
	ibccommon "github.com/okex/exchain/libs/ibc-go/modules/core/common"
	ibckeeper "github.com/okex/exchain/libs/ibc-go/modules/core/keeper"
	"github.com/okex/exchain/libs/ibc-go/testing/mock"
	"github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/capability"
	"github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/core"
	evm2 "github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/evm"
	"github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/fee"
	ica2 "github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/ica"
	staking2 "github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/staking"
	"github.com/okex/exchain/libs/ibc-go/testing/simapp/adapter/transfer"
	"github.com/okex/exchain/libs/system/trace"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/libs/cli"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	tmos "github.com/okex/exchain/libs/tendermint/libs/os"
	tmtypes "github.com/okex/exchain/libs/tendermint/types"
	dbm "github.com/okex/exchain/libs/tm-db"
	"github.com/okex/exchain/x/ammswap"
	"github.com/okex/exchain/x/common/monitor"
	commonversion "github.com/okex/exchain/x/common/version"
	"github.com/okex/exchain/x/dex"
	dexclient "github.com/okex/exchain/x/dex/client"
	distr "github.com/okex/exchain/x/distribution"
	"github.com/okex/exchain/x/erc20"
	erc20client "github.com/okex/exchain/x/erc20/client"
	"github.com/okex/exchain/x/evidence"
	"github.com/okex/exchain/x/evm"
	evmclient "github.com/okex/exchain/x/evm/client"
	evmtypes "github.com/okex/exchain/x/evm/types"
	"github.com/okex/exchain/x/farm"
	farmclient "github.com/okex/exchain/x/farm/client"
	"github.com/okex/exchain/x/genutil"
	"github.com/okex/exchain/x/gov"
	"github.com/okex/exchain/x/gov/keeper"
	"github.com/okex/exchain/x/icamauth"
	icamauthkeeper "github.com/okex/exchain/x/icamauth/keeper"
	icamauthtypes "github.com/okex/exchain/x/icamauth/types"
	"github.com/okex/exchain/x/order"
	"github.com/okex/exchain/x/params"
	paramsclient "github.com/okex/exchain/x/params/client"
	"github.com/okex/exchain/x/slashing"
	"github.com/okex/exchain/x/staking"
	"github.com/okex/exchain/x/token"
	"github.com/okex/exchain/x/wasm"
	wasmclient "github.com/okex/exchain/x/wasm/client"
	wasmkeeper "github.com/okex/exchain/x/wasm/keeper"
)

func init() {
	// set the address prefixes
	config := sdk.GetConfig()
	config.SetCoinType(60)
	okexchain.SetBech32Prefixes(config)
	okexchain.SetBip44CoinType(config)
}

const (
	appName = "OKExChain"
)
const (
	MockFeePort string = mock.ModuleName + ibcfeetypes.ModuleName
)

var (
	// DefaultCLIHome sets the default home directories for the application CLI
	DefaultCLIHome = os.ExpandEnv("$HOME/.exchaincli")

	// DefaultNodeHome sets the folder where the applcation data and configuration will be stored
	DefaultNodeHome = os.ExpandEnv("$HOME/.exchaind")

	// ModuleBasics defines the module BasicManager is in charge of setting up basic,
	// non-dependant module elements, such as codec registration
	// and genesis verification.
	ModuleBasics = module.NewBasicManager(
		auth.AppModuleBasic{},
		supply.AppModuleBasic{},
		genutil.AppModuleBasic{},
		bank.AppModuleBasic{},
		staking.AppModuleBasic{},
		mint.AppModuleBasic{},
		distr.AppModuleBasic{},
		gov.NewAppModuleBasic(
			paramsclient.ProposalHandler,
			distr.CommunityPoolSpendProposalHandler,
			distr.ChangeDistributionTypeProposalHandler,
			distr.WithdrawRewardEnabledProposalHandler,
			distr.RewardTruncatePrecisionProposalHandler,
			dexclient.DelistProposalHandler, farmclient.ManageWhiteListProposalHandler,
			evmclient.ManageContractDeploymentWhitelistProposalHandler,
			evmclient.ManageContractBlockedListProposalHandler,
			evmclient.ManageContractMethodGuFactorProposalHandler,
			evmclient.ManageContractMethodBlockedListProposalHandler,
			evmclient.ManageSysContractAddressProposalHandler,
			evmclient.ManageContractByteCodeProposalHandler,
			govclient.ManageTreasuresProposalHandler,
			govclient.ModifyNextBlockUpdateProposalHandler,
			erc20client.TokenMappingProposalHandler,
			erc20client.ProxyContractRedirectHandler,
			wasmclient.MigrateContractProposalHandler,
			wasmclient.UpdateContractAdminProposalHandler,
			wasmclient.PinCodesProposalHandler,
			wasmclient.UnpinCodesProposalHandler,
			wasmclient.UpdateDeploymentWhitelistProposalHandler,
			wasmclient.UpdateWASMContractMethodBlockedListProposalHandler,
			wasmclient.GetCmdExtraProposal,
		),
		params.AppModuleBasic{},
		crisis.AppModuleBasic{},
		slashing.AppModuleBasic{},
		evidence.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evm.AppModuleBasic{},
		token.AppModuleBasic{},
		dex.AppModuleBasic{},
		order.AppModuleBasic{},
		ammswap.AppModuleBasic{},
		farm.AppModuleBasic{},
		capabilityModule.AppModuleBasic{},
		core.CoreModule{},
		capability.CapabilityModuleAdapter{},
		transfer.TransferModule{},
		erc20.AppModuleBasic{},
		mock.AppModuleBasic{},
		wasm.AppModuleBasic{},
		ica2.TestICAModuleBaisc{},
		fee.TestFeeAppModuleBaisc{},
		icamauth.AppModuleBasic{},
	)

	// module account permissions
	maccPerms = map[string][]string{
		auth.FeeCollectorName:       nil,
		distr.ModuleName:            nil,
		mint.ModuleName:             {supply.Minter},
		staking.BondedPoolName:      {supply.Burner, supply.Staking},
		staking.NotBondedPoolName:   {supply.Burner, supply.Staking},
		gov.ModuleName:              nil,
		token.ModuleName:            {supply.Minter, supply.Burner},
		dex.ModuleName:              nil,
		order.ModuleName:            nil,
		ammswap.ModuleName:          {supply.Minter, supply.Burner},
		farm.ModuleName:             nil,
		farm.YieldFarmingAccount:    nil,
		farm.MintFarmingAccount:     {supply.Burner},
		ibctransfertypes.ModuleName: {authtypes.Minter, authtypes.Burner},
		erc20.ModuleName:            {authtypes.Minter, authtypes.Burner},
		ibcfeetypes.ModuleName:      nil,
		icatypes.ModuleName:         nil,
		mock.ModuleName:             nil,
	}

	GlobalGpIndex = GasPriceIndex{}

	onceLog sync.Once
)

type GasPriceIndex struct {
	RecommendGp *big.Int `json:"recommend-gp"`
}

var _ simapp.App = (*SimApp)(nil)

// SimApp implements an extended ABCI application. It is an application
// that may process transactions through Ethereum's EVM running atop of
// Tendermint consensus.
type SimApp struct {
	*bam.BaseApp

	txconfig client.TxConfig

	CodecProxy *codec.CodecProxy

	invCheckPeriod uint

	// keys to access the substores
	keys    map[string]*sdk.KVStoreKey
	tkeys   map[string]*sdk.TransientStoreKey
	memKeys map[string]*sdk.MemoryStoreKey

	// subspaces
	subspaces map[string]params.Subspace

	// keepers
	AccountKeeper  auth.AccountKeeper
	BankKeeper     *bank.BankKeeperAdapter
	SupplyKeeper   *supply.KeeperAdapter
	StakingKeeper  staking.Keeper
	SlashingKeeper slashing.Keeper
	MintKeeper     mint.Keeper
	DistrKeeper    distr.Keeper
	GovKeeper      gov.Keeper
	CrisisKeeper   crisis.Keeper
	UpgradeKeeper  upgrade.Keeper
	ParamsKeeper   params.Keeper
	EvidenceKeeper evidence.Keeper
	EvmKeeper      *evm.Keeper
	TokenKeeper    token.Keeper
	DexKeeper      dex.Keeper
	OrderKeeper    order.Keeper
	SwapKeeper     ammswap.Keeper
	FarmKeeper     farm.Keeper
	wasmKeeper     wasm.Keeper

	// the module manager
	mm *module.Manager

	// simulation manager
	sm *module.SimulationManager

	blockGasPrice []*big.Int

	configurator module.Configurator
	// ibc
	ScopedIBCKeeper      capabilitykeeper.ScopedKeeper
	ScopedTransferKeeper capabilitykeeper.ScopedKeeper
	ScopedIBCMockKeeper  capabilitykeeper.ScopedKeeper
	ScopedICAMockKeeper  capabilitykeeper.ScopedKeeper
	ScopedICAHostKeeper  capabilitykeeper.ScopedKeeper
	TransferKeeper       ibctransferkeeper.Keeper
	CapabilityKeeper     *capabilitykeeper.Keeper
	IBCKeeper            *ibc.Keeper // IBC Keeper must be a pointer in the app, so we can SetRouter on it correctly
	marshal              *codec.CodecProxy
	heightTasks          map[int64]*upgradetypes.HeightTasks
	Erc20Keeper          erc20.Keeper

	ibcScopeKeep capabilitykeeper.ScopedKeeper
	WasmHandler  wasmkeeper.HandlerOption

	IBCFeeKeeper        ibcfeekeeper.Keeper
	ICAMauthKeeper      icamauthkeeper.Keeper
	ICAControllerKeeper icacontrollerkeeper.Keeper
	ICAHostKeeper       icahostkeeper.Keeper
	ICAAuthModule       mock.IBCModule

	FeeMockModule mock.IBCModule
}

func NewSimApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	skipUpgradeHeights map[int64]bool,
	invCheckPeriod uint,
	baseAppOptions ...func(*bam.BaseApp),
) *SimApp {
	logger.Info("Starting OEC",
		"GenesisHeight", tmtypes.GetStartBlockHeight(),
		"MercuryHeight", tmtypes.GetMercuryHeight(),
		"VenusHeight", tmtypes.GetVenusHeight(),
	)
	//onceLog.Do(func() {
	//	iavl.SetLogger(logger.With("module", "iavl"))
	//	logStartingFlags(logger)
	//})

	codecProxy, interfaceReg := okexchaincodec.MakeCodecSuit(ModuleBasics)

	// NOTE we use custom OKExChain transaction decoder that supports the sdk.Tx interface instead of sdk.StdTx
	bApp := bam.NewBaseApp(appName, logger, db, evm.TxDecoder(codecProxy), baseAppOptions...)

	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetAppVersion(version.Version)
	bApp.SetStartLogHandler(trace.StartTxLog)
	bApp.SetEndLogHandler(trace.StopTxLog)

	bApp.SetInterfaceRegistry(interfaceReg)

	keys := sdk.NewKVStoreKeys(
		bam.MainStoreKey, auth.StoreKey, staking.StoreKey,
		supply.StoreKey, mint.StoreKey, distr.StoreKey, slashing.StoreKey,
		gov.StoreKey, params.StoreKey, upgrade.StoreKey, evidence.StoreKey,
		evm.StoreKey, token.StoreKey, token.KeyLock, dex.StoreKey, dex.TokenPairStoreKey,
		order.OrderStoreKey, ammswap.StoreKey, farm.StoreKey, ibctransfertypes.StoreKey, capabilitytypes.StoreKey,
		ibchost.StoreKey,
		erc20.StoreKey,
		mpt.StoreKey, wasm.StoreKey,
		icacontrollertypes.StoreKey, icahosttypes.StoreKey, ibcfeetypes.StoreKey,
		icamauthtypes.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)
	memKeys := sdk.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)

	app := &SimApp{
		BaseApp:        bApp,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
		subspaces:      make(map[string]params.Subspace),
		heightTasks:    make(map[int64]*upgradetypes.HeightTasks),
		memKeys:        memKeys,
	}
	app.CodecProxy = codecProxy
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
	app.subspaces[icacontrollertypes.SubModuleName] = app.ParamsKeeper.Subspace(icacontrollertypes.SubModuleName)
	app.subspaces[icahosttypes.SubModuleName] = app.ParamsKeeper.Subspace(icahosttypes.SubModuleName)
	app.subspaces[ibcfeetypes.ModuleName] = app.ParamsKeeper.Subspace(ibcfeetypes.ModuleName)

	//proxy := codec.NewMarshalProxy(cc, cdc)
	app.marshal = codecProxy
	// use custom OKExChain account for contracts
	app.AccountKeeper = auth.NewAccountKeeper(
		codecProxy.GetCdc(), keys[auth.StoreKey], keys[mpt.StoreKey], app.subspaces[auth.ModuleName], okexchain.ProtoAccount,
	)

	bankKeeper := bank.NewBaseKeeperWithMarshal(
		&app.AccountKeeper, codecProxy, app.subspaces[bank.ModuleName], app.ModuleAccountAddrs(),
	)
	app.BankKeeper = bank.NewBankKeeperAdapter(bankKeeper)
	app.ParamsKeeper.SetBankKeeper(app.BankKeeper)
	sup := supply.NewKeeper(
		codecProxy.GetCdc(), keys[supply.StoreKey], &app.AccountKeeper, bank.NewBankKeeperAdapter(app.BankKeeper), maccPerms,
	)
	app.SupplyKeeper = supply.NewSupplyKeeperAdapter(sup)
	stakingKeeper := staking2.NewStakingKeeper(
		codecProxy, keys[staking.StoreKey], app.SupplyKeeper, app.subspaces[staking.ModuleName],
	).Keeper
	app.ParamsKeeper.SetStakingKeeper(stakingKeeper)
	app.MintKeeper = mint.NewKeeper(
		codecProxy.GetCdc(), keys[mint.StoreKey], app.subspaces[mint.ModuleName], stakingKeeper,
		app.SupplyKeeper, auth.FeeCollectorName, farm.MintFarmingAccount,
	)
	app.DistrKeeper = distr.NewKeeper(
		codecProxy.GetCdc(), keys[distr.StoreKey], app.subspaces[distr.ModuleName], stakingKeeper,
		app.SupplyKeeper, auth.FeeCollectorName, app.ModuleAccountAddrs(),
	)
	app.SlashingKeeper = slashing.NewKeeper(
		codecProxy.GetCdc(), keys[slashing.StoreKey], stakingKeeper, app.subspaces[slashing.ModuleName],
	)
	app.CrisisKeeper = crisis.NewKeeper(
		app.subspaces[crisis.ModuleName], invCheckPeriod, app.SupplyKeeper, auth.FeeCollectorName,
	)
	app.UpgradeKeeper = upgrade.NewKeeper(skipUpgradeHeights, keys[upgrade.StoreKey], app.marshal.GetCdc())
	app.ParamsKeeper.RegisterSignal(evmtypes.SetEvmParamsNeedUpdate)
	app.EvmKeeper = evm.NewKeeper(
		app.marshal.GetCdc(), keys[evm.StoreKey], app.subspaces[evm.ModuleName], &app.AccountKeeper, app.SupplyKeeper, app.BankKeeper, stakingKeeper, logger)
	(&bankKeeper).SetInnerTxKeeper(app.EvmKeeper)

	app.TokenKeeper = token.NewKeeper(app.BankKeeper, app.subspaces[token.ModuleName], auth.FeeCollectorName, app.SupplyKeeper,
		keys[token.StoreKey], keys[token.KeyLock], app.marshal.GetCdc(), false, &app.AccountKeeper)

	app.DexKeeper = dex.NewKeeper(auth.FeeCollectorName, app.SupplyKeeper, app.subspaces[dex.ModuleName], app.TokenKeeper, stakingKeeper,
		app.BankKeeper, app.keys[dex.StoreKey], app.keys[dex.TokenPairStoreKey], app.marshal.GetCdc())

	app.OrderKeeper = order.NewKeeper(
		app.TokenKeeper, app.SupplyKeeper, app.DexKeeper, app.subspaces[order.ModuleName], auth.FeeCollectorName,
		app.keys[order.OrderStoreKey], app.marshal.GetCdc(), false, monitor.NopOrderMetrics())

	app.SwapKeeper = ammswap.NewKeeper(app.SupplyKeeper, app.TokenKeeper, app.marshal.GetCdc(), app.keys[ammswap.StoreKey], app.subspaces[ammswap.ModuleName])

	app.FarmKeeper = farm.NewKeeper(auth.FeeCollectorName, app.SupplyKeeper.Keeper, app.TokenKeeper, app.SwapKeeper, *app.EvmKeeper, app.subspaces[farm.StoreKey],
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
	app.ibcScopeKeep = scopedIBCKeeper
	scopedTransferKeeper := app.CapabilityKeeper.ScopeToModule(ibctransfertypes.ModuleName)
	// NOTE: the IBC mock keeper and application module is used only for testing core IBC. Do
	// note replicate if you do not need to test core IBC or light clients.
	scopedIBCMockKeeper := app.CapabilityKeeper.ScopeToModule(mock.ModuleName)
	scopedICAMockKeeper := app.CapabilityKeeper.ScopeToModule(mock.ModuleName + icacontrollertypes.SubModuleName)
	scopedICAControllerKeeper := app.CapabilityKeeper.ScopeToModule(icacontrollertypes.SubModuleName)
	scopedICAHostKeeper := app.CapabilityKeeper.ScopeToModule(icahosttypes.SubModuleName)
	scopedICAMauthKeeper := app.CapabilityKeeper.ScopeToModule(icamauthtypes.ModuleName)
	scopedFeeMockKeeper := app.CapabilityKeeper.ScopeToModule(MockFeePort)

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
		v2keeper.ChannelKeeper, &v2keeper.PortKeeper,
		app.SupplyKeeper, app.SupplyKeeper, scopedTransferKeeper, interfaceReg,
	)
	ibctransfertypes.SetMarshal(codecProxy)

	app.IBCFeeKeeper = ibcfeekeeper.NewKeeper(codecProxy, keys[ibcfeetypes.StoreKey], app.GetSubspace(ibcfeetypes.ModuleName),
		v2keeper.ChannelKeeper, // may be replaced with IBC middleware
		v2keeper.ChannelKeeper,
		&v2keeper.PortKeeper, app.SupplyKeeper, app.SupplyKeeper,
	)

	// ICA Controller keeper
	app.ICAControllerKeeper = icacontrollerkeeper.NewKeeper(
		codecProxy, keys[icacontrollertypes.StoreKey], app.GetSubspace(icacontrollertypes.SubModuleName),
		app.IBCFeeKeeper, // use ics29 fee as ics4Wrapper in middleware stack
		app.IBCKeeper.V2Keeper.ChannelKeeper, &app.IBCKeeper.V2Keeper.PortKeeper,
		scopedICAControllerKeeper, app.MsgServiceRouter(),
	)

	// ICA Host keeper
	app.ICAHostKeeper = icahostkeeper.NewKeeper(
		codecProxy, keys[icahosttypes.StoreKey], app.GetSubspace(icahosttypes.SubModuleName),
		app.IBCKeeper.V2Keeper.ChannelKeeper, &app.IBCKeeper.V2Keeper.PortKeeper,
		app.SupplyKeeper, scopedICAHostKeeper, app.MsgServiceRouter(),
	)

	app.ICAMauthKeeper = icamauthkeeper.NewKeeper(
		codecProxy,
		keys[icamauthtypes.StoreKey],
		app.ICAControllerKeeper,
		scopedICAMauthKeeper,
	)

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
		AddRoute(ibchost.RouterKey, ibcclient.NewClientUpdateProposalHandler(v2keeper.ClientKeeper)).
		AddRoute(ibcclienttypes.RouterKey, ibcclient.NewClientUpdateProposalHandler(v2keeper.ClientKeeper)).
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
		app.SupplyKeeper, stakingKeeper, gov.DefaultParamspace, govRouter,
		app.BankKeeper, govProposalHandlerRouter, auth.FeeCollectorName,
	)
	app.ParamsKeeper.SetGovKeeper(app.GovKeeper)
	app.DexKeeper.SetGovKeeper(app.GovKeeper)
	app.FarmKeeper.SetGovKeeper(app.GovKeeper)
	app.EvmKeeper.SetGovKeeper(app.GovKeeper)
	app.MintKeeper.SetGovKeeper(app.GovKeeper)
	app.Erc20Keeper.SetGovKeeper(app.GovKeeper)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := ibcporttypes.NewRouter()
	// Set EVM hooks
	//app.EvmKeeper.SetHooks(evm.NewLogProcessEvmHook(erc20.NewSendToIbcEventHandler(app.Erc20Keeper)))
	// Set IBC hooks
	//app.TransferKeeper = *app.TransferKeeper.SetHooks(erc20.NewIBCTransferHooks(app.Erc20Keeper))
	//transferModule := ibctransfer.NewAppModule(app.TransferKeeper, codecProxy)

	//middle := transfer2.NewIBCModule(app.TransferKeeper)
	transferModule := transfer.TNewTransferModule(app.TransferKeeper, codecProxy)
	left := common.NewDisaleProxyMiddleware()
	middle := ibctransfer.NewIBCModule(app.TransferKeeper, transferModule.AppModule)
	right := ibcfee.NewIBCMiddleware(middle, app.IBCFeeKeeper)
	transferStack := ibcporttypes.NewFacadedMiddleware(left,
		ibccommon.DefaultFactory(tmtypes.HigherThanVenus4, ibc.IBCV4, right),
		ibccommon.DefaultFactory(tmtypes.HigherThanVenus1, ibc.IBCV2, middle))

	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transferStack)

	mockModule := mock.NewAppModule(scopedIBCMockKeeper, &v2keeper.PortKeeper)
	mockIBCModule := mock.NewIBCModule(&mockModule, mock.NewMockIBCApp(mock.ModuleName, scopedIBCMockKeeper))
	ibcRouter.AddRoute(mock.ModuleName, mockIBCModule)
	// The mock module is used for testing IBC
	//mockIBCModule := mock.NewIBCModule(&mockModule, mock.NewMockIBCApp(mock.ModuleName, scopedIBCMockKeeper))

	var icaControllerStack ibcporttypes.IBCModule
	icaControllerStack = mock.NewIBCModule(&mockModule, mock.NewMockIBCApp("", scopedICAMockKeeper))
	app.ICAAuthModule = icaControllerStack.(mock.IBCModule)
	icaControllerStack = icacontroller.NewIBCMiddleware(icaControllerStack, app.ICAControllerKeeper)
	icaControllerStack = ibcfee.NewIBCMiddleware(icaControllerStack, app.IBCFeeKeeper)

	var icaHostStack ibcporttypes.IBCModule
	icaHostStack = icahost.NewIBCModule(app.ICAHostKeeper)
	icaHostStack = ibcfee.NewIBCMiddleware(icaHostStack, app.IBCFeeKeeper)
	// fee
	feeMockModule := mock.NewIBCModule(&mockModule, mock.NewMockIBCApp(MockFeePort, scopedFeeMockKeeper))
	app.FeeMockModule = feeMockModule
	feeWithMockModule := ibcfee.NewIBCMiddleware(feeMockModule, app.IBCFeeKeeper)
	ibcRouter.AddRoute(MockFeePort, feeWithMockModule)

	ibcRouter.AddRoute(icacontrollertypes.SubModuleName, icaControllerStack)
	ibcRouter.AddRoute(icahosttypes.SubModuleName, icaHostStack)
	ibcRouter.AddRoute(icamauthtypes.ModuleName, icaControllerStack)
	ibcRouter.AddRoute(mock.ModuleName+icacontrollertypes.SubModuleName, icaControllerStack) // ica with mock auth module stack route to ica (top level of middleware stack)
	//ibcRouter.AddRoute(ibcmock.ModuleName, mockModule)
	v2keeper.SetRouter(ibcRouter)

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
	app.wasmKeeper = wasm.NewKeeper(
		app.marshal,
		keys[wasm.StoreKey],
		app.subspaces[wasm.ModuleName],
		&app.AccountKeeper,
		bank.NewBankKeeperAdapter(app.BankKeeper),
		v2keeper.ChannelKeeper,
		&v2keeper.PortKeeper,
		nil,
		app.TransferKeeper,
		app.MsgServiceRouter(),
		app.GRPCQueryRouter(),
		wasmDir,
		wasmConfig,
		supportedFeatures,
	)

	// NOTE: Any module instantiated in the module manager that is later modified
	// must be passed by reference here.
	app.mm = module.NewManager(
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app.BaseApp.DeliverTx),
		auth.NewAppModule(app.AccountKeeper),
		bank.NewAppModule(app.BankKeeper, app.AccountKeeper, app.SupplyKeeper),
		crisis.NewAppModule(&app.CrisisKeeper),
		supply.NewAppModule(app.SupplyKeeper.Keeper, app.AccountKeeper),
		gov.NewAppModule(app.GovKeeper, app.SupplyKeeper),
		mint.NewAppModule(app.MintKeeper),
		slashing.NewAppModule(app.SlashingKeeper, app.AccountKeeper, app.StakingKeeper),
		distr.NewAppModule(app.DistrKeeper, app.SupplyKeeper),
		staking2.TNewStakingModule(app.StakingKeeper, app.AccountKeeper, app.SupplyKeeper),
		evidence.NewAppModule(app.EvidenceKeeper),
		evm2.TNewEvmModuleAdapter(app.EvmKeeper, &app.AccountKeeper),
		token.NewAppModule(commonversion.ProtocolVersionV0, app.TokenKeeper, app.SupplyKeeper),
		dex.NewAppModule(commonversion.ProtocolVersionV0, app.DexKeeper, app.SupplyKeeper),
		order.NewAppModule(commonversion.ProtocolVersionV0, app.OrderKeeper, app.SupplyKeeper),
		ammswap.NewAppModule(app.SwapKeeper),
		farm.NewAppModule(app.FarmKeeper),
		params.NewAppModule(app.ParamsKeeper),
		// ibc
		//ibc.NewAppModule(app.IBCKeeper),
		core.NewIBCCOreAppModule(app.IBCKeeper),
		//capabilityModule.NewAppModule(codecProxy, *app.CapabilityKeeper),
		capability.TNewCapabilityModuleAdapter(codecProxy, *app.CapabilityKeeper),
		transferModule,
		erc20.NewAppModule(app.Erc20Keeper),
		mockModule,
		wasm.NewAppModule(*app.marshal, &app.wasmKeeper),
		fee.NewTestFeeAppModule(app.IBCFeeKeeper),
		ica2.NewTestICAModule(codecProxy, &app.ICAControllerKeeper, &app.ICAHostKeeper),
		icamauth.NewAppModule(codecProxy, app.ICAMauthKeeper),
	)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, so as to keep the
	// CanWithdrawInvariant invariant.
	app.mm.SetOrderBeginBlockers(
		bank.ModuleName,
		capabilitytypes.ModuleName,
		order.ModuleName,
		token.ModuleName,
		dex.ModuleName,
		mint.ModuleName,
		distr.ModuleName,
		slashing.ModuleName,
		staking.ModuleName,
		farm.ModuleName,
		evidence.ModuleName,
		evm.ModuleName,
		ibchost.ModuleName,
		ibctransfertypes.ModuleName,
		mock.ModuleName,
		wasm.ModuleName,
	)
	app.mm.SetOrderEndBlockers(
		crisis.ModuleName,
		gov.ModuleName,
		dex.ModuleName,
		order.ModuleName,
		staking.ModuleName,
		evm.ModuleName,
		mock.ModuleName,
		wasm.ModuleName,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	// properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		capabilitytypes.ModuleName,
		auth.ModuleName, distr.ModuleName, staking.ModuleName, bank.ModuleName,
		slashing.ModuleName, gov.ModuleName, mint.ModuleName, supply.ModuleName,
		token.ModuleName, dex.ModuleName, order.ModuleName, ammswap.ModuleName, farm.ModuleName,
		ibctransfertypes.ModuleName,
		ibchost.ModuleName,
		evm.ModuleName, crisis.ModuleName, genutil.ModuleName, params.ModuleName, evidence.ModuleName,
		erc20.ModuleName,
		mock.ModuleName,
		wasm.ModuleName,
		icatypes.ModuleName, ibcfeetypes.ModuleName,
	)

	app.mm.RegisterInvariants(&app.CrisisKeeper)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())
	app.configurator = module.NewConfigurator(app.Codec(), app.MsgServiceRouter(), app.GRPCQueryRouter())
	app.mm.RegisterServices(app.configurator)
	app.setupUpgradeModules(false)

	// create the simulation manager and define the order of the modules for deterministic simulations
	//
	// NOTE: this is not required apps that don't use the simulator for fuzz testing
	// transactions
	app.sm = module.NewSimulationManager(
		auth.NewAppModule(app.AccountKeeper),
		bank.NewAppModule(app.BankKeeper, app.AccountKeeper, app.SupplyKeeper),
		supply.NewAppModule(app.SupplyKeeper.Keeper, app.AccountKeeper),
		gov.NewAppModule(app.GovKeeper, app.SupplyKeeper),
		mint.NewAppModule(app.MintKeeper),
		staking.NewAppModule(app.StakingKeeper, app.AccountKeeper, app.SupplyKeeper),
		distr.NewAppModule(app.DistrKeeper, app.SupplyKeeper),
		slashing.NewAppModule(app.SlashingKeeper, app.AccountKeeper, app.StakingKeeper),
		params.NewAppModule(app.ParamsKeeper), // NOTE: only used for simulation to generate randomized param change proposals
		ibc.NewAppModule(app.IBCKeeper),
		wasm.NewAppModule(*app.marshal, &app.wasmKeeper),
	)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)
	app.MountMemoryStores(memKeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)
	app.WasmHandler = wasmkeeper.HandlerOption{
		WasmConfig:        &wasmConfig,
		TXCounterStoreKey: keys[wasm.StoreKey],
	}
	app.SetAnteHandler(ante.NewAnteHandler(app.AccountKeeper, app.EvmKeeper, app.SupplyKeeper, validateMsgHook(app.OrderKeeper), app.WasmHandler, app.IBCKeeper, app.StakingKeeper, app.ParamsKeeper))
	app.SetEndBlocker(app.EndBlocker)
	app.SetGasRefundHandler(refund.NewGasRefundHandler(app.AccountKeeper, app.SupplyKeeper, app.EvmKeeper))
	app.SetAccNonceHandler(NewAccHandler(app.AccountKeeper))
	app.SetUpdateWasmTxCount(fixCosmosTxCountInWasmForParallelTx(app.WasmHandler.TXCounterStoreKey))
	app.SetUpdateFeeCollectorAccHandler(updateFeeCollectorHandler(app.BankKeeper, app.SupplyKeeper.Keeper))
	app.SetGetFeeCollectorInfo(getFeeCollectorInfo(app.BankKeeper, app.SupplyKeeper.Keeper))
	app.SetParallelTxLogHandlers(fixLogForParallelTxHandler(app.EvmKeeper))
	app.SetPartialConcurrentHandlers(getTxFeeAndFromHandler(app.EvmKeeper))
	app.SetGetTxFeeHandler(getTxFeeHandler())
	app.SetEvmSysContractAddressHandler(NewEvmSysContractAddressHandler(app.EvmKeeper))
	app.SetEvmWatcherCollector(func(...sdk.IWatcher) {})

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
	app.ScopedICAMockKeeper = scopedICAMockKeeper
	app.ScopedICAHostKeeper = scopedICAHostKeeper

	return app
}

func updateFeeCollectorHandler(bk bank.Keeper, sk supply.Keeper) sdk.UpdateFeeCollectorAccHandler {
	return func(ctx sdk.Context, balance sdk.Coins, txFeesplit []*sdk.FeeSplitInfo) error {
		if !balance.Empty() {
			return bk.SetCoins(ctx, sk.GetModuleAccount(ctx, auth.FeeCollectorName).GetAddress(), balance)
		}
		return nil
	}
}

func getFeeCollectorInfo(bk bank.Keeper, sk supply.Keeper) sdk.GetFeeCollectorInfo {
	return func(ctx sdk.Context, onlyGetFeeCollectorStoreKey bool) (sdk.Coins, []byte) {
		if onlyGetFeeCollectorStoreKey {
			return sdk.Coins{}, auth.AddressStoreKey(sk.GetModuleAddress(auth.FeeCollectorName))
		}
		return bk.GetCoins(ctx, sk.GetModuleAddress(auth.FeeCollectorName)), nil
	}
}

func fixLogForParallelTxHandler(ek *evm.Keeper) sdk.LogFix {
	return func(tx []sdk.Tx, logIndex []int, hasEnterEvmTx []bool, anteErrs []error, resp []abci.ResponseDeliverTx) (logs [][]byte) {
		return ek.FixLog(tx, logIndex, hasEnterEvmTx, anteErrs, resp)
	}
}
func evmTxVerifySigHandler(chainID string, blockHeight int64, evmTx *evmtypes.MsgEthereumTx) error {
	chainIDEpoch, err := ethermint.ParseChainID(chainID)
	if err != nil {
		return err
	}
	err = evmTx.VerifySig(chainIDEpoch, blockHeight)
	if err != nil {
		return err
	}
	return nil
}

func getTxFeeAndFromHandler(ek ante.EVMKeeper) sdk.GetTxFeeAndFromHandler {
	return func(ctx sdk.Context, tx sdk.Tx) (fee sdk.Coins, isEvm bool, isE2C bool, from string, to string, err error, supportPara bool) {
		if evmTx, ok := tx.(*evmtypes.MsgEthereumTx); ok {
			isEvm = true
			supportPara = true
			if ante.IsE2CTx(ek, &ctx, evmTx) {
				isE2C = true
				// E2C will include cosmos Msg in the Payload.
				// Sometimes, this Msg do not support parallel execution.
				if !isParaSupportedE2CMsg(evmTx.Data.Payload) {
					supportPara = false
				}
			}
			err = evmTxVerifySigHandler(ctx.ChainID(), ctx.BlockHeight(), evmTx)
			if err != nil {
				return
			}
			fee = evmTx.GetFee()
			from = evmTx.BaseTx.From
			if len(from) > 2 {
				from = strings.ToLower(from[2:])
			}
			if evmTx.To() != nil {
				to = strings.ToLower(evmTx.To().String()[2:])
			}
		} else if feeTx, ok := tx.(authante.FeeTx); ok {
			fee = feeTx.GetFee()
			if stdTx, ok := tx.(*auth.StdTx); ok && len(stdTx.Msgs) == 1 { // only support one message
				if msg, ok := stdTx.Msgs[0].(interface{ CalFromAndToForPara() (string, string) }); ok {
					from, to = msg.CalFromAndToForPara()
					if tmtypes.HigherThanVenus6(ctx.BlockHeight()) {
						supportPara = true
					}
				}
			}
		}

		return
	}
}

func getTxFeeHandler() sdk.GetTxFeeHandler {
	return func(tx sdk.Tx) (fee sdk.Coins) {
		if feeTx, ok := tx.(authante.FeeTx); ok {
			fee = feeTx.GetFee()
		}

		return
	}
}

func isParaSupportedE2CMsg(payload []byte) bool {
	// Here, payload must be E2C's Data.Payload
	p, err := evm.ParseContractParam(payload)
	if err != nil {
		return false
	}
	mw, err := baseapp.ParseMsgWrapper(p)
	if err != nil {
		return false
	}
	switch mw.Name {
	case "wasm/MsgInstantiateContract":
		return false
	case "wasm/MsgMigrateContract":
		return false
	case "wasm/MsgUpdateAdmin":
		return false
	default:
		return true
	}
}

func (app *SimApp) SetOption(req abci.RequestSetOption) (res abci.ResponseSetOption) {
	if req.Key == "CheckChainID" {
		if err := okexchain.IsValidateChainIdWithGenesisHeight(req.Value); err != nil {
			app.Logger().Error(err.Error())
			panic(err)
		}
		err := okexchain.SetChainId(req.Value)
		if err != nil {
			app.Logger().Error(err.Error())
			panic(err)
		}
	}
	return app.BaseApp.SetOption(req)
}

func (app *SimApp) LoadStartVersion(height int64) error {
	return app.LoadVersion(height, app.keys[bam.MainStoreKey])
}

// Name returns the name of the App
func (app *SimApp) Name() string { return app.BaseApp.Name() }

// BeginBlocker updates every begin block
func (app *SimApp) BeginBlocker(ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	return app.mm.BeginBlock(ctx, req)
}

// EndBlocker updates every end block
func (app *SimApp) EndBlocker(ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	// if appconfig.GetOecConfig().GetEnableDynamicGp() {
	// 	GlobalGpIndex = CalBlockGasPriceIndex(app.blockGasPrice, appconfig.GetOecConfig().GetDynamicGpWeight())
	// 	app.blockGasPrice = app.blockGasPrice[:0]
	// }

	return app.mm.EndBlock(ctx, req)
}

// InitChainer updates at chain initialization
func (app *SimApp) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {

	var genesisState simapp.GenesisState
	//app.UpgradeKeeper.SetModuleVersionMap(ctx, app.mm.GetVersionMap())
	app.marshal.GetCdc().MustUnmarshalJSON(req.AppStateBytes, &genesisState)
	return app.mm.InitGenesis(ctx, genesisState)
}

// LoadHeight loads state at a particular height
func (app *SimApp) LoadHeight(height int64) error {
	return app.LoadVersion(height, app.keys[bam.MainStoreKey])
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *SimApp) ModuleAccountAddrs() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range maccPerms {
		if acc == mock.ModuleName {
			continue
		}
		modAccAddrs[supply.NewModuleAddress(acc).String()] = true
	}

	return modAccAddrs
}

// SimulationManager implements the SimulationApp interface
func (app *SimApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *SimApp) GetKey(storeKey string) *sdk.KVStoreKey {
	return app.keys[storeKey]
}

func (app *SimApp) GetMemKey(storeKey string) *sdk.MemoryStoreKey {
	return app.memKeys[storeKey]
}

func (app *SimApp) GetBaseApp() *bam.BaseApp {
	return app.BaseApp
}

func (app *SimApp) GetStakingKeeper() staking.Keeper {
	return app.StakingKeeper
}
func (app *SimApp) GetIBCKeeper() *ibckeeper.Keeper {
	return app.IBCKeeper.V2Keeper
}
func (app *SimApp) GetFacadedKeeper() *ibc.Keeper {
	return app.IBCKeeper
}

func (app *SimApp) GetScopedIBCKeeper() (cap capabilitykeeper.ScopedKeeper) {
	cap = app.ibcScopeKeep
	return
}

func (app *SimApp) AppCodec() *codec.CodecProxy {
	return app.marshal
}

func (app *SimApp) LastCommitID() sdk.CommitID {
	return app.BaseApp.GetCMS().LastCommitID()
}

func (app *SimApp) LastBlockHeight() int64 {
	return app.GetCMS().LastCommitID().Version
}

func (app *SimApp) Codec() *codec.Codec {
	return app.marshal.GetCdc()
}

func (app *SimApp) Marshal() *codec.CodecProxy {
	return app.marshal
}

// GetSubspace returns a param subspace for a given module name.
//
// NOTE: This is solely to be used for testing purposes.
func (app *SimApp) GetSubspace(moduleName string) params.Subspace {
	return app.subspaces[moduleName]
}

var protoCodec = encoding.GetCodec(proto.Name)

func makeInterceptors() map[string]bam.Interceptor {
	m := make(map[string]bam.Interceptor)
	m["/cosmos.tx.v1beta1.Service/Simulate"] = bam.NewRedirectInterceptor("app/simulate")
	m["/cosmos.bank.v1beta1.Query/AllBalances"] = bam.NewRedirectInterceptor("custom/bank/grpc_balances")
	m["/cosmos.staking.v1beta1.Query/Params"] = bam.NewRedirectInterceptor("custom/staking/params4ibc")
	return m
}

// GetMaccPerms returns a copy of the module account permissions
func GetMaccPerms() map[string][]string {
	dupMaccPerms := make(map[string][]string)
	for k, v := range maccPerms {
		dupMaccPerms[k] = v
	}

	return dupMaccPerms
}

func validateMsgHook(orderKeeper order.Keeper) ante.ValidateMsgHandler {
	return func(newCtx sdk.Context, msgs []sdk.Msg) error {

		wrongMsgErr := sdk.ErrUnknownRequest(
			"It is not allowed that a transaction with more than one message contains order or evm message")
		var err error

		for _, msg := range msgs {
			switch assertedMsg := msg.(type) {
			case order.MsgNewOrders:
				if len(msgs) > 1 {
					return wrongMsgErr
				}
				_, err = order.ValidateMsgNewOrders(newCtx, orderKeeper, assertedMsg)
			case order.MsgCancelOrders:
				if len(msgs) > 1 {
					return wrongMsgErr
				}
				err = order.ValidateMsgCancelOrders(newCtx, orderKeeper, assertedMsg)
			case *evmtypes.MsgEthereumTx:
				if len(msgs) > 1 {
					return wrongMsgErr
				}
			}

			if err != nil {
				return err
			}
		}
		return nil
	}
}

func NewAccHandler(ak auth.AccountKeeper) sdk.AccNonceHandler {
	return func(
		ctx sdk.Context, addr sdk.AccAddress,
	) uint64 {
		return ak.GetAccount(ctx, addr).GetSequence()
	}
}

func NewEvmSysContractAddressHandler(ak *evm.Keeper) sdk.EvmSysContractAddressHandler {
	if ak == nil {
		panic("NewEvmSysContractAddressHandler ak is nil")
	}
	return func(
		ctx sdk.Context, addr sdk.AccAddress,
	) bool {
		if addr.Empty() {
			return false
		}
		return ak.IsMatchSysContractAddress(ctx, addr)
	}
}

func PreRun(ctx *server.Context) error {
	// set the dynamic config
	appconfig.RegisterDynamicConfig(ctx.Logger.With("module", "config"))

	// check start flag conflicts
	err := sanity.CheckStart(nil)
	if err != nil {
		return err
	}

	// set config by node mode
	//setNodeConfig(ctx)

	//download pprof
	appconfig.PprofDownload(ctx)

	// pruning options
	_, err = server.GetPruningOptionsFromFlags()
	if err != nil {
		return err
	}
	// repair state on start
	// if viper.GetBool(FlagEnableRepairState) {
	// 	repairStateOnStart(ctx)
	// }

	// init tx signature cache
	tmtypes.InitSignatureCache()
	return nil
}

func (app *SimApp) setupUpgradeModules(onlyTask bool) {
	heightTasks, paramMap, cf, pf, vf := app.CollectUpgradeModules(app.mm)

	app.heightTasks = heightTasks
	if onlyTask {
		return
	}

	app.GetCMS().AppendCommitFilters(cf)
	app.GetCMS().AppendPruneFilters(pf)
	app.GetCMS().AppendVersionFilters(vf)

	vs := app.subspaces
	for k, vv := range paramMap {
		supace, exist := vs[k]
		if !exist {
			continue
		}
		vs[k] = supace.LazyWithKeyTable(subspace.NewKeyTable(vv.ParamSetPairs()...))
	}
}

func (o *SimApp) TxConfig() client.TxConfig {
	return o.txconfig
}

func (o *SimApp) CollectUpgradeModules(m *module.Manager) (map[int64]*upgradetypes.HeightTasks,
	map[string]params.ParamSet, []types.StoreFilter, []types.StoreFilter, []types.VersionFilter) {
	hm := make(map[int64]*upgradetypes.HeightTasks)
	paramsRet := make(map[string]params.ParamSet)
	commitFiltreMap := make(map[*types.StoreFilter]struct{})
	pruneFilterMap := make(map[*types.StoreFilter]struct{})
	versionFilterMap := make(map[*types.VersionFilter]struct{})

	for _, mm := range m.Modules {
		if ada, ok := mm.(upgradetypes.UpgradeModule); ok {
			set := ada.RegisterParam()
			if set != nil {
				if _, exist := paramsRet[ada.ModuleName()]; !exist {
					paramsRet[ada.ModuleName()] = set
				}
			}
			h := ada.UpgradeHeight()
			if h > 0 {
				h++
			}

			cf := ada.CommitFilter()
			if cf != nil {
				if _, exist := commitFiltreMap[cf]; !exist {
					commitFiltreMap[cf] = struct{}{}
				}
			}
			pf := ada.PruneFilter()
			if pf != nil {
				if _, exist := pruneFilterMap[pf]; !exist {
					pruneFilterMap[pf] = struct{}{}
				}
			}
			vf := ada.VersionFilter()
			if vf != nil {
				if _, exist := versionFilterMap[vf]; !exist {
					versionFilterMap[vf] = struct{}{}
				}
			}

			t := ada.RegisterTask()
			if t == nil {
				continue
			}
			if err := t.ValidateBasic(); nil != err {
				panic(err)
			}
			taskList := hm[h]
			if taskList == nil {
				v := make(upgradetypes.HeightTasks, 0)
				taskList = &v
				hm[h] = taskList
			}
			*taskList = append(*taskList, t)
		}
	}

	for _, v := range hm {
		sort.Sort(*v)
	}

	commitFilters := make([]types.StoreFilter, 0)
	pruneFilters := make([]types.StoreFilter, 0)
	versionFilters := make([]types.VersionFilter, 0)
	for pointerFilter, _ := range commitFiltreMap {
		commitFilters = append(commitFilters, *pointerFilter)
	}
	for pointerFilter, _ := range pruneFilterMap {
		pruneFilters = append(pruneFilters, *pointerFilter)
	}
	for pointerFilter, _ := range versionFilterMap {
		versionFilters = append(versionFilters, *pointerFilter)
	}

	return hm, paramsRet, commitFilters, pruneFilters, versionFilters
}

// GetModuleManager returns the app module manager
// NOTE: used for testing purposes
func (app *SimApp) GetModuleManager() *module.Manager {
	return app.mm
}

func fixCosmosTxCountInWasmForParallelTx(storeKey sdk.StoreKey) sdk.UpdateCosmosTxCount {
	return func(ctx sdk.Context, txCount int) {
		wasmkeeper.UpdateTxCount(ctx, storeKey, txCount)
	}
}
