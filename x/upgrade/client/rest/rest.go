package rest

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	govRest "github.com/okex/okexchain/x/gov/client/rest"
)

func ProposalRESTHandler(context.CLIContext) govRest.ProposalRESTHandler {
	return govRest.ProposalRESTHandler{}
}
