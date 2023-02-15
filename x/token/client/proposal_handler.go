package client

import (
	govcli "github.com/okex/exchain/x/gov/client"
	"github.com/okex/exchain/x/token/client/cli"
	"github.com/okex/exchain/x/token/client/rest"
)

var (
	ModifyDefaultBondDenomProposalHandler = govcli.NewProposalHandler(cli.GetCmdModifyDefaultBondDenomProposal, rest.ModifyDefaultBondDenomProposalRESTHandler)
	OKT2OKBProposalHandler                = govcli.NewProposalHandler(cli.GetCmdOKT2OKBProposal, rest.OKT2OKBProposalRESTHandler)
)
