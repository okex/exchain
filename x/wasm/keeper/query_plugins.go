package keeper

import (
	"encoding/json"
	"errors"

	"github.com/okex/exchain/libs/cosmos-sdk/baseapp"

	channeltypes "github.com/okex/exchain/libs/ibc-go/modules/core/04-channel/types"

	"github.com/okex/exchain/x/wasm/types"

	wasmvmtypes "github.com/CosmWasm/wasmvm/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	distributiontypes "github.com/okex/exchain/libs/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/okex/exchain/libs/cosmos-sdk/x/staking/types"
)

type QueryHandler struct {
	Ctx         sdk.Context
	Plugins     WasmVMQueryHandler
	Caller      sdk.WasmAddress
	gasRegister GasRegister
}

func NewQueryHandler(ctx sdk.Context, vmQueryHandler WasmVMQueryHandler, caller sdk.WasmAddress, gasRegister GasRegister) QueryHandler {
	return QueryHandler{
		Ctx:         ctx,
		Plugins:     vmQueryHandler,
		Caller:      caller,
		gasRegister: gasRegister,
	}
}

type GRPCQueryRouter interface {
	Route(path string) baseapp.GRPCQueryHandler
}

// -- end baseapp interfaces --

var _ wasmvmtypes.Querier = QueryHandler{}

func (q QueryHandler) Query(request wasmvmtypes.QueryRequest, gasLimit uint64) ([]byte, error) {
	// set a limit for a subCtx
	sdkGas := q.gasRegister.FromWasmVMGas(gasLimit)
	// discard all changes/ events in subCtx by not committing the cached context
	subCtx, _ := q.Ctx.CacheContext()
	subCtx.SetGasMeter(sdk.NewGasMeter(sdkGas))

	// make sure we charge the higher level context even on panic
	defer func() {
		q.Ctx.GasMeter().ConsumeGas(subCtx.GasMeter().GasConsumed(), "contract sub-query")
	}()

	res, err := q.Plugins.HandleQuery(subCtx, q.Caller, request)
	if err == nil {
		// short-circuit, the rest is dealing with handling existing errors
		return res, nil
	}

	// special mappings to system error (which are not redacted)
	var noSuchContract *types.ErrNoSuchContract
	if ok := errors.As(err, &noSuchContract); ok {
		err = wasmvmtypes.NoSuchContract{Addr: noSuchContract.Addr}
	}

	// Issue #759 - we don't return error string for worries of non-determinism
	return nil, redactError(err)
}

func (q QueryHandler) GasConsumed() uint64 {
	return q.gasRegister.ToWasmVMGas(q.Ctx.GasMeter().GasConsumed())
}

type CustomQuerier func(ctx sdk.Context, request json.RawMessage) ([]byte, error)

type QueryPlugins struct {
	Bank   func(ctx sdk.Context, request *wasmvmtypes.BankQuery) ([]byte, error)
	Custom CustomQuerier
	//IBC      func(ctx sdk.Context, caller sdk.WasmAddress, request *wasmvmtypes.IBCQuery) ([]byte, error)
	//Staking  func(ctx sdk.Context, request *wasmvmtypes.StakingQuery) ([]byte, error)
	Stargate func(ctx sdk.Context, request *wasmvmtypes.StargateQuery) ([]byte, error)
	Wasm     func(ctx sdk.Context, request *wasmvmtypes.WasmQuery) ([]byte, error)
}

type contractMetaDataSource interface {
	GetContractInfo(ctx sdk.Context, contractAddress sdk.WasmAddress) *types.ContractInfo
}

type wasmQueryKeeper interface {
	contractMetaDataSource
	QueryRaw(ctx sdk.Context, contractAddress sdk.WasmAddress, key []byte) []byte
	QuerySmart(ctx sdk.Context, contractAddr sdk.WasmAddress, req []byte) ([]byte, error)
	IsPinnedCode(ctx sdk.Context, codeID uint64) bool
}

func DefaultQueryPlugins(
	bank types.BankViewKeeper,
	//staking types.StakingKeeper,
	//distKeeper types.DistributionKeeper,
	channelKeeper types.ChannelKeeper,
	queryRouter GRPCQueryRouter,
	wasm wasmQueryKeeper,
) QueryPlugins {
	return QueryPlugins{
		Bank:   BankQuerier(bank),
		Custom: NoCustomQuerier,
		//IBC:    IBCQuerier(wasm, channelKeeper),
		//Staking:  StakingQuerier(staking, distKeeper),
		Stargate: StargateQuerier(queryRouter),
		Wasm:     WasmQuerier(wasm),
	}
}

