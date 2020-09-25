package keeper

import (
	"github.com/cosmos/cosmos-sdk/x/supply"
	"github.com/okex/okexchain/x/token"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	swap "github.com/okex/okexchain/x/ammswap/keeper"
	"github.com/okex/okexchain/x/farm/types"
)

// Keeper of the farm store
type Keeper struct {
	storeKey     sdk.StoreKey
	cdc          *codec.Codec
	paramspace   types.ParamSubspace
	supplyKeeper supply.Keeper
	tokenKeeper  token.Keeper
	swapKeeper   swap.Keeper
}

// NewKeeper creates a farm keeper
func NewKeeper(
	cdc *codec.Codec, key sdk.StoreKey, paramspace types.ParamSubspace,
	supplyKeeper supply.Keeper, tokenKeeper token.Keeper,
) Keeper {
	keeper := Keeper{
		storeKey:     key,
		cdc:          cdc,
		paramspace:   paramspace.WithKeyTable(types.ParamKeyTable()),
		supplyKeeper: supplyKeeper,
		tokenKeeper:  tokenKeeper,
	}
	return keeper
}

func (k Keeper) StoreKey() sdk.StoreKey {
	return k.storeKey
}

func (k Keeper) SupplyKeeper() supply.Keeper {
	return k.supplyKeeper
}
