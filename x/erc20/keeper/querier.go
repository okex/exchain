package keeper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	ethcmm "github.com/ethereum/go-ethereum/common"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	transfertypes "github.com/okex/exchain/libs/ibc-go/modules/apps/transfer/types"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/x/common"
	"github.com/okex/exchain/x/erc20/types"
)

// NewQuerier is the module level router for state queries
func NewQuerier(keeper Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		if len(path) < 1 {
			return nil, sdkerrors.Wrap(sdkerrors.ErrInvalidRequest,
				"Insufficient parameters, at least 1 parameter is required")
		}

		switch path[0] {
		case types.QueryParameters:
			return queryParams(ctx, keeper)
		case types.QueryTokenMappingChannel:
			return queryTokenMappingChannel(ctx, req, keeper)
		case types.QueryTokenMapping:
			return queryTokenMapping(ctx, keeper)
		case types.QueryDenomByContract:
			return queryDenomByContract(ctx, req, keeper)
		case types.QueryContractByDenom:
			return queryContractByDenom(ctx, req, keeper)
		case types.QueryContractTem:
			return queryContractTemplate(ctx, req, keeper)
		default:
			return nil, sdkerrors.Wrap(sdkerrors.ErrUnknownRequest, "unknown query endpoint")
		}
	}
}

func queryParams(ctx sdk.Context, keeper Keeper) (res []byte, err sdk.Error) {
	params := keeper.GetParams(ctx)
	res, errUnmarshal := codec.MarshalJSONIndent(types.ModuleCdc, params)
	if errUnmarshal != nil {
		return nil, sdk.ErrInternal(sdk.AppendMsgToErr("failed to marshal result to JSON", errUnmarshal.Error()))
	}
	return res, nil
}

func queryTokenMappingChannel(ctx sdk.Context, req abci.RequestQuery, keeper Keeper) ([]byte, error) {
	var params types.TokenMappingByChannelRequest
	err := keeper.cdc.UnmarshalJSON(req.Data, &params)
	if err != nil {
		return nil, common.ErrUnMarshalJSONFailed(sdk.AppendMsgToErr("incorrectly formatted request data", err.Error()))
	}

	path := transfertypes.PortID + "/" + strings.ReplaceAll(params.Channels, ",", "/"+transfertypes.PortID+"/")
	trace := transfertypes.DenomTrace{
		Path:      path,
		BaseDenom: params.BaseDenom,
	}
	hash := trace.Hash()
	_, found := keeper.transferKeeper.GetDenomTrace(ctx, hash)
	if !found {
		return nil, fmt.Errorf("the denom trace for the channel %s and denom %s is not found", params.Channels, params.BaseDenom)
	}

	hexHash := hex.EncodeToString(hash)
	denom := transfertypes.DenomPrefix + "/" + hexHash
	contract, found := keeper.GetContractByDenom(ctx, denom)
	if !found {
		return nil, fmt.Errorf("the erc20 contract for the channel %s and denom %s is not found", params.Channels, params.BaseDenom)
	}
	mapping := types.QueryTokenMappingResponse{
		Denom:     denom,
		Contract:  contract.String(),
		Path:      trace.Path,
		BaseDenom: trace.BaseDenom,
	}
	res, errUnmarshal := codec.MarshalJSONIndent(types.ModuleCdc, mapping)
	if errUnmarshal != nil {
		return nil, sdk.ErrInternal(sdk.AppendMsgToErr("failed to marshal result to JSON", errUnmarshal.Error()))
	}
	return res, nil
}

func queryTokenMapping(ctx sdk.Context, keeper Keeper) ([]byte, error) {
	var mappings []types.QueryTokenMappingResponse
	keeper.IterateMapping(ctx, func(denom, contract string) bool {
		mapping := types.QueryTokenMappingResponse{
			Denom:    denom,
			Contract: contract,
		}

		if types.IsValidIBCDenom(denom) {
			hexHash := denom[len(transfertypes.DenomPrefix+"/"):]
			hash, err := transfertypes.ParseHexHash(hexHash)
			if err == nil {
				denomTrace, found := keeper.transferKeeper.GetDenomTrace(ctx, hash)
				if found {
					mapping.Path = denomTrace.Path
					mapping.BaseDenom = denomTrace.BaseDenom
				}
			}
		}
		mappings = append(mappings, mapping)
		return false
	})

	res, errUnmarshal := codec.MarshalJSONIndent(types.ModuleCdc, mappings)
	if errUnmarshal != nil {
		return nil, sdk.ErrInternal(sdk.AppendMsgToErr("failed to marshal result to JSON", errUnmarshal.Error()))
	}
	return res, nil
}

func queryDenomByContract(ctx sdk.Context, req abci.RequestQuery, keeper Keeper) ([]byte, error) {
	var params types.DenomByContractRequest
	err := keeper.cdc.UnmarshalJSON(req.Data, &params)
	if err != nil {
		return nil, common.ErrUnMarshalJSONFailed(sdk.AppendMsgToErr("incorrectly formatted request data", err.Error()))
	}

	denom, found := keeper.GetDenomByContract(ctx, ethcmm.HexToAddress(params.Contract))
	if !found {
		return nil, fmt.Errorf("coin denom for contract %s is not found", params.Contract)
	}

	return []byte(denom), nil
}

func queryContractByDenom(ctx sdk.Context, req abci.RequestQuery, keeper Keeper) ([]byte, error) {
	var params types.ContractByDenomRequest
	err := keeper.cdc.UnmarshalJSON(req.Data, &params)
	if err != nil {
		return nil, common.ErrUnMarshalJSONFailed(sdk.AppendMsgToErr("incorrectly formatted request data", err.Error()))
	}

	contract, found := keeper.GetContractByDenom(ctx, params.Denom)
	if !found {
		return nil, fmt.Errorf("contract for the coin denom %s is not found", params.Denom)
	}

	return []byte(contract.String()), nil
}

func queryContractTemplate(ctx sdk.Context, req abci.RequestQuery, keeper Keeper) ([]byte, error) {
	ret := types.ContractTemplate{}
	proxy, found := keeper.GetProxyTemplateContract(ctx)
	if found {
		ret.Proxy = string(types.MustMarshalCompileContract(proxy))
	}
	imple, found := keeper.GetImplementTemplateContract(ctx)
	if found {
		ret.Implement = string(types.MustMarshalCompileContract(imple))
	}
	data, _ := json.Marshal(ret)
	return data, nil
}
