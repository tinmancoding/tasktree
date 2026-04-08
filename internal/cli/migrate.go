package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newMigrateCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate [path]",
		Short: "Convert a legacy .tasktree.toml to Tasktree.yml",
		Long: `Reads .tasktree.toml in the given directory (defaults to current directory),
converts it to the new Tasktree.yml format, and renames the old file to .tasktree.toml.bak.

Resolved state fields (resolved_ref, commit) are intentionally discarded.
Live state is always queried from Git checkouts directly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}

			absRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			if root != "." {
				absRoot = root
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Found .tasktree.toml in %s.\n\n", absRoot)
			fmt.Fprintln(cmd.OutOrStdout(), "Converting to Tasktree.yml...")
			fmt.Fprintln(cmd.OutOrStdout(), "")

			result, err := deps.migrateService.Run(absRoot)
			if err != nil {
				return formatError(err)
			}

			for _, source := range result.Sources {
				detail := source.URL
				if source.Branch != "" {
					detail += fmt.Sprintf("  (branch: %s)", source.Branch)
				} else if source.Ref != "" {
					detail += fmt.Sprintf("  (ref: %s)", source.Ref)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-20s git  %s\n", source.Name, detail)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Note: resolved_ref and commit fields are not carried over.")
			fmt.Fprintln(cmd.OutOrStdout(), "      Live state is always queried from the Git checkouts directly.")
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "Written: %s\n", result.NewPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed: .tasktree.toml → .tasktree.toml.bak\n")
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "Migration complete. Review Tasktree.yml and commit it to version control.")
			return nil
		},
	}
}
