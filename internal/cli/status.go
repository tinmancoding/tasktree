package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/output"
)

func newStatusCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show live status for repositories in the current tasktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.statusService.Run(context.Background(), cwd)
			if err != nil {
				return formatError(err)
			}
			rows := make([]struct {
				Name  string
				Path  string
				Head  string
				State string
			}, 0, len(result.Repos))
			for _, repo := range result.Repos {
				rows = append(rows, struct {
					Name  string
					Path  string
					Head  string
					State string
				}{Name: repo.Name, Path: repo.Path, Head: repo.Head, State: repo.State})
			}
			return output.WriteStatusTable(cmd.OutOrStdout(), result.TasktreeName, result.Root, rows)
		},
	}
}
