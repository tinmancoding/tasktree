package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddArchiveCmd(deps dependencies) *cobra.Command {
	var sha256, format, name, path string
	var stripComponents int

	cmd := &cobra.Command{
		Use:   "archive <url>",
		Short: "Download and extract a remote archive into the tasktree",
		Long: `Downloads a remote archive (tar.gz, tar.bz2, or zip) and extracts it
into the tasktree directory. Supports sha256 verification and stripping
leading path components (equivalent to tar --strip-components).

Example:
  tasktree add archive https://github.com/org/repo/archive/refs/tags/v1.0.tar.gz \
    --strip-components 1 --sha256 abc123...`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			result, err := deps.addArchiveService.Run(context.Background(), cwd, app.AddArchiveOptions{
				URL:             args[0],
				SHA256:          sha256,
				Format:          format,
				StripComponents: stripComponents,
				Name:            name,
				Path:            path,
			})
			if err != nil {
				return formatError(err)
			}
			sourcePath := result.Source.Path
			if sourcePath == "" {
				sourcePath = result.Source.Name
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Extracted %s at %s\n", result.Source.Name, sourcePath)
			return err
		},
	}
	cmd.Flags().StringVar(&sha256, "sha256", "", "Expected SHA-256 hex digest of the archive for verification")
	cmd.Flags().StringVar(&format, "format", "", "Archive format: tar.gz, tar.bz2, or zip (inferred from URL if omitted)")
	cmd.Flags().StringVar(&name, "name", "", "Destination directory name (defaults to the last URL path segment)")
	cmd.Flags().StringVar(&path, "path", "", "Destination path relative to the tasktree root (defaults to --name)")
	cmd.Flags().Var(newIntValue(0, &stripComponents), "strip-components",
		"Number of leading path components to strip on extraction (like tar --strip-components)")
	return cmd
}

// intValue is a pflag.Value that holds an int.
type intValue struct {
	val *int
}

func newIntValue(def int, p *int) *intValue {
	*p = def
	return &intValue{val: p}
}

func (v *intValue) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*v.val = n
	return nil
}

func (v *intValue) Type() string   { return "int" }
func (v *intValue) String() string { return strconv.Itoa(*v.val) }
