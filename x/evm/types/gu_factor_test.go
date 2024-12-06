package types

import (
	"fmt"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMarshalGuFactor(t *testing.T) {
	str := "{\"gu_factor\":\"6000.000000000000000000\"}"
	factor, err := UnmarshalGuFactor(str)
	require.NoError(t, err)

	result := factor.Factor.MulInt(sdk.NewIntFromUint64(1220)).TruncateInt().Uint64()
	t.Log("result", result)
	require.Equal(t, uint64(7320000), result)

	t.Log("-1", sdk.NewDec(-1).String(), sdk.NewDec(-1).IsNegative())
	str = "{\"gu_factor1\":\"6000.000000000000000000\"}"
	factor, err = UnmarshalGuFactor(str)
	t.Log(fmt.Sprintf("errkey %s", err))

	str = "{\"\":\"6000.000000000000000000\"}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("nonekey %s", err))

	str = "{}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("nonedata %s", err))

	str = "--"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("err data %s", err))

	str = "{\"gu_factor\":\"6000.000000000000000000\",1}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("errjson %s", err))

	str = "{\"gu_factor\":\"6000.0000000000000000000000\"}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("6000.0000000000000000000000 more lenght errnumber %s", err))

	str = "{\"gu_factor\":\"600.0.0000000000000000000000\"}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("errnumber %s", err))

	str = "{\"gu_factor\":\"a\"}"
	factor, err = UnmarshalGuFactor(str)
	require.Error(t, err)
	t.Log(fmt.Sprintf("not a number %s", err))

	str = "{\"gu_factor\":\"6000.000000000000000000\",\"gu_factor1\":\"599.000000000000000000\"}"
	factor, err = UnmarshalGuFactor(str)
	require.NoError(t, err)
	t.Log("in64()", factor.Factor.Int64(), "TruncateInt64", factor.Factor.TruncateInt64(), "TruncateInt().Uint64", factor.Factor.TruncateInt().Uint64())
	require.Equal(t, factor.Factor.TruncateInt().Uint64(), uint64(6000))

	var ff GuFactor
	require.NotNil(t, ff)
	require.NotNil(t, ff.Factor)
	require.Nil(t, ff.Factor.Int)
}