func (e QueryPlugins) Merge(o *QueryPlugins) QueryPlugins {
	// only update if this is non-nil and then only set values
	if o == nil {
		return e
	}
	if o.Bank != nil {
		e.Bank = o.Bank
	}
	if o.Custom != nil {
		e.Custom = o.Custom
	}
	//if o.IBC != nil {
	//	e.IBC = o.IBC
	//}
	//if o.Staking != nil {
	//	e.Staking = o.Staking
	//}
	if o.Stargate != nil {
		e.Stargate = o.Stargate
	}
	if o.Wasm != nil {
		e.Wasm = o.Wasm
	}
	return e
}

// HandleQuery executes the requested query
func (e QueryPlugins) HandleQuery(ctx sdk.Context, caller sdk.WasmAddress, request wasmvmtypes.QueryRequest) ([]byte, error) {
	// do the query
	if request.Bank != nil {
		return e.Bank(ctx, request.Bank)
	}
	if request.Custom != nil {
		return e.Custom(ctx, request.Custom)
	}
	//if request.IBC != nil {
	//	return e.IBC(ctx, caller, request.IBC)
	//}
	//if request.Staking != nil {
	//	return e.Staking(ctx, request.Staking)
	//}
	if request.Stargate != nil {
		return e.Stargate(ctx, request.Stargate)
	}
	if request.Wasm != nil {
		return e.Wasm(ctx, request.Wasm)
	}
	return nil, wasmvmtypes.Unknown{}
}

func BankQuerier(bankKeeper types.BankViewKeeper) func(ctx sdk.Context, request *wasmvmtypes.BankQuery) ([]byte, error) {
	return func(ctx sdk.Context, request *wasmvmtypes.BankQuery) ([]byte, error) {
		if request.AllBalances != nil {
			addr, err := sdk.WasmAddressFromBech32(request.AllBalances.Address)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.AllBalances.Address)
			}
			coins := bankKeeper.GetAllBalances(ctx, sdk.WasmToAccAddress(addr))
			adapters := sdk.CoinsToCoinAdapters(coins)
			res := wasmvmtypes.AllBalancesResponse{
				Amount: ConvertSdkCoinsToWasmCoins(adapters),
			}
			return json.Marshal(res)
		}
		if request.Balance != nil {
			addr, err := sdk.WasmAddressFromBech32(request.Balance.Address)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.Balance.Address)
			}
			coin := bankKeeper.GetBalance(ctx, sdk.WasmToAccAddress(addr), request.Balance.Denom)
			adapter := sdk.CoinToCoinAdapter(coin)
			res := wasmvmtypes.BalanceResponse{
				Amount: ConvertSdkCoinToWasmCoin(adapter),
			}
			return json.Marshal(res)
		}
		return nil, wasmvmtypes.UnsupportedRequest{Kind: "unknown BankQuery variant"}
	}
}

func NoCustomQuerier(sdk.Context, json.RawMessage) ([]byte, error) {
	return nil, wasmvmtypes.UnsupportedRequest{Kind: "custom"}
}

func IBCQuerier(wasm contractMetaDataSource, channelKeeper types.ChannelKeeper) func(ctx sdk.Context, caller sdk.WasmAddress, request *wasmvmtypes.IBCQuery) ([]byte, error) {
	return func(ctx sdk.Context, caller sdk.WasmAddress, request *wasmvmtypes.IBCQuery) ([]byte, error) {
		if request.PortID != nil {
			contractInfo := wasm.GetContractInfo(ctx, caller)
			res := wasmvmtypes.PortIDResponse{
				PortID: contractInfo.IBCPortID,
			}
			return json.Marshal(res)
		}
		if request.ListChannels != nil {
			portID := request.ListChannels.PortID
			channels := make(wasmvmtypes.IBCChannels, 0)
			channelKeeper.IterateChannels(ctx, func(ch channeltypes.IdentifiedChannel) bool {
				// it must match the port and be in open state
				if (portID == "" || portID == ch.PortId) && ch.State == channeltypes.OPEN {
					newChan := wasmvmtypes.IBCChannel{
						Endpoint: wasmvmtypes.IBCEndpoint{
							PortID:    ch.PortId,
							ChannelID: ch.ChannelId,
						},
						CounterpartyEndpoint: wasmvmtypes.IBCEndpoint{
							PortID:    ch.Counterparty.PortId,
							ChannelID: ch.Counterparty.ChannelId,
						},
						Order:        ch.Ordering.String(),
						Version:      ch.Version,
						ConnectionID: ch.ConnectionHops[0],
					}
					channels = append(channels, newChan)
				}
				return false
			})
			res := wasmvmtypes.ListChannelsResponse{
				Channels: channels,
			}
			return json.Marshal(res)
		}
		if request.Channel != nil {
			channelID := request.Channel.ChannelID
			portID := request.Channel.PortID
			if portID == "" {
				contractInfo := wasm.GetContractInfo(ctx, caller)
				portID = contractInfo.IBCPortID
			}
			got, found := channelKeeper.GetChannel(ctx, portID, channelID)
			var channel *wasmvmtypes.IBCChannel
			// it must be in open state
			if found && got.State == channeltypes.OPEN {
				channel = &wasmvmtypes.IBCChannel{
					Endpoint: wasmvmtypes.IBCEndpoint{
						PortID:    portID,
						ChannelID: channelID,
					},
					CounterpartyEndpoint: wasmvmtypes.IBCEndpoint{
						PortID:    got.Counterparty.PortId,
						ChannelID: got.Counterparty.ChannelId,
					},
					Order:        got.Ordering.String(),
					Version:      got.Version,
					ConnectionID: got.ConnectionHops[0],
				}
			}
			res := wasmvmtypes.ChannelResponse{
				Channel: channel,
			}
			return json.Marshal(res)
		}
		return nil, wasmvmtypes.UnsupportedRequest{Kind: "unknown IBCQuery variant"}
	}
}

