package cli

import (
	"github.com/spf13/cobra"
)

// newAddCmd returns the "add" command group.
//
// Subcommands handle each source type explicitly:
//
//	tasktree add git   <url>       [--branch] [--from] [--name]
//	tasktree add http  <url>       [--sha256] [--header] [--name] [--path]
//	tasktree add archive <url>     [--sha256] [--format] [--strip-components] [--name] [--path]
//	tasktree add static <name>     --content <value> [--mode] [--path]
//	tasktree add local <src-path>  [--name] [--path] [--copy]
//
// For backward compatibility, running "tasktree add <url>" without a
// subcommand is treated as "tasktree add git <url>".
func newAddCmd(deps dependencies) *cobra.Command {
	// git flags live on the parent for backward-compat with "tasktree add <url>".
	var branch, from, name string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a source to the current tasktree",
		Long: `Add a source to the tasktree, materializing it on disk immediately.

Each source type has its own subcommand:

  tasktree add git <url>       [--branch] [--from] [--name]
  tasktree add http <url>      [--sha256] [--header] [--name] [--path]
  tasktree add archive <url>   [--sha256] [--format] [--strip-components] [--name] [--path]
  tasktree add static <name>   --content <value> [--mode] [--path]
  tasktree add local <src>     [--name] [--path] [--copy]

Running "tasktree add <url>" without a subcommand is equivalent to
"tasktree add git <url>" for backward compatibility.`,
		// Allow args so the backward-compat git fallback can receive <url>.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			// Backward-compat: treat as "add git <url>".
			return runAddGit(cmd, args, deps, branch, from, name)
		},
	}

	// git-specific flags on the parent for backward compat.
	cmd.Flags().StringVar(&branch, "branch", "", "Branch to use: reuse if local, track if remote, or create from --from")
	cmd.Flags().StringVar(&from, "from", "", "Base ref for branch creation, or direct checkout when --branch is omitted")
	cmd.Flags().StringVar(&name, "name", "", "Checkout directory name")

	cmd.AddCommand(newAddGitCmd(deps))
	cmd.AddCommand(newAddHTTPCmd(deps))
	cmd.AddCommand(newAddArchiveCmd(deps))
	cmd.AddCommand(newAddStaticCmd(deps))
	cmd.AddCommand(newAddLocalCmd(deps))

	return cmd
}
