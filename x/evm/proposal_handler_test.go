package evm_test

import (
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/okex/exchain/x/evm"
	"github.com/okex/exchain/x/evm/types"
	govtypes "github.com/okex/exchain/x/gov/types"
)

func (suite *EvmTestSuite) TestProposalHandler_ManageContractDeploymentWhitelistProposal() {
	addr1 := ethcmn.BytesToAddress([]byte{0x0}).Bytes()
	addr2 := ethcmn.BytesToAddress([]byte{0x1}).Bytes()

	proposal := types.NewManageContractDeploymentWhitelistProposal(
		"default title",
		"default description",
		types.AddressList{addr1, addr2},
		true,
	)

	suite.govHandler = evm.NewManageContractDeploymentWhitelistProposalHandler(suite.app.EvmKeeper)
	govProposal := govtypes.Proposal{
		Content: proposal,
	}

	testCases := []struct {
		msg                   string
		prepare               func()
		targetAddrListToCheck types.AddressList
	}{
		{
			"add address into whitelist",
			func() {},
			types.AddressList{addr1, addr2},
		},
		{
			"add address repeatedly",
			func() {},
			types.AddressList{addr1, addr2},
		},
		{
			"delete an address from whitelist",
			func() {
				proposal.IsAdded = false
				proposal.DistributorAddrs = types.AddressList{addr1}
				govProposal.Content = proposal
			},
			types.AddressList{addr2},
		},
		{
			"delete an address from whitelist",
			func() {
				proposal.IsAdded = false
				proposal.DistributorAddrs = types.AddressList{addr1}
				govProposal.Content = proposal
			},
			types.AddressList{addr2},
		},
		{
			"delete two addresses from whitelist which contains one of them only",
			func() {
				proposal.DistributorAddrs = types.AddressList{addr1, addr2}
				govProposal.Content = proposal
			},
			types.AddressList{},
		},
		{
			"delete two addresses from whitelist which contains none of them",
			func() {},
			types.AddressList{},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.msg, func() {
			tc.prepare()

			err := suite.govHandler(suite.ctx, &govProposal)
			suite.Require().NoError(err)

			// check the whitelist with target address list
			curWhitelist := suite.stateDB.GetContractDeploymentWhitelist()
			suite.Require().Equal(len(tc.targetAddrListToCheck), len(curWhitelist))

			for i, addr := range curWhitelist {
				suite.Require().Equal(tc.targetAddrListToCheck[i], addr)
			}
		})
	}
}

func (suite *EvmTestSuite) TestProposalHandler_ManageContractBlockedListProposal() {
	addr1 := ethcmn.BytesToAddress([]byte{0x0}).Bytes()
	addr2 := ethcmn.BytesToAddress([]byte{0x1}).Bytes()

	proposal := types.NewManageContractBlockedListProposal(
		"default title",
		"default description",
		types.AddressList{addr1, addr2},
		true,
	)

	suite.govHandler = evm.NewManageContractDeploymentWhitelistProposalHandler(suite.app.EvmKeeper)
	govProposal := govtypes.Proposal{
		Content: proposal,
	}

	testCases := []struct {
		msg                   string
		prepare               func()
		targetAddrListToCheck types.AddressList
	}{
		{
			"add address into blocked list",
			func() {},
			types.AddressList{addr1, addr2},
		},
		{
			"add address repeatedly",
			func() {},
			types.AddressList{addr1, addr2},
		},
		{
			"delete an address from blocked list",
			func() {
				proposal.IsAdded = false
				proposal.ContractAddrs = types.AddressList{addr1}
				govProposal.Content = proposal
			},
			types.AddressList{addr2},
		},
		{
			"delete an address from blocked list",
			func() {
				proposal.IsAdded = false
				proposal.ContractAddrs = types.AddressList{addr1}
				govProposal.Content = proposal
			},
			types.AddressList{addr2},
		},
		{
			"delete two addresses from blocked list which contains one of them only",
			func() {
				proposal.ContractAddrs = types.AddressList{addr1, addr2}
				govProposal.Content = proposal
			},
			types.AddressList{},
		},
		{
			"delete two addresses from blocked list which contains none of them",
			func() {},
			types.AddressList{},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.msg, func() {
			tc.prepare()

			err := suite.govHandler(suite.ctx, &govProposal)
			suite.Require().NoError(err)

			// check the blocked list with target address list
			curBlockedList := suite.stateDB.GetContractBlockedList()
			suite.Require().Equal(len(tc.targetAddrListToCheck), len(curBlockedList))

			for i, addr := range curBlockedList {
				suite.Require().Equal(tc.targetAddrListToCheck[i], addr)
			}
		})
	}
}
