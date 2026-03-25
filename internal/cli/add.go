package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddCmd(deps dependencies) *cobra.Command {
	var ref string
	var branch string
	var name string

	cmd := &cobra.Command{
		Use:   "add <repo-url>",
		Short: "Add a repository to the current tasktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.addService.Run(context.Background(), cwd, app.AddOptions{
				RepoURL: args[0],
				Ref:     ref,
				Branch:  branch,
				Name:    name,
			})
			if err != nil {
				return formatError(err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s at %s\n", result.Repo.Name, result.Repo.Path)
			return err
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Branch, tag, commit, or ref to check out")
	cmd.Flags().StringVar(&branch, "branch", "", "Create a new local branch from the resolved starting point")
	cmd.Flags().StringVar(&name, "name", "", "Checkout directory name")
	return cmd
}
