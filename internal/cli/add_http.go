package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
)

func newAddHTTPCmd(deps dependencies) *cobra.Command {
	var sha256 string
	var headers []string // "Key: Value" or "Key=Value" pairs
	var name, path string

	cmd := &cobra.Command{
		Use:   "http <url>",
		Short: "Download a file from an HTTPS URL and add it to the tasktree",
		Long: `Downloads a single file from an HTTPS URL and places it at the declared path.

The download is idempotent: running "tasktree apply" later will re-download
the file only if it is missing from disk.

Example:
  tasktree add http https://example.com/config/base.json --sha256 e3b0c4...`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			hdrs, err := parseHeaders(headers)
			if err != nil {
				return formatError(err)
			}
			result, err := deps.addHTTPService.Run(context.Background(), cwd, app.AddHTTPOptions{
				URL:     args[0],
				SHA256:  sha256,
				Headers: hdrs,
				Name:    name,
				Path:    path,
			})
			if err != nil {
				return formatError(err)
			}
			sourcePath := result.Source.Path
			if sourcePath == "" {
				sourcePath = result.Source.Name
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %s at %s\n", result.Source.Name, sourcePath)
			return err
		},
	}
	cmd.Flags().StringVar(&sha256, "sha256", "", "Expected SHA-256 hex digest for verification")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "HTTP request header in 'Name: Value' or 'Name=Value' format (repeatable)")
	cmd.Flags().StringVar(&name, "name", "", "Destination name (defaults to the last URL path segment)")
	cmd.Flags().StringVar(&path, "path", "", "Destination path relative to the tasktree root (defaults to --name)")
	return cmd
}

// parseHeaders converts a slice of "Key: Value" or "Key=Value" strings into a
// map. Returns an error if any entry cannot be parsed.
func parseHeaders(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, h := range raw {
		if idx := strings.Index(h, ": "); idx >= 0 {
			out[h[:idx]] = h[idx+2:]
			continue
		}
		if idx := strings.IndexByte(h, '='); idx >= 0 {
			out[h[:idx]] = h[idx+1:]
			continue
		}
		return nil, fmt.Errorf("invalid header %q: must be in 'Name: Value' or 'Name=Value' format", h)
	}
	return out, nil
}
