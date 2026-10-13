package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// GenerateCLIDocs writes one Markdown page per command into dir. The output is
// deterministic so a test can compare it with the committed pages.
func GenerateCLIDocs(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root := NewRoot(nil, nil)
	root.DisableAutoGenTag = true
	prepender := func(string) string { return "" }
	// Pages link to each other by file name, which MkDocs resolves.
	link := func(name string) string { return name }
	return doc.GenMarkdownTreeCustom(root, dir, prepender, link)
}

func genDocsCmd(o *options) *cobra.Command {
	return &cobra.Command{
		Use:    "gen-docs <dir>",
		Short:  "Write the CLI reference as Markdown pages",
		Hidden: true,
		Args:   exact(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			if err := GenerateCLIDocs(dir); err != nil {
				return err
			}
			fmt.Fprintln(o.out, "wrote CLI reference to", args[0])
			return nil
		},
	}
}
