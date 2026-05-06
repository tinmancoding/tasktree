package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddStaticCmd(deps dependencies) *cobra.Command {
	var content, mode, path string

	cmd := &cobra.Command{
		Use:   "static <name>",
		Short: "Write inline content to a file in the tasktree",
		Long: `Writes the given content to a file at the declared path inside the tasktree.
The content is stored inline in Tasktree.yml, making the file reproducible
across machines via "tasktree apply".

Example:
  tasktree add static docker-compose.override.yml \
    --content 'services:\n  api:\n    environment:\n      DEBUG: "true"' \
    --mode 0644`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.addStaticService.Run(context.Background(), cwd, app.AddStaticOptions{
				Name:    args[0],
				Path:    path,
				Content: content,
				Mode:    mode,
			})
			if err != nil {
				return formatError(err)
			}
			sourcePath := result.Source.Path
			if sourcePath == "" {
				sourcePath = result.Source.Name
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s\n", result.Source.Name, sourcePath)
			return err
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "File content to write (required)")
	cmd.Flags().StringVar(&mode, "mode", "", "Unix file permission mode as octal string (default 0644)")
	cmd.Flags().StringVar(&path, "path", "", "Destination path relative to the tasktree root (defaults to <name>)")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}
