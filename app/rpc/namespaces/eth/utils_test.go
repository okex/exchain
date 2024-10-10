package eth

import (
	"testing"

	evmtypes "github.com/okex/exchain/x/evm/types"

	"github.com/stretchr/testify/require"
)

func Test_TransformDataError(t *testing.T) {

	sdkerr := newWrappedCosmosError(7, `["execution reverted","message","HexData","0x00000000000"]:failed message tail`, evmtypes.ModuleName)
	err := TransformDataError(sdkerr, "eth_estimateGas").(DataError)
	require.NotNil(t, err.ErrorData())
	require.Equal(t, err.Error(), `["execution reverted","message","HexData","0x00000000000"]`)
	require.Equal(t, err.ErrorData(), RPCNullData)
	require.Equal(t, err.ErrorCode(), DefaultEVMErrorCode)
	err = TransformDataError(sdkerr, "eth_call").(DataError)
	require.NotNil(t, err.ErrorData())
	require.Equal(t, err.Error(), `["execution reverted","message","HexData","0x00000000000"]`)
	require.Equal(t, err.ErrorData(), RPCNullData)
	require.Equal(t, err.ErrorCode(), DefaultEVMErrorCode)
}