func StargateQuerier(queryRouter GRPCQueryRouter) func(ctx sdk.Context, request *wasmvmtypes.StargateQuery) ([]byte, error) {
	return func(ctx sdk.Context, msg *wasmvmtypes.StargateQuery) ([]byte, error) {
		return nil, wasmvmtypes.UnsupportedRequest{Kind: "Stargate queries are disabled."}
	}
}

//var queryDenyList = []string{
//	"/cosmos.tx.",
//	"/cosmos.base.tendermint.",
//}
//
//func StargateQuerier(queryRouter GRPCQueryRouter) func(ctx sdk.Context, request *wasmvmtypes.StargateQuery) ([]byte, error) {
//	return func(ctx sdk.Context, msg *wasmvmtypes.StargateQuery) ([]byte, error) {
//		for _, b := range queryDenyList {
//			if strings.HasPrefix(msg.Path, b) {
//				return nil, wasmvmtypes.UnsupportedRequest{Kind: "path is not allowed from the contract"}
//			}
//		}
//
//		route := queryRouter.Route(msg.Path)
//		if route == nil {
//			return nil, wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("No route to query '%s'", msg.Path)}
//		}
//		req := abci.RequestQuery{
//			Data: msg.Data,
//			Path: msg.Path,
//		}
//		res, err := route(ctx, req)
//		if err != nil {
//			return nil, err
//		}
//		return res.Value, nil
//	}
//}

func StakingQuerier(keeper types.StakingKeeper, distKeeper types.DistributionKeeper) func(ctx sdk.Context, request *wasmvmtypes.StakingQuery) ([]byte, error) {
	return func(ctx sdk.Context, request *wasmvmtypes.StakingQuery) ([]byte, error) {
		if request.BondedDenom != nil {
			denom := keeper.BondDenom(ctx)
			res := wasmvmtypes.BondedDenomResponse{
				Denom: denom,
			}
			return json.Marshal(res)
		}
		if request.AllValidators != nil {
			validators := keeper.GetBondedValidatorsByPower(ctx)
			// validators := keeper.GetAllValidators(ctx)
			wasmVals := make([]wasmvmtypes.Validator, len(validators))
			for i, v := range validators {
				wasmVals[i] = wasmvmtypes.Validator{
					Address:       v.OperatorAddress.String(),
					Commission:    v.Commission.Rate.String(),
					MaxCommission: v.Commission.MaxRate.String(),
					MaxChangeRate: v.Commission.MaxChangeRate.String(),
				}
			}
			res := wasmvmtypes.AllValidatorsResponse{
				Validators: wasmVals,
			}
			return json.Marshal(res)
		}
		if request.Validator != nil {
			valAddr, err := sdk.ValAddressFromBech32(request.Validator.Address)
			if err != nil {
				return nil, err
			}
			v, found := keeper.GetValidator(ctx, valAddr)
			res := wasmvmtypes.ValidatorResponse{}
			if found {
				res.Validator = &wasmvmtypes.Validator{
					Address:       v.OperatorAddress.String(),
					Commission:    v.Commission.Rate.String(),
					MaxCommission: v.Commission.MaxRate.String(),
					MaxChangeRate: v.Commission.MaxChangeRate.String(),
				}
			}
			return json.Marshal(res)
		}
		if request.AllDelegations != nil {
			delegator, err := sdk.WasmAddressFromBech32(request.AllDelegations.Delegator)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.AllDelegations.Delegator)
			}
			sdkDels := keeper.GetAllDelegatorDelegations(ctx, sdk.WasmToAccAddress(delegator))
			delegations, err := sdkToDelegations(ctx, keeper, sdkDels)
			if err != nil {
				return nil, err
			}
			res := wasmvmtypes.AllDelegationsResponse{
				Delegations: delegations,
			}
			return json.Marshal(res)
		}
		if request.Delegation != nil {
			delegator, err := sdk.WasmAddressFromBech32(request.Delegation.Delegator)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.Delegation.Delegator)
			}
			validator, err := sdk.ValAddressFromBech32(request.Delegation.Validator)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.Delegation.Validator)
			}

			var res wasmvmtypes.DelegationResponse
			d, found := keeper.GetDelegation(ctx, sdk.WasmToAccAddress(delegator), validator)
			if found {
				res.Delegation, err = sdkToFullDelegation(ctx, keeper, distKeeper, d)
				if err != nil {
					return nil, err
				}
			}
			return json.Marshal(res)
		}
		return nil, wasmvmtypes.UnsupportedRequest{Kind: "unknown Staking variant"}
	}
}

