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
			repoURL, err := deps.aliasResolve.Run(args[0])
			if err != nil {
				return formatError(err)
			}
			result, err := deps.addService.Run(context.Background(), cwd, app.AddOptions{
				RepoURL: repoURL,
				Ref:     ref,
				Branch:  branch,
				Name:    name,
			})
			if err != nil {
				return formatError(err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Added %s at %s\n", result.Repo.Name, result.Repo.Path); err != nil {
				return err
			}
			registrations, err := deps.aliasRegister.Run(repoURL)
			if err != nil {
				return formatError(err)
			}
			for _, registration := range registrations {
				switch registration.Status {
				case "added":
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Registered alias %s -> %s\n", registration.Alias, repoURL); err != nil {
						return err
					}
				case "existing":
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Alias %s already points to %s\n", registration.Alias, repoURL); err != nil {
						return err
					}
				case "conflict":
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Skipped alias %s; already used by %s\n", registration.Alias, registration.URL); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Branch, tag, commit, or ref to check out")
	cmd.Flags().StringVar(&branch, "branch", "", "Create a new local branch from the resolved starting point")
	cmd.Flags().StringVar(&name, "name", "", "Checkout directory name")
	return cmd
}
