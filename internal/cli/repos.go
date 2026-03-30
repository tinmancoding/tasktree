package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/output"
)

func newReposCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "List repositories in the current tasktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			_, file, err := deps.listService.Run(cwd)
			if err != nil {
				return formatError(err)
			}
			return output.WriteRepoTable(cmd.OutOrStdout(), file.Repos)
		},
	}
}
