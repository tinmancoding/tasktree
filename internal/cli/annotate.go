package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/output"
)

func newAnnotateCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "annotate",
		Short: "Manage annotations for the current tasktree",
	}
	cmd.AddCommand(
		newAnnotateSetCmd(deps),
		newAnnotateUnsetCmd(deps),
		newAnnotateListCmd(deps),
	)
	return cmd
}

func newAnnotateSetCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set or update an annotation",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := deps.annotateService.Set(cwd, args[0], args[1]); err != nil {
				return formatError(err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Annotation %q set.\n", args[0])
			return err
		},
	}
}

func newAnnotateUnsetCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove an annotation (no-op if the key does not exist)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := deps.annotateService.Unset(cwd, args[0]); err != nil {
				return formatError(err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Annotation %q removed.\n", args[0])
			return err
		},
	}
}

func newAnnotateListCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all annotations for the current tasktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			entries, err := deps.annotateService.List(cwd)
			if err != nil {
				return formatError(err)
			}
			rows := make([]output.AnnotationRow, len(entries))
			for i, e := range entries {
				rows[i] = output.AnnotationRow{Key: e.Key, Value: e.Value}
			}
			return output.WriteAnnotationsTable(cmd.OutOrStdout(), rows)
		},
	}
}
