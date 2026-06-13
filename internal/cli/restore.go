package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newRestoreCmd(deps dependencies) *cobra.Command {
	var into string
	var skipBootstrap bool

	cmd := &cobra.Command{
		Use:   "restore <snapshot>",
		Short: "Reproduce a workspace from a snapshot .tar.gz",
		Long: `Reproduces the exact working state captured by 'tasktree snapshot' onto a
fresh directory: re-materializes sources pinned to the recorded commits,
replays local commits, and restores dirty edits. Bootstrap steps run after
restore (use --skip-bootstrap to skip).

Pass - to read the snapshot from stdin. The target defaults to ./<name>
from the embedded spec; use --into to choose a directory. The target must be
empty or not yet exist.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			var input io.Reader
			if args[0] == "-" {
				input = cmd.InOrStdin()
			} else {
				f, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("open snapshot: %w", err)
				}
				defer func() { _ = f.Close() }()
				input = f
			}

			result, err := deps.restoreService.Run(ctx, cwd, app.RestoreOptions{
				Input:         input,
				Into:          into,
				SkipBootstrap: skipBootstrap,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return formatError(err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Restored %q to %s\n", result.Tasktree, result.Target)
			return nil
		},
	}

	cmd.Flags().StringVar(&into, "into", "", "Target directory (default: ./<tasktree-name>)")
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "Restore sources only; do not run bootstrap steps")
	return cmd
}
