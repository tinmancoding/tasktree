package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInitCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a tasktree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			root, err := deps.initService.Run(target)
			if err != nil {
				return formatError(err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Initialized tasktree at %s\n", root)
			return err
		},
	}
}
