package ante

import (
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
)

// AccountBlockedVerificationDecorator check whether signer is blocked.
type AccountBlockedVerificationDecorator struct {
	evmKeeper EVMKeeper
}

// NewAccountBlockedVerificationDecorator creates a new AccountBlockedVerificationDecorator instance
func NewAccountBlockedVerificationDecorator(evmKeeper EVMKeeper) AccountBlockedVerificationDecorator {
	return AccountBlockedVerificationDecorator{
		evmKeeper: evmKeeper,
	}
}

// AnteHandle check wether signer of tx(contains cosmos-tx and eth-tx) is blocked.
func (abvd AccountBlockedVerificationDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	pin := ctx.AntePin()
	if pin != nil {
		pin("AccountBlockedVerificationDecorator", true)
	}
	signers := tx.GetSigners()

	currentGasMeter := ctx.GasMeter()
	ctx = ctx.WithGasMeter(sdk.NewInfiniteGasMeter())

	for _, signer := range signers {
		//TODO it may be optimizate by cache blockedAddressList
		if ok := abvd.evmKeeper.IsAddressBlocked(ctx, signer); ok {
			ctx = ctx.WithGasMeter(currentGasMeter)
			return ctx, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "address: %s has been blocked", signer.String())
		}
	}
	ctx = ctx.WithGasMeter(currentGasMeter)
	if pin != nil {
		pin("AccountBlockedVerificationDecorator", false)
	}
	return next(ctx, tx, simulate)
}
