package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddCmd(deps dependencies) *cobra.Command {
	var branch string
	var from string
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
				Branch:  branch,
				From:    from,
				Name:    name,
			})
			if err != nil {
				return formatError(err)
			}

			// Print branch resolution path message.
			switch result.BranchPath {
			case app.BranchPathLocalExisting:
				msg := fmt.Sprintf("Using existing local branch %q", result.EffectiveBranch)
				if result.IgnoredFrom != "" {
					msg += fmt.Sprintf("; ignoring --from %q", result.IgnoredFrom)
				}
				msg += "."
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), msg); err != nil {
					return err
				}
			case app.BranchPathRemoteTracking:
				msg := fmt.Sprintf("Using existing remote branch %q from origin", result.EffectiveBranch)
				if result.IgnoredFrom != "" {
					msg += fmt.Sprintf("; ignoring --from %q", result.IgnoredFrom)
				}
				msg += "."
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), msg); err != nil {
					return err
				}
			case app.BranchPathCreated:
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Creating new branch %q from %q.\n", result.EffectiveBranch, result.EffectiveFrom); err != nil {
					return err
				}
			case app.BranchPathHeadless:
				// Use the Ref field if set (explicit checkout request), else the source name.
				ref := result.Source.Git.Ref
				if ref == "" {
					ref = result.Source.Name
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Checking out %q without creating a branch.\n", ref); err != nil {
					return err
				}
			}

			// Resolve display path: use Path if set, else Name.
			sourcePath := result.Source.Path
			if sourcePath == "" {
				sourcePath = result.Source.Name
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Added %s at %s\n", result.Source.Name, sourcePath); err != nil {
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
	cmd.Flags().StringVar(&branch, "branch", "", "Branch to use: reuse if local, track if remote, or create from --from")
	cmd.Flags().StringVar(&from, "from", "", "Base ref for branch creation, or direct checkout when --branch is omitted")
	cmd.Flags().StringVar(&name, "name", "", "Checkout directory name")
	return cmd
}
