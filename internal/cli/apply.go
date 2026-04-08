package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newApplyCmd(deps dependencies) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Materialize sources declared in Tasktree.yml that are not yet on disk",
		Long: `Reads Tasktree.yml and ensures each declared source is present on disk.

Sources whose destination path already exists are skipped without error,
making apply safe to run repeatedly.

Use --dry-run to preview what would be done without making any changes.

Source types other than "git" are not yet implemented and will be skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.applyService.Run(ctx, cwd, app.ApplyOptions{DryRun: dryRun})
			if err != nil {
				return formatError(err)
			}

			if len(result.Results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No sources declared in Tasktree.yml.")
				return nil
			}

			allSkipped := true
			for _, r := range result.Results {
				if r.Status != app.SourceApplyStatusSkipped {
					allSkipped = false
					break
				}
			}
			if allSkipped {
				fmt.Fprintln(cmd.OutOrStdout(), "All sources are already present.")
				return nil
			}

			for _, r := range result.Results {
				sourcePath := r.Source.Path
				if sourcePath == "" {
					sourcePath = r.Source.Name
				}
				switch r.Status {
				case app.SourceApplyStatusSkipped:
					fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s (already present)\n", r.Source.Name)

				case app.SourceApplyStatusUnsupported:
					fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s (source type %q is not yet supported)\n", r.Source.Name, r.Source.Type)

				case app.SourceApplyStatusWouldClone:
					msg := fmt.Sprintf("Would clone %s at %s", r.Source.Name, sourcePath)
					if r.Source.Git != nil {
						if r.Source.Git.Branch != "" {
							msg += fmt.Sprintf(" (branch: %s)", r.Source.Git.Branch)
						} else if r.Source.Git.Ref != "" {
							msg += fmt.Sprintf(" (ref: %s)", r.Source.Git.Ref)
						}
					}
					fmt.Fprintln(cmd.OutOrStdout(), msg)

				case app.SourceApplyStatusCloned:
					switch r.BranchPath {
					case app.BranchPathLocalExisting:
						fmt.Fprintf(cmd.OutOrStdout(), "Using existing local branch %q.\n", r.EffectiveBranch)
					case app.BranchPathRemoteTracking:
						fmt.Fprintf(cmd.OutOrStdout(), "Using existing remote branch %q from origin.\n", r.EffectiveBranch)
					case app.BranchPathCreated:
						fmt.Fprintf(cmd.OutOrStdout(), "Creating new branch %q from %q.\n", r.EffectiveBranch, r.EffectiveFrom)
					case app.BranchPathHeadless:
						if r.Source.Git != nil && r.Source.Git.Ref != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "Checking out %q without creating a branch.\n", r.Source.Git.Ref)
						}
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Cloned %s at %s\n", r.Source.Name, sourcePath)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be done without making changes")
	return cmd
}
