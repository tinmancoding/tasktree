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
	c := &cobra.Command{Use: "sync", Short: "Synchronize task records, definitions, and repository branches", Long: "Synchronize the configured hub remote and committed tasktree state. Outside a tasktree this is sync --all (checkouts are not touched). STATUS! in list marks unacknowledged record conflicts. Log entries interleaving in time are inserted in timestamp/id order, preserving local entry bytes.", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().BoolVar(&x.NoRepos, "no-repos", false, "skip repository branch updates")
	c.Flags().BoolVar(&x.All, "all", false, "sync all task refs without touching checkouts")
	c.Flags().BoolVar(&x.AllowUnrelated, "allow-unrelated", false, "authorize joining unrelated hub histories")
	c.Flags().BoolVar(&j, "json", false, "emit the sync report as JSON")
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
	c := &cobra.Command{Use: "resolve [path...]", Short: "Inspect or explicitly merge divergent tasktree histories", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&take, "take", "", "take local or remote for paths changed on both sides (all such paths by default)")
	c.Flags().BoolVar(&j, "json", false, "emit comparison and result as JSON")
	return c
}
