package types

import (
	"math/rand"
	"testing"
	"time"

	"github.com/okex/exchain/libs/cosmos-sdk/x/gov/types"
	exgovtypes "github.com/okex/exchain/x/gov/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ProposalSuite struct {
	suite.Suite
}

func TestProposalSuite(t *testing.T) {
	suite.Run(t, new(ProposalSuite))
}

func RandStr(length int) string {
	str := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	bytes := []byte(str)
	result := []byte{}
	rand.Seed(time.Now().UnixNano() + int64(rand.Intn(100)))
	for i := 0; i < length; i++ {
		result = append(result, bytes[rand.Intn(len(bytes))])
	}
	return string(result)
}

func (suite *ProposalSuite) TestNewChangeDistributionTypeProposal() {
	testCases := []struct {
		title               string
		proposalTitle       string
		proposalDescription string
		distrType           uint32
		err                 error
	}{
		{
			"no proposal title",
			"",
			"description",
			0,
			exgovtypes.ErrInvalidProposalContent("title is required"),
		},
		{
			"gt max proposal title length",
			RandStr(types.MaxTitleLength + 1),
			"description",
			0,
			exgovtypes.ErrInvalidProposalContent("title length is bigger than max title length"),
		},
		{
			"gt max proposal title length",
			RandStr(types.MaxTitleLength),
			"",
			0,
			exgovtypes.ErrInvalidProposalContent("description is required"),
		},
		{
			"gt max proposal description length",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength + 1),
			0,
			exgovtypes.ErrInvalidProposalContent("description length is bigger than max description length"),
		},
		{
			"error type",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength),
			2,
			ErrInvalidDistributionType(),
		},
		{
			"normal, type 0",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength),
			0,
			nil,
		},
		{
			"normal, type 1",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength),
			1,
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.title, func() {
			title := tc.proposalTitle
			description := tc.proposalDescription
			proposal := NewChangeDistributionTypeProposal(title, description, tc.distrType)

			require.Equal(suite.T(), title, proposal.GetTitle())
			require.Equal(suite.T(), description, proposal.GetDescription())
			require.Equal(suite.T(), RouterKey, proposal.ProposalRoute())
			require.Equal(suite.T(), ProposalTypeChangeDistributionType, proposal.ProposalType())
			require.NotPanics(suite.T(), func() {
				_ = proposal.String()
			})

			err := proposal.ValidateBasic()
			require.Equal(suite.T(), tc.err, err)
		})
	}
}

func (suite *ProposalSuite) TestNewWithdrawRewardEnabledProposal() {
	testCases := []struct {
		title               string
		proposalTitle       string
		proposalDescription string
		enabled             bool
		err                 error
	}{
		{
			"no proposal title",
			"",
			"description",
			true,
			exgovtypes.ErrInvalidProposalContent("title is required"),
		},
		{
			"gt max proposal title length",
			RandStr(types.MaxTitleLength + 1),
			"description",
			true,
			exgovtypes.ErrInvalidProposalContent("title length is bigger than max title length"),
		},
		{
			"gt max proposal title length",
			RandStr(types.MaxTitleLength),
			"",
			true,
			exgovtypes.ErrInvalidProposalContent("description is required"),
		},
		{
			"gt max proposal description length",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength + 1),
			true,
			exgovtypes.ErrInvalidProposalContent("description length is bigger than max description length"),
		},
		{
			"normal, enabled true",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength),
			true,
			nil,
		},
		{
			"normal, enabled false",
			RandStr(types.MaxTitleLength),
			RandStr(types.MaxDescriptionLength),
			false,
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.title, func() {
			title := tc.proposalTitle
			description := tc.proposalDescription
			proposal := NewWithdrawRewardEnabledProposal(title, description, tc.enabled)

			require.Equal(suite.T(), title, proposal.GetTitle())
			require.Equal(suite.T(), description, proposal.GetDescription())
			require.Equal(suite.T(), RouterKey, proposal.ProposalRoute())
			require.Equal(suite.T(), ProposalTypeWithdrawRewardEnabled, proposal.ProposalType())
			require.NotPanics(suite.T(), func() {
				_ = proposal.String()
			})

			err := proposal.ValidateBasic()
			require.Equal(suite.T(), tc.err, err)
		})
	}
}
