package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/okex/okexchain/x/ammswap/types"
	"github.com/spf13/cobra"
	"strings"
)

// GetQueryCmd returns the cli query commands for this module
func GetQueryCmd(queryRoute string, cdc *codec.Codec) *cobra.Command {
	// Group swap queries under a subcommand
	swapQueryCmd := &cobra.Command{
		Use:                        "swap",
		Short:                      fmt.Sprintf("Querying commands for the %s module", types.ModuleName),
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	swapQueryCmd.AddCommand(
		flags.GetCommands(
			GetCmdSwapTokenPair(queryRoute, cdc),
			GetCmdQueryParams(queryRoute, cdc),
		)...,
	)

	return swapQueryCmd
}

//GetCmdSwapTokenPair query exchange with token name
func GetCmdSwapTokenPair(queryRoute string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "pool-info [token]",
		Short: "Query pool info by token name",
		Long: 	strings.TrimSpace(
			fmt.Sprintf(`Query pool info by token name.

Example:
$ okexchaincli query swap pool-info eth-355

`),
		),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)
			tokenName := args[0]

			res, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s/%s", queryRoute, types.QuerySwapTokenPair, tokenName), nil)
			if err != nil {
				fmt.Printf("exchange - %s doesn't exist. error:%s \n", tokenName, err.Error())
				return nil
			}

			fmt.Println(string(res))
			return nil
		},
	}
}

func GetCmdQueryParams(queryRoute string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "params",
		Short: "Query the parameters of the AMM swap system",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query the parameters of the AMM swap system.

Example:
$ %s query swap params
`,
				version.ClientName,
			),
		),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)
			tp, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/params", queryRoute), nil)
			if err != nil {
				return err
			}

			var params types.Params
			cdc.MustUnmarshalJSON(tp, &params)

			return cliCtx.PrintOutput(params)
		},
	}
}
