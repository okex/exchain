package multisig

import (
	//"github.com/tendermint/tendermint/crypto/sr25519"

	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	cryptotypes "github.com/okex/exchain/libs/cosmos-sdk/crypto/types"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/ibc-tx/internal/ed25519"
	"github.com/okex/exchain/libs/cosmos-sdk/x/auth/ibc-tx/internal/secp256k1"
)

// TODO: Figure out API for others to either add their own pubkey types, or
// to make verify / marshal accept a AminoCdc.
const (
	PubKeyAminoRoute = "tendermint/PubKeyMultisigThreshold"
)

//nolint
// Deprecated: Amino is being deprecated in the SDK. But even if you need to
// use Amino for some reason, please use `codec/legacy.Cdc` instead.
var AminoCdc = codec.New()

func init() {
	AminoCdc.RegisterInterface((*cryptotypes.PubKey)(nil), nil)
	AminoCdc.RegisterConcrete(ed25519.PubKey{},
		ed25519.PubKeyName, nil)
	//AminoCdc.RegisterConcrete(sr25519.PubKey{},
	//	sr25519.PubKeyName, nil)
	AminoCdc.RegisterConcrete(&secp256k1.PubKey{},
		secp256k1.PubKeyName, nil)
	AminoCdc.RegisterConcrete(&LegacyAminoPubKey{},
		PubKeyAminoRoute, nil)
}
