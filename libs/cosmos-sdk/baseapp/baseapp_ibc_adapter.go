package baseapp

import (
	"github.com/okex/exchain/libs/cosmos-sdk/codec/types"
	sdk "github.com/okex/exchain/libs/cosmos-sdk/types"
	sdkerrors "github.com/okex/exchain/libs/cosmos-sdk/types/errors"
	abci "github.com/okex/exchain/libs/tendermint/abci/types"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// SetInterfaceRegistry sets the InterfaceRegistry.
func (app *BaseApp) SetInterfaceRegistry(registry types.InterfaceRegistry) {
	app.interfaceRegistry = registry
	app.grpcQueryRouter.SetInterfaceRegistry(registry)
	app.msgServiceRouter.SetInterfaceRegistry(registry)
}

// MountMemoryStores mounts all in-memory KVStores with the BaseApp's internal
// commit multi-store.
func (app *BaseApp) MountMemoryStores(keys map[string]*sdk.MemoryStoreKey) {
	for _, memKey := range keys {
		app.MountStore(memKey, sdk.StoreTypeMemory)
	}
}

func (app *BaseApp) handleQueryGRPC(handler GRPCQueryHandler, req abci.RequestQuery) abci.ResponseQuery {
	ctx, err := app.createQueryContext(req.Height, req.Prove)
	if err != nil {
		return sdkerrors.QueryResult(err)
	}

	res, err := handler(ctx, req)
	if err != nil {
		res = sdkerrors.QueryResult(gRPCErrorToSDKError(err))
		res.Height = req.Height
		return res
	}

	return res
}

func (app *BaseApp) createQueryContext(height int64, prove bool) (sdk.Context, error) {
	if err := checkNegativeHeight(height); err != nil {
		return sdk.Context{}, err
	}

	// when a client did not provide a query height, manually inject the latest
	if height == 0 {
		height = app.LastBlockHeight()
	}

	if height <= 1 && prove {
		return sdk.Context{},
			sdkerrors.Wrap(
				sdkerrors.ErrInvalidRequest,
				"cannot query with proof when height <= 1; please provide a valid height",
			)
	}

	cacheMS, err := app.cms.CacheMultiStoreWithVersion(height)
	if err != nil {
		return sdk.Context{},
			sdkerrors.Wrapf(
				sdkerrors.ErrInvalidRequest,
				"failed to load state at height %d; %s (latest height: %d)", height, err, app.LastBlockHeight(),
			)
	}

	// branch the commit-multistore for safety
	ctx := sdk.NewContext(
		cacheMS, app.checkState.ctx.BlockHeader(), true, app.logger,
	).WithMinGasPrices(app.minGasPrices)

	return ctx, nil
}

func checkNegativeHeight(height int64) error {
	if height < 0 {
		// Reject invalid heights.
		return sdkerrors.Wrap(
			sdkerrors.ErrInvalidRequest,
			"cannot query with height < 0; please provide a valid height",
		)
	}
	return nil
}

func gRPCErrorToSDKError(err error) error {
	status, ok := grpcstatus.FromError(err)
	if !ok {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	switch status.Code() {
	case codes.NotFound:
		return sdkerrors.Wrap(sdkerrors.ErrKeyNotFound, err.Error())
	case codes.InvalidArgument:
		return sdkerrors.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	case codes.FailedPrecondition:
		return sdkerrors.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	case codes.Unauthenticated:
		return sdkerrors.Wrap(sdkerrors.ErrUnauthorized, err.Error())
	default:
		return sdkerrors.Wrap(sdkerrors.ErrUnknownRequest, err.Error())
	}
}

type Interceptor interface {
	Before(req *abci.RequestQuery) error
	After(resp *abci.ResponseQuery)
}

var (
	_ Interceptor = (*FunctionInterceptor)(nil)
)

type FunctionInterceptor struct {
	beforeF func(req *abci.RequestQuery) error
	afterF  func(resp *abci.ResponseQuery)
}

func NewFunctionInterceptor(beforeF func(req *abci.RequestQuery) error, afterF func(resp *abci.ResponseQuery)) *FunctionInterceptor {
	return &FunctionInterceptor{beforeF: beforeF, afterF: afterF}
}

func (f *FunctionInterceptor) Before(req *abci.RequestQuery) error {
	return f.beforeF(req)
}

func (f *FunctionInterceptor) After(resp *abci.ResponseQuery) {
	f.afterF(resp)
}
