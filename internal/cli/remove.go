package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRemoveCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a repository from the current tasktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			removedPath, err := deps.removeService.Run(cwd, args[0])
			if err != nil {
				return formatError(err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", removedPath)
			return err
		},
	}
}
