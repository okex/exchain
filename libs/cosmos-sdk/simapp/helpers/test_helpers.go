package helpers

import (
	"math/rand"
	"time"

	"github.com/okex/exchain/libs/tendermint/crypto"

	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth"
	"github.com/okex/exchain/libs/cosmos-sdk/x/simulation"
)

// SimAppChainID hardcoded chainID for simulation
const (
	DefaultGenTxGas = 1000000
	SimAppChainID   = "simulation-app"
)

// GenTx generates a signed mock transaction.
func GenTx(msgs []sdk.Msg, feeAmt sdk.Coins, gas uint64, chainID string, accnums []uint64, seq []uint64, priv ...crypto.PrivKey) *auth.StdTx {
	fee := auth.StdFee{
		Amount: feeAmt,
		Gas:    gas,
	}

	sigs := make([]auth.StdSignature, len(priv))

	// create a random length memo
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	memo := simulation.RandStringOfLength(r, simulation.RandIntBetween(r, 0, 100))
	memo = ""

	for i, p := range priv {
		// use a empty chainID for ease of testing
		sig, err := p.Sign(auth.StdSignBytes(chainID, accnums[i], seq[i], fee, msgs, memo))
		if err != nil {
			panic(err)
		}

		sigs[i] = auth.StdSignature{
			PubKey:    p.PubKey(),
			Signature: sig,
		}
	}

	return auth.NewStdTx(msgs, fee, sigs, memo)
}
