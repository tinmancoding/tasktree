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
	var skipBootstrap bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Materialize sources declared in Tasktree.yml that are not yet on disk",
		Long: `Reads Tasktree.yml and ensures each declared source is present on disk.

Sources whose destination path already exists are skipped without error,
making apply safe to run repeatedly.

After all sources are materialized, the workspace-level bootstrap steps run
(on every apply). Use --skip-bootstrap to materialize sources only.

Use --dry-run to preview what would be done without making any changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.applyService.Run(ctx, cwd, app.ApplyOptions{
				DryRun:        dryRun,
				SkipBootstrap: skipBootstrap,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return formatError(err)
			}

			printSourceSummary(cmd, result)
			printBootstrapSummary(cmd, result, dryRun)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be done without making changes")
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "Materialize sources only; do not run bootstrap steps")
	return cmd
}

func printSourceSummary(cmd *cobra.Command, result app.ApplyResult) {
	if len(result.Results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No sources declared in Tasktree.yml.")
		return
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
		return
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
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s (unknown source type %q)\n", r.Source.Name, r.Source.Type)

		case app.SourceApplyStatusWouldApply:
			msg := fmt.Sprintf("Would apply %s at %s", r.Source.Name, sourcePath)
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

		case app.SourceApplyStatusCreated:
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s\n", r.Source.Name, sourcePath)

		case app.SourceApplyStatusLinked:
			fmt.Fprintf(cmd.OutOrStdout(), "Linked %s at %s\n", r.Source.Name, sourcePath)

		case app.SourceApplyStatusCopied:
			fmt.Fprintf(cmd.OutOrStdout(), "Copied %s at %s\n", r.Source.Name, sourcePath)

		case app.SourceApplyStatusDownloaded:
			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %s at %s\n", r.Source.Name, sourcePath)

		case app.SourceApplyStatusExtracted:
			fmt.Fprintf(cmd.OutOrStdout(), "Extracted %s at %s\n", r.Source.Name, sourcePath)
		}
	}
}

func printBootstrapSummary(cmd *cobra.Command, result app.ApplyResult, dryRun bool) {
	if !dryRun || len(result.BootstrapPlan) == 0 {
		return
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Bootstrap plan:")
	for i, step := range result.BootstrapPlan {
		line := fmt.Sprintf("  %d. %s: %s", i+1, step.Name, step.Run)
		if step.Workdir != "" {
			line += fmt.Sprintf(" (workdir: %s)", step.Workdir)
		}
		fmt.Fprintln(out, line)
		if step.WorkdirError != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", step.WorkdirError)
		} else if !step.WorkdirExists {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: bootstrap step %q: workdir %q does not exist\n", step.Name, step.Workdir)
		}
	}
}
