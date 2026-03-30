package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPruneCmd(deps dependencies) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove stale tasktrees from the global registry",
		Long: `Remove registry entries whose paths no longer exist on disk or no longer
contain a .tasktree.toml file.

Use --dry-run to preview what would be removed without making any changes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := deps.pruneService.Run(dryRun)
			if err != nil {
				return formatError(err)
			}
			if len(results) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Nothing to prune.")
				return err
			}
			for _, r := range results {
				if dryRun {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Would remove %s (%s, %s)\n", r.Name, r.Path, r.Status); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed %s (%s)\n", r.Name, r.Path); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview removals without modifying the registry")
	return cmd
}
