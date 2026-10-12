package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinmancoding/tasktree/v2/internal/app"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/output"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type notInTasktreeError struct{}

func (notInTasktreeError) Error() string { return "not in a tasktree; pass -t" }

type usageError struct{ message string }

func (e usageError) Error() string { return e.message }
func Code(err error) int {
	if err == nil {
		return 0
	}
	var ni notInTasktreeError
	if errors.As(err, &ni) {
		return 4
	}
	var nf hub.NotFoundError
	if errors.As(err, &nf) {
		return 3
	}
	var conflict hub.ConflictError
	if errors.As(err, &conflict) {
		return 5
	}
	var gate app.GateError
	if errors.As(err, &gate) {
		return 6
	}
	var ue usageError
	if errors.As(err, &ue) {
		return 2
	}
	s := err.Error()
	// Cobra/pflag does not expose a stable public error type for flag parsing.
	// Match only its concrete flag diagnostics; application errors containing
	// words such as "requires" must retain exit 1.
	if strings.HasPrefix(s, "unknown flag:") || strings.HasPrefix(s, "unknown shorthand flag:") || strings.HasPrefix(s, "flag needs an argument:") || strings.HasPrefix(s, "invalid argument ") {
		return 2
	}
	return 1
}

type options struct {
	chdir    string
	verbose  bool
	author   string
	out, err io.Writer
}

