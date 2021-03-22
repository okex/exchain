package utils

import (
	"os"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	okexchain "github.com/okex/okexchain/app/types"
	"github.com/okex/okexchain/x/evm/types"
	"github.com/stretchr/testify/require"
)

const (
	expectedJSONString = `{
  "title": "manage contract proposal whitelist with a distributor address",
  "description": "add a distributor address into the whitelist",
  "distributor_addresses": [
    "okexchain1hw4r48aww06ldrfeuq2v438ujnl6alsz0685a0",
    "okexchain1qj5c07sm6jetjz8f509qtrxgh4psxkv32x0qas"
  ],
  "is_added": true,
  "deposit": [
    {
      "denom": "okt",
      "amount": "100.000000000000000000"
    }
  ]
}`
	fileName                 = "./proposal.json"
	expectedTitle            = "manage contract proposal whitelist with a distributor address"
	expectedDescription      = "add a distributor address into the whitelist"
	expectedDistributorAddr1 = "okexchain1hw4r48aww06ldrfeuq2v438ujnl6alsz0685a0"
	expectedDistributorAddr2 = "okexchain1qj5c07sm6jetjz8f509qtrxgh4psxkv32x0qas"
)

func init() {
	config := sdk.GetConfig()
	okexchain.SetBech32Prefixes(config)
}

func TestParseManageContractDeploymentWhitelistProposalJSON(t *testing.T) {
	// create JSON file
	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0666)
	require.NoError(t, err)
	_, err = f.WriteString(expectedJSONString)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// remove the temporary JSON file
	defer os.Remove(fileName)

	proposal, err := ParseManageContractDeploymentWhitelistProposalJSON(types.ModuleCdc, fileName)
	require.NoError(t, err)
	require.Equal(t, expectedTitle, proposal.Title)
	require.Equal(t, expectedDescription, proposal.Description)
	require.True(t, proposal.IsAdded)
	require.Equal(t, 1, len(proposal.Deposit))
	require.Equal(t, sdk.DefaultBondDenom, proposal.Deposit[0].Denom)
	require.True(t, sdk.NewDec(100).Equal(proposal.Deposit[0].Amount))
	require.Equal(t, 2, len(proposal.DistributorAddrs))
	require.Equal(t, expectedDistributorAddr1, proposal.DistributorAddrs[0].String())
	require.Equal(t, expectedDistributorAddr2, proposal.DistributorAddrs[1].String())
}
