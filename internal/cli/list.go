package cli

import (
	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/output"
)

func newListCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all known tasktrees on this machine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := deps.listTasktreesService.Run()
			if err != nil {
				return formatError(err)
			}
			rows := make([]output.TasktreeRow, len(entries))
			for i, e := range entries {
				row := output.TasktreeRow{
					Name: e.Name,
					Path: e.Path,
				}
				if e.Status != app.TasktreeStatusOK {
					row.Status = string(e.Status)
				}
				rows[i] = row
			}
			return output.WriteTasktreeTable(cmd.OutOrStdout(), rows)
		},
	}
}
