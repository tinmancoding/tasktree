package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newInitCmd(deps dependencies) *cobra.Command {
	var annotateFlags []string

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a tasktree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			annotations, err := parseAnnotateFlags(annotateFlags)
			if err != nil {
				return formatError(err)
			}

			root, err := deps.initService.Run(target, app.InitOptions{Annotations: annotations})
			if err != nil {
				return formatError(err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Initialized tasktree at %s\n", root)
			return err
		},
	}

	cmd.Flags().StringArrayVar(&annotateFlags, "annotate", nil,
		"Set an annotation at init time as key=value (repeatable)")

	return cmd
}

// parseAnnotateFlags converts a slice of "key=value" strings into a map.
// It returns an error if any entry does not contain "=".
func parseAnnotateFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.IndexByte(f, '=')
		if idx < 0 {
			return nil, fmt.Errorf("--annotate value %q must be in key=value format", f)
		}
		result[f[:idx]] = f[idx+1:]
	}
	return result, nil
}
