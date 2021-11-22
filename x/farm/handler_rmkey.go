package farm

import (
	ethcmn "github.com/ethereum/go-ethereum/common"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/x/farm/keeper"
	"github.com/okex/exchain/x/farm/types"
)

func init() {
	destroyPoolHandler = handleMsgRmKeys
}

func handleMsgRmKeys(ctx sdk.Context, k keeper.Keeper, msg types.MsgDestroyPool) (*sdk.Result, error) {
	evmKeeper := k.EvmKeeper()
	err := evmKeeper.ForEachStorage(ctx, msg.Contract, func(key, value ethcmn.Hash) bool {
		evmKeeper.DeleteStateDirectly(ctx, msg.Contract, key)
		return false // todo: need to add a judgement, in case of deleting too many keys in one transaction
	})
	if err != nil {
		return nil, err
	}
	return &sdk.Result{}, nil
}
