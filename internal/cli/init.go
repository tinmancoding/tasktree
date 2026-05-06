package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/variable"
)

func newInitCmd(deps dependencies) *cobra.Command {
	var annotateFlags []string
	var fromTemplate string
	var name string
	var dir string
	var apply bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "init [path] [key=value...]",
		Short: "Initialize a tasktree",
		// Args validation is done manually to support both the legacy positional
		// path and the new key=value pairs for template mode.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromTemplate != "" {
				return runInitFromTemplate(cmd, deps, args, fromTemplate, name, dir, apply, dryRun)
			}
			return runInit(cmd, deps, args, annotateFlags)
		},
	}

	cmd.Flags().StringArrayVar(&annotateFlags, "annotate", nil,
		"Set an annotation at init time as key=value (repeatable)")
	cmd.Flags().StringVar(&fromTemplate, "from", "",
		"Template name or file path to initialize from")
	cmd.Flags().StringVar(&name, "name", "",
		"Override workspace directory name (only with --from)")
	cmd.Flags().StringVar(&dir, "dir", "",
		"Target directory for the workspace (only with --from)")
	cmd.Flags().BoolVar(&apply, "apply", false,
		"Run apply after creating the workspace (only with --from)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Show what would be created without writing files (only with --from)")

	return cmd
}

// runInit handles the classic `tasktree init [path]` flow.
func runInit(cmd *cobra.Command, deps dependencies, args []string, annotateFlags []string) error {
	target := "."
	if len(args) == 1 {
		target = args[0]
	} else if len(args) > 1 {
		return fmt.Errorf("too many arguments: use --from to pass key=value template variables")
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
}

// runInitFromTemplate handles `tasktree init --from <template> [key=value...]`.
func runInitFromTemplate(
	cmd *cobra.Command,
	deps dependencies,
	args []string,
	fromTemplate, name, dir string,
	apply, dryRun bool,
) error {
	// Parse key=value positional arguments as template variables.
	cliVars, err := variable.ParseKVArgs(args)
	if err != nil {
		return formatError(err)
	}

	opts := app.InitFromTemplateOptions{
		Template: fromTemplate,
		Vars:     cliVars,
		Name:     name,
		Dir:      dir,
		DryRun:   dryRun,
	}

	result, err := deps.initService.RunFromTemplate(opts)
	if err != nil {
		return formatError(err)
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would create tasktree at %s\n", result.Root)
		fmt.Fprintf(cmd.OutOrStdout(), "  name: %s\n", result.Spec.Metadata.Name)
		if len(result.Spec.Spec.Sources) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  sources:\n")
			for _, src := range result.Spec.Spec.Sources {
				if src.Git != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "    - %s (git): %s", src.Name, src.Git.URL)
					if src.Git.Branch != "" {
						fmt.Fprintf(cmd.OutOrStdout(), " @ %s", src.Git.Branch)
					}
					fmt.Fprintln(cmd.OutOrStdout())
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "    - %s (%s)\n", src.Name, src.Type)
				}
			}
		}
		return nil
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Initialized tasktree at %s\n", result.Root)
	if err != nil {
		return err
	}

	if apply {
		// Delegate to the apply service using the new workspace root.
		applyResult, err := deps.applyService.Run(cmd.Context(), result.Root, app.ApplyOptions{})
		if err != nil {
			return formatError(err)
		}
		_ = applyResult
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Applied workspace at %s\n", result.Root)
		return err
	}

	return nil
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
