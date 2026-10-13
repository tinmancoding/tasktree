package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinmancoding/tasktree/v2/internal/app"
)

func syncCmd(o *options) *cobra.Command {
	var x app.SyncOptions
	var j bool
	var ref string
	c := &cobra.Command{Use: "sync", Short: "Synchronize task records, definitions, and repository branches", Long: `Exchange state with the tasks remote. Without a remote, sync still reconciles the local tasks repository and reports local state.

Inside a tasktree (or with -t), sync does the following:
  1. Fetch the remote.
  2. Merge main. Task records are merged field by field: log entries are united in time order, labels and refs are merged as sets, and a field changed on both sides keeps the local value, gets a "comment" log entry with the remote value, and is flagged as a conflict (shown as "!" in list; clear it with "info ack").
  3. Fast-forward the task branch. If local and remote both moved, sync stops with exit 5; run "resolve".
  4. Apply recorded repository commits that came from the other machine to clean repositories on their branch.
  5. Update repositories by mode, up to sync.jobs at a time: follow and work repositories fast-forward to origin; dirty or diverged ones are skipped and reported. Gitlink moves this causes are committed.
  6. Push main and the task branch with a lease. A rejected push repeats the sync, up to 3 times.
  7. Report dirty, diverged, skipped and unpushed state.

Sync refuses to run when the root has uncommitted changes; run "commit" first. It never pushes code to a repository's origin; use "push".

Outside a tasktree, sync is the same as --all: it exchanges main and every task branch and does not touch any checkout. It reports checkouts that are behind and checkouts of closed tasks.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		if ref != "" {
			if x.All {
				return usageError{"--task and --all are mutually exclusive"}
			}
			id, root, e := requireTree(a, ref)
			if e != nil {
				return e
			}
			x.Root, x.ID = root, id
		} else if !x.All {
			root, cfg, e := app.DiscoverRoot("")
			if e == nil {
				x.Root = root
				x.ID = cfg.TaskID
			} else if errors.Is(e, os.ErrNotExist) {
				x.All = true
			} else {
				return e
			}
		}
		r, e := a.Sync(cmd.Context(), x)
		if j {
			if encErr := json.NewEncoder(o.out).Encode(r); encErr != nil {
				return encErr
			}
		} else {
			writeSyncReport(o, r)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&x.NoRepos, "no-repos", false, "skip step 5 (updating repositories from origin); recorded commits from the other machine are still applied")
	c.Flags().BoolVar(&x.All, "all", false, "exchange main and every task branch; do not touch checkouts")
	c.Flags().BoolVar(&x.AllowUnrelated, "allow-unrelated", false, "allow the first merge of main histories that share no commit")
	c.Flags().BoolVar(&j, "json", false, "emit the report as JSON")
	return c
}
func writeSyncReport(o *options, r app.SyncReport) {
	for _, s := range r.Steps {
		fmt.Fprintln(o.out, s)
	}
	fmt.Fprintln(o.out, "Report:")
	for _, section := range []struct {
		name  string
		items []string
	}{{"dirty", r.Dirty}, {"diverged", r.Diverged}, {"skipped", r.Skipped}, {"unpushed gitlinks", r.UnpushedGitlinks}, {"checkouts behind", r.CheckoutsBehind}, {"closed-task checkouts", r.ClosedTaskCheckouts}, {"failed", r.Failed}} {
		if len(section.items) == 0 {
			fmt.Fprintf(o.out, "  %s: none\n", section.name)
			continue
		}
		fmt.Fprintf(o.out, "  %s:\n", section.name)
		for _, s := range section.items {
			fmt.Fprintf(o.out, "    %s\n", s)
		}
	}
}
func resolveCmd(o *options) *cobra.Command {
	var take, ref string
	var j bool
	c := &cobra.Command{Use: "resolve [path...]", Short: "Inspect or explicitly merge divergent tasktree histories", Long: `Resolve a task branch that diverged between this machine and the remote.

Without --take, resolve lists the local and remote commits, the paths changed on both sides, and the paths that merge cleanly. It changes nothing when the histories have not diverged.

Paths changed on one side only are merged automatically. Paths changed differently on both sides need a decision: --take local or --take remote applies to all of them, or to the paths given as arguments (repositories count as paths). Resolve then creates a merge commit, updates the checkout and syncs. Repository branches are never reset; if one is ahead of or diverged from the chosen commit, it keeps its tip and the mismatch is reported.`, Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		var id, root string
		if ref != "" {
			if id, root, e = requireTree(a, ref); e != nil {
				return e
			}
		} else {
			var cfg app.TasktreeConfig
			root, cfg, e = app.DiscoverRoot("")
			if errors.Is(e, os.ErrNotExist) {
				return notInTasktreeError{}
			}
			if e != nil {
				return e
			}
			id = cfg.TaskID
		}
		r, e := a.Resolve(cmd.Context(), id, root, take, args)
		if j {
			if encErr := json.NewEncoder(o.out).Encode(r); encErr != nil {
				return encErr
			}
		} else if r.NothingToResolve {
			fmt.Fprintln(o.out, "nothing to resolve")
		} else {
			fmt.Fprintln(o.out, "Local commits:")
			for _, s := range r.LocalCommits {
				fmt.Fprintln(o.out, "  "+s)
			}
			fmt.Fprintln(o.out, "Remote commits:")
			for _, s := range r.RemoteCommits {
				fmt.Fprintln(o.out, "  "+s)
			}
			fmt.Fprintln(o.out, "Changed on both sides (files and gitlinks):")
			for _, s := range r.Paths {
				fmt.Fprintln(o.out, "  "+s)
			}
			fmt.Fprintln(o.out, "Auto-merged paths:")
			for _, s := range r.AutoMerged {
				fmt.Fprintln(o.out, "  "+s)
			}
			if r.Commit != "" {
				line := fmt.Sprintf("Merged %d local and %d remote commit(s)", len(r.LocalCommits), len(r.RemoteCommits))
				if len(r.AutoMerged) > 0 {
					line += "; auto-merged: " + strings.Join(r.AutoMerged, ", ")
				}
				if len(r.Paths) > 0 {
					line += "; took " + take + ": " + strings.Join(r.Paths, ", ")
				}
				fmt.Fprintln(o.out, line)
				fmt.Fprintln(o.out, "Resolved: "+r.Commit)
				writeSyncReport(o, r.Report)
			}
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVar(&take, "take", "", "keep the local or remote version of paths changed on both sides (all such paths unless paths are given)")
	c.Flags().BoolVar(&j, "json", false, "emit the comparison and result as JSON")
	return c
}
