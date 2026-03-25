package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootSubcommand(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "root",
		Short: "Print the current tasktree root",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, err := deps.rootService.Run(cwd)
			if err != nil {
				return formatError(err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), root)
			return err
		},
	}
}
