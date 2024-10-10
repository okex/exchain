package sanity

import (
	"testing"

	"github.com/spf13/cobra"

	apptype "github.com/okex/exchain/app/types"
	"github.com/okex/exchain/libs/cosmos-sdk/server"
	"github.com/okex/exchain/libs/cosmos-sdk/store/types"
	"github.com/okex/exchain/libs/tendermint/consensus"
	"github.com/okex/exchain/libs/tendermint/state"
	sm "github.com/okex/exchain/libs/tendermint/state"
	ttypes "github.com/okex/exchain/libs/tendermint/types"
	"github.com/okex/exchain/x/evm/watcher"
)

func getCommandNodeModeRpcPruningNothing() *cobra.Command {
	return getCommand([]universeFlag{
		&stringFlag{
			Name:    apptype.FlagNodeMode,
			Default: "",
			Changed: true,
			Value:   string(apptype.RpcNode),
		},
		&stringFlag{
			Name:    server.FlagPruning,
			Default: types.PruningOptionDefault,
			Changed: true,
			Value:   types.PruningOptionNothing,
		},
	})
}

func getCommandFastQueryPruningNothing() *cobra.Command {
	return getCommand([]universeFlag{
		&boolFlag{
			Name:    watcher.FlagFastQuery,
			Default: false,
			Changed: true,
			Value:   true,
		},
		&stringFlag{
			Name:    server.FlagPruning,
			Default: "",
			Changed: true,
			Value:   types.PruningOptionNothing,
		},
	})
}

func getCommandEnablePreruntxDownloadDelta() *cobra.Command {
	return getCommand([]universeFlag{
		&boolFlag{
			Name:    consensus.EnablePrerunTx,
			Default: false,
			Changed: true,
			Value:   true,
		},
		&boolFlag{
			Name:    ttypes.FlagDownloadDDS,
			Default: false,
			Changed: true,
			Value:   true,
		},
	})
}

func getCommandDeliverTxsExecModeSerial(v int) *cobra.Command {
	return getCommand([]universeFlag{
		&intFlag{
			Name:    sm.FlagDeliverTxsExecMode,
			Default: 0,
			Changed: true,
			Value:   v,
		},
	})
}

func TestCheckStart(t *testing.T) {
	tests := []struct {
		name    string
		cmdFunc func()
		wantErr bool
	}{
		{name: "range-TxsExecModeSerial 0", cmdFunc: func() { getCommandDeliverTxsExecModeSerial(int(state.DeliverTxsExecModeSerial)) }, wantErr: false},
		{name: "range-TxsExecModeSerial 1", cmdFunc: func() { getCommandDeliverTxsExecModeSerial(1) }, wantErr: true},
		{name: "range-TxsExecModeSerial 2", cmdFunc: func() { getCommandDeliverTxsExecModeSerial(state.DeliverTxsExecModeParallel) }, wantErr: false},
		{name: "range-TxsExecModeSerial 3", cmdFunc: func() { getCommandDeliverTxsExecModeSerial(3) }, wantErr: true},
		{name: "1. conflicts --fast-query and --pruning=nothing", cmdFunc: func() { getCommandFastQueryPruningNothing() }, wantErr: true},
		{name: "2. conflicts --enable-preruntx and --download-delta", cmdFunc: func() { getCommandEnablePreruntxDownloadDelta() }, wantErr: true},
		{name: "3. conflicts --node-mod=rpc and --pruning=nothing", cmdFunc: func() { getCommandNodeModeRpcPruningNothing() }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.cmdFunc()
			if err = CheckStart(nil); (err != nil) != tt.wantErr {
				t.Errorf("CheckStart() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Log(err)
		})
	}
}

func getCommandNodeModeArchiveFastQuery() *cobra.Command {
	return getCommand([]universeFlag{
		&stringFlag{
			Name:    apptype.FlagNodeMode,
			Default: "",
			Changed: true,
			Value:   string(apptype.ArchiveNode),
		},
		&boolFlag{
			Name:    watcher.FlagFastQuery,
			Default: false,
			Changed: true,
			Value:   true,
		},
	})
}

func TestCheckStartArchive(t *testing.T) {
	type args struct {
		cmd *cobra.Command
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "1. conflicts --node-mod=archive and --fast-query", args: args{cmd: getCommandNodeModeArchiveFastQuery()}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if err = CheckStart(nil); (err != nil) != tt.wantErr {
				t.Errorf("CheckStart() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Log(err)
		})
	}
}
