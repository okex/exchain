package app

import (
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"github.com/okex/exchain/libs/tendermint/libs/log"
	dbm "github.com/okex/exchain/libs/tm-db"
)

type Option func(option *SetupOption)

type SetupOption struct {
	chainId string
}

func WithChainId(chainId string) Option {
	return func(option *SetupOption) {
		option.chainId = chainId
	}
}

// Setup initializes a new OKExChainApp. A Nop logger is set in OKExChainApp.
func Setup(isCheckTx bool, options ...Option) *OKExChainApp {
	db := dbm.NewMemDB()
	app := NewOKExChainApp(log.NewNopLogger(), db, nil, true, map[int64]bool{}, 0)

	if !isCheckTx {
		setupOption := &SetupOption{chainId: ""}
		for _, opt := range options {
			opt(setupOption)
		}
		// init chain must be called to stop deliverState from being nil
		genesisState := NewDefaultGenesisState()
		stateBytes, err := codec.MarshalJSONIndent(app.Codec(), genesisState)
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		app.InitChain(
			abci.RequestInitChain{
				Validators:    []abci.ValidatorUpdate{},
				AppStateBytes: stateBytes,
				ChainId:       setupOption.chainId,
			},
		)
	}

	return app
}
