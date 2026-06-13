package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newSnapshotCmd(deps dependencies) *cobra.Command {
	var output string
	var includeIgnored bool

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture the workspace's concrete state into a portable .tar.gz",
		Long: `Captures a lightweight, self-contained snapshot of the workspace's concrete
state: per-git-source base pins, local commits (git bundle), and dirty
working-tree edits. The result is a single portable .tar.gz that 'tasktree
restore' can reproduce on a fresh machine.

By default ignored files are excluded; use --include-ignored to capture them.
Use -o - to stream the snapshot to stdout.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			// Stream to stdout.
			if output == "-" {
				_, err := deps.snapshotService.Run(ctx, cwd, app.SnapshotOptions{
					IncludeIgnored: includeIgnored,
					Output:         cmd.OutOrStdout(),
				})
				if err != nil {
					return formatError(err)
				}
				return nil
			}

			// Write to a temp file in the destination dir, then atomically rename.
			destDir := cwd
			if output != "" {
				destDir = filepath.Dir(output)
			}
			tmp, err := os.CreateTemp(destDir, ".tasktree-snapshot-*.tmp")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}
			tmpPath := tmp.Name()
			cleanup := true
			defer func() {
				if cleanup {
					_ = os.Remove(tmpPath)
				}
			}()

			result, err := deps.snapshotService.Run(ctx, cwd, app.SnapshotOptions{
				IncludeIgnored: includeIgnored,
				Output:         tmp,
			})
			if err != nil {
				_ = tmp.Close()
				return formatError(err)
			}
			if err := tmp.Close(); err != nil {
				return err
			}

			finalPath := output
			if finalPath == "" {
				name := result.Tasktree
				if name == "" {
					name = "tasktree"
				}
				finalPath = filepath.Join(cwd, fmt.Sprintf("%s-%s.tar.gz", name, time.Now().UTC().Format("20060102T150405Z")))
			}
			if err := os.Rename(tmpPath, finalPath); err != nil {
				return fmt.Errorf("write snapshot: %w", err)
			}
			cleanup = false

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote snapshot to %s (%d sources)\n", finalPath, len(result.Sources))
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output path for the snapshot (.tar.gz); use - for stdout")
	cmd.Flags().BoolVar(&includeIgnored, "include-ignored", false, "Capture .gitignore'd files in the dirty archive")
	return cmd
}