func NewRoot(out, errOut io.Writer) *cobra.Command {
	o := &options{out: out, err: errOut}
	root := &cobra.Command{Use: "tt2", Short: "Task management backed by git", SilenceUsage: true, SilenceErrors: true, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if o.chdir != "" {
			if err := os.Chdir(o.chdir); err != nil {
				return fmt.Errorf("change directory: %w", err)
			}
		}
		return nil
	}}
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().StringVarP(&o.chdir, "chdir", "C", "", "run as if started in this directory")
	root.PersistentFlags().BoolVarP(&o.verbose, "verbose", "v", false, "print git commands")
	root.PersistentFlags().StringVar(&o.author, "author", "", "author for log entries")
	root.AddCommand(setupCmd(o), configCmd(o), newCmd(o), listCmd(o), infoCmd(o), noteCmd(o), markCmd(o), openCmd(o), addCmd(o), removeCmd(o), commitCmd(o), rootCmd(o))
	return root
}
func (o *options) application() (app.App, error) {
	h, err := home.Resolve()
	if err != nil {
		return app.App{}, err
	}
	var v io.Writer
	if o.verbose {
		v = o.err
	}
	a := app.New(h, v)
	a.AuthorOverride = o.author
	a.WarningWriter = o.err
	return a, nil
}
func exact(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usageError{fmt.Sprintf("%s accepts %d arg(s), received %d", cmd.CommandPath(), n, len(args))}
		}
		return nil
	}
}
func setupCmd(o *options) *cobra.Command {
	var x app.SetupOptions
	c := &cobra.Command{Use: "setup", Short: "Initialize ~/.tasktree and machine config", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, err := o.application()
		if err != nil {
			return err
		}
		x.TasksRootSet = cmd.Flags().Changed("tasks-root")
		x.AuthorSet = cmd.Flags().Changed("author")
		x.MachineSet = cmd.Flags().Changed("machine")
		x.RemoteSet = cmd.Flags().Changed("remote")
		migrated, err := a.MigrateV1Aliases()
		if err != nil {
			return err
		}
		cfg, err := a.Setup(cmd.Context(), x)
		if err != nil {
			return err
		}
		fmt.Fprintln(o.out, "tasktree setup complete")
		if migrated {
			fmt.Fprintln(o.out, "copied V1 repository aliases to aliases.yml")
		}
		if cfg.Remote == "" {
			fmt.Fprintln(o.err, "warning: ~/.tasktree is the only copy; back it up")
		}
		return nil
	}}
	c.Flags().StringVar(&x.TasksRoot, "tasks-root", "", "task checkout root")
	c.Flags().StringVar(&x.Author, "author", "", "default author")
	c.Flags().StringVar(&x.Machine, "machine", "", "machine name")
	c.Flags().StringVar(&x.Remote, "remote", "", "hub remote URL")
	return c
}
func configCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Read or change machine config"}
	root.AddCommand(&cobra.Command{Use: "get <key>", Short: "Print a config value", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		v, e := a.ConfigGet(args[0])
		if e == nil {
			fmt.Fprintln(o.out, v)
		}
		return e
	}}, &cobra.Command{Use: "set <key> <value>", Short: "Set a config value", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		return a.ConfigSet(args[0], args[1])
	}})
	return root
}
func newCmd(o *options) *cobra.Command {
	var x app.NewOptions
	var open bool
	var path string
	c := &cobra.Command{Use: "new <title>", Short: "Create a task (optionally a subtask, optionally open its tasktree)", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		x.Title = args[0]
		a, e := o.application()
		if e != nil {
			return e
		}
		t, e := a.NewTask(cmd.Context(), x)
		if e != nil {
			return e
		}
		if !open {
			fmt.Fprintf(o.out, "%s %s\n", t.Handle, t.ID)
			return nil
		}
		fmt.Fprintf(o.err, "%s %s\n", t.Handle, t.ID)
		p, e := a.Open(cmd.Context(), t.ID, path, false, o.err)
		if e == nil {
			fmt.Fprintln(o.out, p)
		}
		return e
	}}
	c.Flags().StringVar(&x.Jira, "jira", "", "Jira key")
	c.Flags().StringVar(&x.Handle, "handle", "", "task handle")
	c.Flags().StringVar(&x.Status, "status", "incoming", "initial status")
	c.Flags().StringVar(&x.Parent, "parent", "", "parent task")
	c.Flags().BoolVar(&open, "open", false, "create and open a tasktree")
	c.Flags().StringVar(&path, "path", "", "checkout path (with --open)")
	return c
}
func listCmd(o *options) *cobra.Command {
	var all, j bool
	var statuses []string
	c := &cobra.Command{Use: "list", Short: "List tasks (open tasks by default)", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		tasks, e := a.List(all, statuses)
		if e != nil {
			return e
		}
		if j {
			return json.NewEncoder(o.out).Encode(tasks)
		}
		parent := map[string]string{}
		allTasks, _ := a.List(true, nil)
		for _, t := range allTasks {
			parent[t.ID] = t.Handle
		}
		rows := make([]output.TaskRow, 0, len(tasks))
		for _, t := range tasks {
			p := ""
			if t.Parent != nil {
				p = parent[*t.Parent]
			}
			rows = append(rows, output.TaskRow{Handle: t.Handle, Status: t.Status, Title: t.Title, ID: t.ID, Parent: p})
		}
		return output.WriteTaskTable(o.out, rows)
	}}
	c.Flags().BoolVar(&all, "all", false, "include closed tasks")
	c.Flags().StringSliceVar(&statuses, "status", nil, "filter by status (repeatable)")
	c.Flags().BoolVar(&j, "json", false, "emit JSON")
	return c
}
func resolveTarget(a app.App, explicit string) (string, string, error) {
	id, root, err := a.ResolveTarget(explicit, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", notInTasktreeError{}
		}
		return "", "", err
	}
	return id, root, nil
}
func infoCmd(o *options) *cobra.Command {
	var ref string
	var j bool
	c := &cobra.Command{Use: "info", Short: "Show a task record and, inside a tasktree, its repositories", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, root, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		t, e := a.Info(resolved)
		if e != nil {
			return e
		}
		var tree *app.TasktreeInfo
		if root != "" {
			x, inspectErr := a.InspectTasktree(cmd.Context(), t.ID, root)
			if inspectErr != nil {
				return inspectErr
			}
			tree = &x
		}
		if j {
			b, marshalErr := json.Marshal(t)
			if marshalErr != nil {
				return marshalErr
			}
			var payload map[string]any
			if unmarshalErr := json.Unmarshal(b, &payload); unmarshalErr != nil {
				return unmarshalErr
			}
			if tree != nil {
				payload["tasktree"] = tree
			}
			return json.NewEncoder(o.out).Encode(payload)
		}
		parent := "-"
		if t.Parent != nil {
			parent = *t.Parent
			if tasks, listErr := a.List(true, nil); listErr == nil {
				for _, candidate := range tasks {
					if candidate.ID == *t.Parent {
						parent = candidate.Handle
						break
					}
				}
			}
		}
		writeInfo(o.out, t, parent)
		if tree != nil {
			return writeRepoInfo(o.out, *tree)
		}
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&j, "json", false, "emit JSON")
	return c
}
func noteCmd(o *options) *cobra.Command {
	var ref, kind string
	c := &cobra.Command{Use: "note <text>", Short: "Append a note, decision, comment, or ref to a task log", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		if e := record.ValidateEntryBody(args[0]); e != nil {
			return usageError{e.Error()}
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		t, e := a.Note(cmd.Context(), resolved, args[0], kind)
		if e == nil {
			fmt.Fprintf(o.out, "note added to %s\n", t.Handle)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVar(&kind, "kind", "note", "entry kind")
	return c
}
func markCmd(o *options) *cobra.Command {
	var ref string
	var markOptions app.MarkOptions
	c := &cobra.Command{Use: "mark <status>", Short: "Set task status; closing tears down the local tasktree", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		if e := record.ValidateStatus(args[0]); e != nil {
			return usageError{e.Error()}
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		cwd, _ := os.Getwd()
		result, e := a.Mark(cmd.Context(), resolved, args[0], markOptions)
		if markOptions.Force {
			for _, loss := range result.Loss {
				fmt.Fprintln(o.err, loss)
			}
		}
		if result.RemovedAt != "" && pathWithin(cwd, result.RemovedAt) {
			fmt.Fprintln(o.err, "tasktree removed; run: cd ..")
		}
		if e == nil {
			fmt.Fprintf(o.out, "%s marked %s\n", result.Task.Handle, result.Task.Status)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&markOptions.Keep, "keep", false, "change status without tearing down the tasktree")
	c.Flags().BoolVar(&markOptions.Force, "force", false, "close despite work that will be lost")
	return c
}

func pathWithin(path, root string) bool {
	path, pathErr := filepath.Abs(path)
	root, rootErr := filepath.Abs(root)
	if pathErr != nil || rootErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeInfo(w io.Writer, t record.Task, parent string) {
	fmt.Fprintf(w, "ID: %s\nHandle: %s\nTitle: %s\nStatus: %s\n", t.ID, t.Handle, t.Title, t.Status)
	fmt.Fprintf(w, "Parent: %s\n", parent)
	fmt.Fprintf(w, "Labels: %s\nCreated: %s\nUpdated: %s\n", strings.Join(t.Labels, ", "), t.CreatedAt.Format("2006-01-02T15:04:05Z"), t.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintln(w, "Refs:")
	for _, r := range t.Refs {
		fmt.Fprintf(w, "  %s: %s\n", r.Type, r.Value)
	}
	fmt.Fprintf(w, "Description:\n%s\nLog:\n", t.Description)
	for _, e := range t.Log {
		fmt.Fprintf(w, "  %s %s %s@%s: %s\n", e.Timestamp.Format("2006-01-02T15:04:05Z"), e.Kind, e.Author, e.Machine, e.Body)
	}
}
func writeRepoInfo(w io.Writer, tree app.TasktreeInfo) error {
	if _, err := fmt.Fprintf(w, "Tasktree: %s\nRoot changes: %d\n", tree.Path, tree.RootDirty); err != nil {
		return err
	}
	rows := make([]output.RepoRow, 0, len(tree.Repos))
	for _, r := range tree.Repos {
		state := "clean"
		if r.Dirty {
			state = "modified"
		}
		rows = append(rows, output.RepoRow{Name: r.Name, Mode: r.Mode, Branch: r.Branch, Head: r.Head, State: state, Ahead: r.Ahead, Behind: r.Behind, Unpushed: r.Unpushed})
	}
	return output.WriteRepoTable(w, rows)
}

func openCmd(o *options) *cobra.Command {
	var path string
	var force bool
	c := &cobra.Command{Use: "open <handle|id>", Short: "Materialize or locate a task's tasktree; prints its path", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		p, e := a.Open(cmd.Context(), args[0], path, force, o.err)
		if e == nil {
			fmt.Fprintln(o.out, p)
		}
		return e
	}}
	c.Flags().StringVar(&path, "path", "", "checkout path")
	c.Flags().BoolVar(&force, "force", false, "replace an orphaned hidden git directory after reporting loss")
	return c
}
func requireTree(a app.App, ref string) (string, string, error) {
	id, root, e := resolveTarget(a, ref)
	if e != nil {
		return "", "", e
	}
	if root == "" {
		return "", "", notInTasktreeError{}
	}
	return id, root, nil
}
func addCmd(o *options) *cobra.Command {
	var ref string
	var x app.AddOptions
	c := &cobra.Command{Use: "add <alias|url>", Short: "Add a repository to the tasktree", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		x.URL = args[0]
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		r, e := a.Add(cmd.Context(), id, root, x)
		if e == nil {
			for _, m := range r.Messages {
				fmt.Fprintln(o.err, m)
			}
			fmt.Fprintf(o.out, "added %s (%s)\n", r.Name, r.Mode)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVar(&x.Branch, "branch", "", "branch")
	c.Flags().StringVar(&x.From, "from", "", "base ref")
	c.Flags().StringVar(&x.Name, "name", "", "repository name")
	c.Flags().StringVar(&x.Mode, "mode", "", "follow, work, or pin")
	return c
}
func removeCmd(o *options) *cobra.Command {
	var ref string
	var force bool
	c := &cobra.Command{Use: "remove <name>", Short: "Remove a repository from the tasktree", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		loss, e := a.Remove(cmd.Context(), id, root, args[0], force)
		if e == nil {
			for _, m := range loss {
				fmt.Fprintln(o.err, m)
			}
			fmt.Fprintf(o.out, "removed %s\n", args[0])
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&force, "force", false, "remove despite lost work")
	return c
}
func commitCmd(o *options) *cobra.Command {
	var ref, msg string
	c := &cobra.Command{Use: "commit", Short: "Record root files and repository commits on the task branch", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		committed, warnings, e := a.Commit(cmd.Context(), id, root, msg)
		for _, w := range warnings {
			fmt.Fprintln(o.err, "warning:", w)
		}
		if e == nil {
			if committed {
				fmt.Fprintln(o.out, "tasktree committed and pushed")
			} else {
				fmt.Fprintln(o.out, "nothing to commit")
			}
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	return c
}
func rootCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "root", Short: "Print the tasktree root path", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		_, root, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		if root == "" {
			return notInTasktreeError{}
		}
		fmt.Fprintln(o.out, root)
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}

func Execute(ctx context.Context, args []string, out, errOut io.Writer) error {
	r := NewRoot(out, errOut)
	r.SetArgs(args)
	return r.ExecuteContext(ctx)
}