func sdkToDelegations(ctx sdk.Context, keeper types.StakingKeeper, delegations []stakingtypes.Delegation) (wasmvmtypes.Delegations, error) {
	result := make([]wasmvmtypes.Delegation, len(delegations))
	bondDenom := keeper.BondDenom(ctx)

	for i, d := range delegations {
		delAddr, err := sdk.WasmAddressFromBech32(d.DelegatorAddress.String())
		if err != nil {
			return nil, sdkerrors.Wrap(err, "delegator address")
		}
		valAddr, err := sdk.ValAddressFromBech32(d.ValidatorAddress.String())
		if err != nil {
			return nil, sdkerrors.Wrap(err, "validator address")
		}

		// shares to amount logic comes from here:
		// https://github.com/okex/exchain/libs/cosmos-sdk/blob/v0.38.3/x/staking/keeper/querier.go#L404
		val, found := keeper.GetValidator(ctx, valAddr)
		if !found {
			return nil, sdkerrors.Wrap(stakingtypes.ErrNoValidatorFound, "can't load validator for delegation")
		}
		amount := sdk.NewCoin(bondDenom, val.TokensFromShares(d.Shares).TruncateInt())
		adapter := sdk.CoinToCoinAdapter(amount)
		result[i] = wasmvmtypes.Delegation{
			Delegator: delAddr.String(),
			Validator: valAddr.String(),
			Amount:    ConvertSdkCoinToWasmCoin(adapter),
		}
	}
	return result, nil
}

func sdkToFullDelegation(ctx sdk.Context, keeper types.StakingKeeper, distKeeper types.DistributionKeeper, delegation stakingtypes.Delegation) (*wasmvmtypes.FullDelegation, error) {
	delAddr, err := sdk.WasmAddressFromBech32(delegation.DelegatorAddress.String())
	if err != nil {
		return nil, sdkerrors.Wrap(err, "delegator address")
	}
	valAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress.String())
	if err != nil {
		return nil, sdkerrors.Wrap(err, "validator address")
	}
	val, found := keeper.GetValidator(ctx, valAddr)
	if !found {
		return nil, sdkerrors.Wrap(stakingtypes.ErrNoValidatorFound, "can't load validator for delegation")
	}
	bondDenom := keeper.BondDenom(ctx)
	amount := sdk.NewCoin(bondDenom, val.TokensFromShares(delegation.Shares).TruncateInt())
	adapter := sdk.CoinToCoinAdapter(amount)
	delegationCoins := ConvertSdkCoinToWasmCoin(adapter)

	// FIXME: this is very rough but better than nothing...
	// https://github.com/okex/exchain/issues/282
	// if this (val, delegate) pair is receiving a redelegation, it cannot redelegate more
	// otherwise, it can redelegate the full amount
	// (there are cases of partial funds redelegated, but this is a start)
	redelegateCoins := wasmvmtypes.NewCoin(0, bondDenom)
	if !keeper.HasReceivingRedelegation(ctx, sdk.WasmToAccAddress(delAddr), valAddr) {
		redelegateCoins = delegationCoins
	}

	// FIXME: make a cleaner way to do this (modify the sdk)
	// we need the info from `distKeeper.calculateDelegationRewards()`, but it is not public
	// neither is `queryDelegationRewards(ctx sdk.Context, _ []string, req abci.RequestQuery, k Keeper)`
	// so we go through the front door of the querier....
	accRewards, err := getAccumulatedRewards(ctx, distKeeper, delegation)
	if err != nil {
		return nil, err
	}

	return &wasmvmtypes.FullDelegation{
		Delegator:          delAddr.String(),
		Validator:          valAddr.String(),
		Amount:             delegationCoins,
		AccumulatedRewards: accRewards,
		CanRedelegate:      redelegateCoins,
	}, nil
}

