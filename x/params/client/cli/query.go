package cli

import (
	"fmt"
	"github.com/okex/exchain/libs/cosmos-sdk/client/flags"
	"strconv"
	"strings"

	"github.com/okex/exchain/x/params/types"

	"github.com/okex/exchain/libs/cosmos-sdk/client/context"
	"github.com/okex/exchain/libs/cosmos-sdk/codec"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the cli query commands for this module
func GetQueryCmd(queryRoute string, cdc *codec.Codec) *cobra.Command {
	queryCmd := &cobra.Command{
		Use:   "params",
		Short: "Querying commands for the params module",
	}

	queryCmd.AddCommand(flags.GetCommands(
		GetCmdQueryParams(queryRoute, cdc),
		GetCmdQueryUpgrade(queryRoute, cdc),
		GetCmdQueryGasConfig(queryRoute, cdc),
		GetCmdQueryBlockConfig(queryRoute, cdc),
	)...)

	return queryCmd
}

// GetCmdQueryParams implements the query params command.
func GetCmdQueryParams(queryRoute string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "params",
		Short: "Query parameters of params",
		Long: strings.TrimSpace(`Query parameters of params:

$ exchaincli query params params
`),
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			route := fmt.Sprintf("custom/%s/%s", queryRoute, types.QueryParams)
			bz, _, err := cliCtx.QueryWithData(route, nil)
			if err != nil {
				return err
			}

			var params types.Params
			cdc.MustUnmarshalJSON(bz, &params)
			return cliCtx.PrintOutput(params)
		},
	}
}

// GetCmdQueryParams implements the query params command.
func GetCmdQueryGasConfig(queryRoute string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "gasconfig",
		Short: "Query parameters of gasconfig",
		Long: strings.TrimSpace(`Query parameters of gasconfig:

$ exchaincli query params gasconfig
`),
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			route := fmt.Sprintf("custom/%s/%s", queryRoute, types.QueryGasConfig)
			bz, _, err := cliCtx.QueryWithData(route, nil)
			if err != nil {
				return err
			}

			var params types.GasConfig
			cdc.MustUnmarshalJSON(bz, &params)
			return cliCtx.PrintOutput(params.GasConfig)
		},
	}
}

// GetCmdQueryBlockConfig implements the query params command.
func GetCmdQueryBlockConfig(queryRoute string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "blockconfig",
		Short: "Query parameters of blockconfig",
		Long: strings.TrimSpace(`Query parameters of blockconfig:

$ exchaincli query params blockconfig
`),
		Args: cobra.MinimumNArgs(0),
		RunE: func(_ *cobra.Command, args []string) error {
			height := int64(0)
			if len(args) > 0 {
				var err error
				height, err = strconv.ParseInt(args[0], 10, 64)
				if err != nil {
					return err
				}
			}
			cliCtx := context.NewCLIContext().WithCodec(cdc).WithHeight(height)

			route := fmt.Sprintf("custom/%s/%s", queryRoute, types.QueryBlockConfig)
			bz, _, err := cliCtx.QueryWithData(route, nil)
			if err != nil {
				return err
			}

			var params types.BlockConfig
			cdc.MustUnmarshalJSON(bz, &params)
			return cliCtx.PrintOutput(params)
		},
	}
}
