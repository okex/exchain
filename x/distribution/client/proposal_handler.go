package client

import (
	"github.com/okx/exchain/x/distribution/client/cli"
	"github.com/okx/exchain/x/distribution/client/rest"
	govclient "github.com/okx/exchain/x/gov/client"
)

// param change proposal handler
var (
	CommunityPoolSpendProposalHandler      = govclient.NewProposalHandler(cli.GetCmdCommunityPoolSpendProposal, rest.CommunityPoolSpendProposalRESTHandler)
	ChangeDistributionTypeProposalHandler  = govclient.NewProposalHandler(cli.GetChangeDistributionTypeProposal, rest.ChangeDistributionTypeProposalRESTHandler)
	WithdrawRewardEnabledProposalHandler   = govclient.NewProposalHandler(cli.GetWithdrawRewardEnabledProposal, rest.WithdrawRewardEnabledProposalRESTHandler)
	RewardTruncatePrecisionProposalHandler = govclient.NewProposalHandler(cli.GetRewardTruncatePrecisionProposal, rest.RewardTruncatePrecisionProposalRESTHandler)
)
