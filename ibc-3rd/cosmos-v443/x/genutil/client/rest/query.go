package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/okex/exchain/ibc-3rd/cosmos-v443/client"
	sdk "github.com/okex/exchain/ibc-3rd/cosmos-v443/types"
	"github.com/okex/exchain/ibc-3rd/cosmos-v443/types/rest"
	"github.com/okex/exchain/ibc-3rd/cosmos-v443/x/genutil/types"
)

// QueryGenesisTxs writes the genesis transactions to the response if no error
// occurs.
func QueryGenesisTxs(clientCtx client.Context, w http.ResponseWriter) {
	resultGenesis, err := clientCtx.Client.Genesis(context.Background())
	if err != nil {
		rest.WriteErrorResponse(
			w, http.StatusInternalServerError,
			fmt.Sprintf("failed to retrieve genesis from client: %s", err),
		)
		return
	}

	appState, err := types.GenesisStateFromGenDoc(*resultGenesis.Genesis)
	if err != nil {
		rest.WriteErrorResponse(
			w, http.StatusInternalServerError,
			fmt.Sprintf("failed to decode genesis doc: %s", err),
		)
		return
	}

	genState := types.GetGenesisStateFromAppState(clientCtx.Codec, appState)
	genTxs := make([]sdk.Tx, len(genState.GenTxs))
	for i, tx := range genState.GenTxs {
		err := clientCtx.LegacyAmino.UnmarshalJSON(tx, &genTxs[i])
		if err != nil {
			rest.WriteErrorResponse(
				w, http.StatusInternalServerError,
				fmt.Sprintf("failed to decode genesis transaction: %s", err),
			)
			return
		}
	}

	rest.PostProcessResponseBare(w, clientCtx, genTxs)
}
