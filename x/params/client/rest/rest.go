package rest

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/types/rest"
	"github.com/okex/exchain/x/common"
	"github.com/okex/exchain/x/params"
	"github.com/okex/exchain/x/params/types"
	"net/http"
)

// RegisterRoutes, a central function to define routes
// which is called by the rest module in main application
func RegisterRoutes(cliCtx context.CLIContext, r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/params/blockconfig"), QueryBlockConfigFn(cliCtx)).Methods("GET")
}

func QueryBlockConfigFn(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cliCtx, ok := rest.ParseQueryHeightOrReturnBadRequest(w, cliCtx, r)
		if !ok {
			return
		}

		route := fmt.Sprintf("custom/%s/%s", params.RouterKey, types.QueryBlockConfig)
		bz, _, err := cliCtx.QueryWithData(route, nil)
		if err != nil {
			sdkErr := common.ParseSDKError(err.Error())
			common.HandleErrorMsg(w, cliCtx, sdkErr.Code, err.Error())
			return
		}
		rest.PostProcessResponseBare(w, cliCtx, bz)
	}
}