// FIXME: simplify this enormously when
// https://github.com/okex/exchain/libs/cosmos-sdk/issues/7466 is merged
func getAccumulatedRewards(ctx sdk.Context, distKeeper types.DistributionKeeper, delegation stakingtypes.Delegation) ([]wasmvmtypes.Coin, error) {
	// Try to get *delegator* reward info!
	params := distributiontypes.QueryDelegationRewardsParams{
		DelegatorAddress: delegation.DelegatorAddress,
		ValidatorAddress: delegation.ValidatorAddress,
	}
	cache, _ := ctx.CacheContext()
	qres, err := distKeeper.DelegationRewards(sdk.WrapSDKContext(cache), &params)
	if err != nil {
		return nil, err
	}

	// now we have it, convert it into wasmvm types
	rewards := make([]wasmvmtypes.Coin, qres.Len())
	for i, r := range *qres {
		rewards[i] = wasmvmtypes.Coin{
			Denom:  r.Denom,
			Amount: r.Amount.TruncateInt().String(),
		}
	}
	return rewards, nil
}

func WasmQuerier(k wasmQueryKeeper) func(ctx sdk.Context, request *wasmvmtypes.WasmQuery) ([]byte, error) {
	return func(ctx sdk.Context, request *wasmvmtypes.WasmQuery) ([]byte, error) {
		switch {
		case request.Smart != nil:
			addr, err := sdk.WasmAddressFromBech32(request.Smart.ContractAddr)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.Smart.ContractAddr)
			}
			msg := types.RawContractMessage(request.Smart.Msg)
			if err := msg.ValidateBasic(); err != nil {
				return nil, sdkerrors.Wrap(err, "json msg")
			}
			return k.QuerySmart(ctx, addr, msg)
		case request.Raw != nil:
			addr, err := sdk.WasmAddressFromBech32(request.Raw.ContractAddr)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.Raw.ContractAddr)
			}
			return k.QueryRaw(ctx, addr, request.Raw.Key), nil
		case request.ContractInfo != nil:
			addr, err := sdk.WasmAddressFromBech32(request.ContractInfo.ContractAddr)
			if err != nil {
				return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, request.ContractInfo.ContractAddr)
			}
			info := k.GetContractInfo(ctx, addr)
			if info == nil {
				return nil, &types.ErrNoSuchContract{Addr: request.ContractInfo.ContractAddr}
			}

			res := wasmvmtypes.ContractInfoResponse{
				CodeID:  info.CodeID,
				Creator: info.Creator,
				Admin:   info.Admin,
				Pinned:  k.IsPinnedCode(ctx, info.CodeID),
				IBCPort: info.IBCPortID,
			}
			return json.Marshal(res)
		}
		return nil, wasmvmtypes.UnsupportedRequest{Kind: "unknown WasmQuery variant"}
	}
}

// ConvertSdkCoinsToWasmCoins covert sdk type to wasmvm coins type
func ConvertSdkCoinsToWasmCoins(coins []sdk.CoinAdapter) wasmvmtypes.Coins {
	converted := make(wasmvmtypes.Coins, len(coins))
	for i, c := range coins {
		converted[i] = ConvertSdkCoinToWasmCoin(c)
	}
	return converted
}

// ConvertSdkCoinToWasmCoin covert sdk type to wasmvm coin type
func ConvertSdkCoinToWasmCoin(coin sdk.CoinAdapter) wasmvmtypes.Coin {
	return wasmvmtypes.Coin{
		Denom:  coin.Denom,
		Amount: coin.Amount.String(),
	}
}

var _ WasmVMQueryHandler = WasmVMQueryHandlerFn(nil)

// WasmVMQueryHandlerFn is a helper to construct a function based query handler.
type WasmVMQueryHandlerFn func(ctx sdk.Context, caller sdk.WasmAddress, request wasmvmtypes.QueryRequest) ([]byte, error)

// HandleQuery delegates call into wrapped WasmVMQueryHandlerFn
func (w WasmVMQueryHandlerFn) HandleQuery(ctx sdk.Context, caller sdk.WasmAddress, request wasmvmtypes.QueryRequest) ([]byte, error) {
	return w(ctx, caller, request)
}
