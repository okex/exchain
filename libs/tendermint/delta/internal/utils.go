package internal

import (
	"fmt"

	"github.com/okex/exchain/libs/tendermint/types"
)

func GenDeltaKey(height int64) string {
	return fmt.Sprintf("DH-%d:%d", types.DeltaVersion, height)
}
