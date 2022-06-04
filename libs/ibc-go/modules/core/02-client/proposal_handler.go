package client

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	"github.com/okex/exchain/libs/ibc-go/modules/core/02-client/keeper"
	"github.com/okex/exchain/libs/ibc-go/modules/core/02-client/types"
	govtypes "github.com/okex/exchain/x/gov/types"
)

// NewClientUpdateProposalHandler defines the client update proposal handler
func NewClientUpdateProposalHandler(k keeper.Keeper) govtypes.Handler {
	return func(ctx sdk.Context, content *govtypes.Proposal) sdk.Error {
		cont := content.Content
		if cm39, ok := content.Content.(govtypes.CM39ContentAdapter); ok {
			cc, err := cm39.Conv2CM39Content(k.GetCdcProxy())
			if nil != err {
				return sdkerrors.Wrapf(err, "convert failed")
			}
			cont = cc
		}
		switch c := cont.(type) {
		case *types.ClientUpdateProposal:
			return k.ClientUpdateProposal(ctx, c)
		case *types.UpgradeProposal:
			return k.HandleUpgradeProposal(ctx, c)
		default:
			return sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized ibc proposal content type: %T", c)
		}
	}
}
