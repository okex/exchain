package v100

import (
	"fmt"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	codectypes "github.com/okex/exchain/libs/cosmos-sdk/codec/types"
	"github.com/okex/exchain/libs/cosmos-sdk/store/prefix"
	store "github.com/okex/exchain/libs/cosmos-sdk/store/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	clienttypes "github.com/okex/exchain/libs/ibc-go/modules/core/02-client/types"
	host "github.com/okex/exchain/libs/ibc-go/modules/core/24-host"
	"github.com/okex/exchain/libs/ibc-go/modules/core/exported"
	"github.com/okex/exchain/libs/ibc-go/modules/core/types"
	smtypes "github.com/okex/exchain/libs/ibc-go/modules/light-clients/06-solomachine/types"
	ibctmtypes "github.com/okex/exchain/libs/ibc-go/modules/light-clients/07-tendermint/types"
	"strings"
)

// MigrateStore performs in-place store migrations from SDK v0.40 of the IBC module to v1.0.0 of ibc-go.
// The migration includes:
//
// - Migrating solo machine client states from v1 to v2 protobuf definition
// - Pruning all solo machine consensus states
// - Pruning expired tendermint consensus states
// - Adds ProcessedHeight and Iteration keys for unexpired tendermint consensus states
func MigrateStore(ctx sdk.Context, storeKey store.StoreKey, cdc *codec.CodecProxy) (err error) {
	store := ctx.KVStore(storeKey)
	iterator := sdk.KVStorePrefixIterator(store, host.KeyClientStorePrefix)

	var clients []string

	// collect all clients
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		keySplit := strings.Split(string(iterator.Key()), "/")
		if keySplit[len(keySplit)-1] != host.KeyClientState {
			continue
		}

		// key is clients/{clientid}/clientState
		// Thus, keySplit[1] is clientID
		clients = append(clients, keySplit[1])
	}

	for _, clientID := range clients {
		clientType, _, err := types.ParseClientIdentifier(clientID)
		if err != nil {
			return err
		}

		clientPrefix := []byte(fmt.Sprintf("%s/%s/", host.KeyClientStorePrefix, clientID))
		clientStore := prefix.NewStore(ctx.KVStore(storeKey), clientPrefix)

		bz := clientStore.Get(host.ClientStateKey())
		if bz == nil {
			return clienttypes.ErrClientNotFound
		}

		switch clientType {
		case exported.Solomachine:
			any := &codectypes.Any{}
			if err := cdc.GetProtocMarshal().UnmarshalBinaryBare(bz, any); err != nil {
				return sdkerrors.Wrap(err, "failed to unmarshal client state bytes into solo machine client state")
			}

			clientState := &ClientState{}
			if err := cdc.GetProtocMarshal().UnmarshalBinaryBare(any.Value, clientState); err != nil {
				return sdkerrors.Wrap(err, "failed to unmarshal client state bytes into solo machine client state")
			}

			updatedClientState := migrateSolomachine(clientState)

			bz, err := clienttypes.MarshalClientState(cdc, updatedClientState)
			if err != nil {
				return sdkerrors.Wrap(err, "failed to unmarshal client state bytes into solo machine client state")
			}

			// update solomachine in store
			clientStore.Set(host.ClientStateKey(), bz)

			pruneSolomachineConsensusStates(clientStore)

		case exported.Tendermint:
			var clientState exported.ClientState
			if err := cdc.GetProtocMarshal().UnmarshalInterface(bz, &clientState); err != nil {
				return sdkerrors.Wrap(err, "failed to unmarshal client state bytes into tendermint client state")
			}

			tmClientState, ok := clientState.(*ibctmtypes.ClientState)
			if !ok {
				return sdkerrors.Wrap(clienttypes.ErrInvalidClient, "client state is not tendermint even though client id contains 07-tendermint")
			}

			// add iteration keys so pruning will be successful
			if err = addConsensusMetadata(ctx, clientStore, cdc, tmClientState); err != nil {
				return err
			}

			if err = ibctmtypes.PruneAllExpiredConsensusStates(ctx, clientStore, cdc, tmClientState); err != nil {
				return err
			}

		default:
			continue
		}
	}

	return nil
}

// migrateSolomachine migrates the solomachine from v1 to v2 solo machine protobuf defintion.
func migrateSolomachine(clientState *ClientState) *smtypes.ClientState {
	isFrozen := clientState.FrozenSequence != 0
	consensusState := &smtypes.ConsensusState{
		PublicKey:   clientState.ConsensusState.PublicKey,
		Diversifier: clientState.ConsensusState.Diversifier,
		Timestamp:   clientState.ConsensusState.Timestamp,
	}

	return &smtypes.ClientState{
		Sequence:                 clientState.Sequence,
		IsFrozen:                 isFrozen,
		ConsensusState:           consensusState,
		AllowUpdateAfterProposal: clientState.AllowUpdateAfterProposal,
	}
}

// pruneSolomachineConsensusStates removes all solomachine consensus states from the
// client store.
func pruneSolomachineConsensusStates(clientStore sdk.KVStore) {
	iterator := sdk.KVStorePrefixIterator(clientStore, []byte(host.KeyConsensusStatePrefix))
	var heights []exported.Height

	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		keySplit := strings.Split(string(iterator.Key()), "/")
		// key is in the format "consensusStates/<height>"
		if len(keySplit) != 2 || keySplit[0] != string(host.KeyConsensusStatePrefix) {
			continue
		}

		// collect consensus states to be pruned
		heights = append(heights, clienttypes.MustParseHeight(keySplit[1]))
	}

	// delete all consensus states
	for _, height := range heights {
		clientStore.Delete(host.ConsensusStateKey(height))
	}
}

// addConsensusMetadata adds the iteration key and processed height for all tendermint consensus states
// These keys were not included in the previous release of the IBC module. Adding the iteration keys allows
// for pruning iteration.
func addConsensusMetadata(ctx sdk.Context, clientStore sdk.KVStore, cdc *codec.CodecProxy, clientState *ibctmtypes.ClientState) error {
	var heights []exported.Height
	iterator := sdk.KVStorePrefixIterator(clientStore, []byte(host.KeyConsensusStatePrefix))

	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		keySplit := strings.Split(string(iterator.Key()), "/")
		// consensus key is in the format "consensusStates/<height>"
		if len(keySplit) != 2 {
			continue
		}

		heights = append(heights, clienttypes.MustParseHeight(keySplit[1]))
	}

	for _, height := range heights {
		// set the iteration key and processed height
		// these keys were not included in the SDK v0.42.0 release
		ibctmtypes.SetProcessedHeight(clientStore, height, clienttypes.GetSelfHeight(ctx))
		ibctmtypes.SetIterationKey(clientStore, height)
	}

	return nil
}
