package rest

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/types/rest"
	comm "github.com/okex/exchain/x/common"
	"github.com/okex/exchain/x/erc20/types"
	govRest "github.com/okex/exchain/x/gov/client/rest"
)

// RegisterRoutes - Central function to define routes that get registered by the main application
func RegisterRoutes(cliCtx context.CLIContext, r *mux.Router) {
	r.HandleFunc("/erc20/token_mapping", tokenMappingHandlerFn(cliCtx)).Methods("GET")
	r.HandleFunc("/erc20/contract/{denom_hash}", contractByDenomHandlerFn(cliCtx)).Methods("GET")
	r.HandleFunc("/erc20/denom/{contract}", denomByContractHandlerFn(cliCtx)).Methods("GET")
	r.HandleFunc("/erc20/token_mapping_channel/{channels}/{denom}", tokenMappingChannelHandlerFn(cliCtx)).Methods("GET")
}

func tokenMappingHandlerFn(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cliCtx, ok := rest.ParseQueryHeightOrReturnBadRequest(w, cliCtx, r)
		if !ok {
			return
		}

		res, height, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.RouterKey, types.QueryTokenMapping), nil)
		if err != nil {
			sdkErr := comm.ParseSDKError(err.Error())
			comm.HandleErrorMsg(w, cliCtx, sdkErr.Code, sdkErr.Message)
			return
		}

		var result []types.QueryTokenMappingResponse
		if err := cliCtx.Codec.UnmarshalJSON(res, &result); err != nil {
			comm.HandleErrorMsg(w, cliCtx, comm.CodeUnMarshalJSONFailed, err.Error())
			return
		}

		cliCtx = cliCtx.WithHeight(height)
		rest.PostProcessResponse(w, cliCtx, result)
	}
}

func tokenMappingChannelHandlerFn(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channels := mux.Vars(r)["channels"]
		denom := mux.Vars(r)["denom"]

		cliCtx, ok := rest.ParseQueryHeightOrReturnBadRequest(w, cliCtx, r)
		if !ok {
			return
		}

		params := cliCtx.Codec.MustMarshalJSON(types.TokenMappingByChannelRequest{Channels: channels, BaseDenom: denom})
		res, height, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.RouterKey, types.QueryTokenMappingChannel), params)
		if err != nil {
			sdkErr := comm.ParseSDKError(err.Error())
			comm.HandleErrorMsg(w, cliCtx, sdkErr.Code, sdkErr.Message)
			return
		}

		cliCtx = cliCtx.WithHeight(height)
		rest.PostProcessResponse(w, cliCtx, res)
	}
}

func contractByDenomHandlerFn(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		denom := mux.Vars(r)["denom_hash"]

		cliCtx, ok := rest.ParseQueryHeightOrReturnBadRequest(w, cliCtx, r)
		if !ok {
			return
		}

		params := cliCtx.Codec.MustMarshalJSON(types.ContractByDenomRequest{types.IbcDenomPrefix + denom})
		res, height, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.RouterKey, types.QueryContractByDenom), params)
		if err != nil {
			sdkErr := comm.ParseSDKError(err.Error())
			comm.HandleErrorMsg(w, cliCtx, sdkErr.Code, sdkErr.Message)
			return
		}

		cliCtx = cliCtx.WithHeight(height)
		rest.PostProcessResponse(w, cliCtx, res)
	}
}

func denomByContractHandlerFn(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contract := mux.Vars(r)["contract"]

		cliCtx, ok := rest.ParseQueryHeightOrReturnBadRequest(w, cliCtx, r)
		if !ok {
			return
		}

		params := cliCtx.Codec.MustMarshalJSON(types.DenomByContractRequest{contract})
		res, height, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.RouterKey, types.QueryDenomByContract), params)
		if err != nil {
			sdkErr := comm.ParseSDKError(err.Error())
			comm.HandleErrorMsg(w, cliCtx, sdkErr.Code, sdkErr.Message)
			return
		}

		cliCtx = cliCtx.WithHeight(height)
		rest.PostProcessResponse(w, cliCtx, res)
	}
}

// TokenMappingProposalRESTHandler defines erc20 proposal handler
func TokenMappingProposalRESTHandler(context.CLIContext) govRest.ProposalRESTHandler {
	return govRest.ProposalRESTHandler{}
}

// ProxyContractRedirectRESTHandler defines erc20 proxy contract redirect proposal handler
func ProxyContractRedirectRESTHandler(context.CLIContext) govRest.ProposalRESTHandler {
	return govRest.ProposalRESTHandler{}
}
func ContractTemplateProposalRESTHandler(context.CLIContext) govRest.ProposalRESTHandler {
	return govRest.ProposalRESTHandler{}
}
