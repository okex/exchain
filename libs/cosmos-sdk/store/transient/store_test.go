package transient

import (
	"github.com/okx/exchain/libs/iavl"
	"testing"

	"github.com/stretchr/testify/require"
)

var k, v = []byte("hello"), []byte("world")

func TestTransientStore(t *testing.T) {
	tstore := NewStore()

	require.Nil(t, tstore.Get(k))

	tstore.Set(k, v)

	require.Equal(t, v, tstore.Get(k))

	tstore.Commit(&iavl.TreeDelta{}, nil)

	require.Nil(t, tstore.Get(k))
}
