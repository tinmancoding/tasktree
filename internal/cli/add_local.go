package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddLocalCmd(deps dependencies) *cobra.Command {
	var name, path string
	var copy bool

	cmd := &cobra.Command{
		Use:   "local <src-path>",
		Short: "Link or copy a local filesystem path into the tasktree",
		Long: `Creates a symlink (default) or a recursive copy of a local path inside
the tasktree. The source path is recorded in Tasktree.yml so that
"tasktree apply" can recreate the link or copy on other machines.

Example:
  tasktree add local /home/user/shared-scripts --name scripts
  tasktree add local ../sibling-repo --copy`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.addLocalService.Run(context.Background(), cwd, app.AddLocalOptions{
				SourcePath: args[0],
				Name:       name,
				Path:       path,
				Copy:       copy,
			})
			if err != nil {
				return formatError(err)
			}
			sourcePath := result.Source.Path
			if sourcePath == "" {
				sourcePath = result.Source.Name
			}
			verb := "Linked"
			if !result.Symlinked {
				verb = "Copied"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s at %s\n", verb, result.Source.Name, sourcePath)
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Destination name (defaults to the base name of <src-path>)")
	cmd.Flags().StringVar(&path, "path", "", "Destination path relative to the tasktree root (defaults to --name)")
	cmd.Flags().BoolVar(&copy, "copy", false, "Copy instead of symlinking")
	return cmd
}
