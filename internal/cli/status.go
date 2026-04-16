package cli

import (
	"context"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/output"
)

func newStatusCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show live status for repositories in the current tasktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.statusService.Run(context.Background(), cwd)
			if err != nil {
				return formatError(err)
			}

			// Build sorted annotations slice for deterministic output.
			annotationKeys := make([]string, 0, len(result.Annotations))
			for k := range result.Annotations {
				annotationKeys = append(annotationKeys, k)
			}
			sort.Strings(annotationKeys)
			annotations := make([]struct {
				Key   string
				Value string
			}, 0, len(annotationKeys))
			for _, k := range annotationKeys {
				annotations = append(annotations, struct {
					Key   string
					Value string
				}{Key: k, Value: result.Annotations[k]})
			}

			repos := make([]struct {
				Name  string
				Path  string
				Head  string
				State string
			}, 0, len(result.Repos))
			for _, repo := range result.Repos {
				repos = append(repos, struct {
					Name  string
					Path  string
					Head  string
					State string
				}{Name: repo.Name, Path: repo.Path, Head: repo.Head, State: repo.State})
			}
			return output.WriteStatusTable(cmd.OutOrStdout(), result.TasktreeName, result.Root, annotations, repos)
		},
	}
}
