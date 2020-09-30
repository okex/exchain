package farm

import (
	"github.com/okex/okexchain/x/farm/keeper"
	"github.com/okex/okexchain/x/farm/types"
)

const (
	StoreKey            = types.StoreKey
	DefaultParamspace   = types.DefaultParamspace
	DefaultCodespace    = types.DefaultCodespace
	ModuleName          = types.ModuleName
	MintFarmingAccount  = types.MintFarmingAccount
	YieldFarmingAccount = types.YieldFarmingAccount
)

var (
	NewKeeper = keeper.NewKeeper
)

type (
	Keeper = keeper.Keeper
)
