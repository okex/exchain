package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/okex/exchain/ibc-3rd/cosmos-v443/types"
	"github.com/okex/exchain/ibc-3rd/cosmos-v443/x/distribution/types"
)

func TestValidateGenesis(t *testing.T) {

	fp := types.InitialFeePool()
	require.Nil(t, fp.ValidateGenesis())

	fp2 := types.FeePool{CommunityPool: sdk.DecCoins{{Denom: "stake", Amount: sdk.NewDec(-1)}}}
	require.NotNil(t, fp2.ValidateGenesis())
}
