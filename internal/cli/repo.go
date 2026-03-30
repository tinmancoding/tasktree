package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/output"
)

func newRepoCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repository aliases",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newRepoAddAliasCmd(deps), newRepoRemoveAliasCmd(deps), newRepoAliasesCmd(deps))
	return cmd
}

func newRepoAddAliasCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "add-alias <alias> <clone-url>",
		Short: "Add a repository alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.aliasSet.Run(args[0], args[1]); err != nil {
				return formatError(err)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Added alias %s -> %s\n", args[0], args[1])
			return err
		},
	}
}

func newRepoRemoveAliasCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "remove-alias <alias>",
		Short: "Remove a repository alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.aliasRemove.Run(args[0]); err != nil {
				return formatError(err)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed alias %s\n", args[0])
			return err
		},
	}
}

func newRepoAliasesCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "aliases",
		Short: "List repository aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases, err := deps.aliasList.Run()
			if err != nil {
				return formatError(err)
			}
			rows := make([]struct {
				Alias string
				URL   string
			}, 0, len(aliases))
			for _, alias := range aliases {
				rows = append(rows, struct {
					Alias string
					URL   string
				}{Alias: alias.Alias, URL: alias.URL})
			}
			return output.WriteRepoAliasTable(cmd.OutOrStdout(), rows)
		},
	}
}
