package merkle

import (
	"github.com/okex/exchain/dependence/tendermint/crypto/tmhash"
)

// TODO: make these have a large predefined capacity
var (
	leafPrefix  = []byte{0}
	innerPrefix = []byte{1}
)

// returns tmhash(0x00 || leaf)
func leafHash(leaf []byte) []byte {
	return tmhash.Sum(append(leafPrefix, leaf...))
}

// returns tmhash(0x01 || left || right)
func innerHash(left []byte, right []byte) []byte {
	return tmhash.Sum(append(innerPrefix, append(left, right...)...))
}
