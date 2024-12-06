package v1

import (
	"github.com/okex/exchain/libs/tendermint/delta/internal"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMakeGetDeltaRequestPath(t *testing.T) {
	tests := []struct {
		base   string
		height int64
		expect string
	}{
		{
			base:   "https://my.space.io",
			height: 3,
			expect: "https://my.space.io" + "/" + apiPathGetDelta + internal.GenDeltaKey(3),
		},
		{
			base:   "https://my.space.io/",
			height: 4,
			expect: "https://my.space.io/" + apiPathGetDelta + internal.GenDeltaKey(4),
		},
	}

	for _, tt := range tests {
		path := MakeGetDeltaRequestPath(tt.base, tt.height)
		assert.Equal(t, tt.expect, path)
	}
}
