package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	var ue usageError
	if errors.As(err, &ue) {
		return 2
	}
	s := err.Error()
	if strings.Contains(s, "unknown flag") || strings.Contains(s, "requires") || strings.Contains(s, "accepts ") {
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
	root.AddCommand(setupCmd(o), configCmd(o), newCmd(o), listCmd(o), infoCmd(o), noteCmd(o), markCmd(o))
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
	c := &cobra.Command{Use: "setup", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, err := o.application()
		if err != nil {
			return err
		}
		x.TasksRootSet = cmd.Flags().Changed("tasks-root")
		x.AuthorSet = cmd.Flags().Changed("author")
		x.MachineSet = cmd.Flags().Changed("machine")
		x.RemoteSet = cmd.Flags().Changed("remote")
		cfg, err := a.Setup(cmd.Context(), x)
		if err != nil {
			return err
		}
		fmt.Fprintln(o.out, "tasktree setup complete")
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
	root := &cobra.Command{Use: "config"}
	root.AddCommand(&cobra.Command{Use: "get <key>", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		v, e := a.ConfigGet(args[0])
		if e == nil {
			fmt.Fprintln(o.out, v)
		}
		return e
	}}, &cobra.Command{Use: "set <key> <value>", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
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
	c := &cobra.Command{Use: "new <title>", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		x.Title = args[0]
		a, e := o.application()
		if e != nil {
			return e
		}
		t, e := a.NewTask(cmd.Context(), x)
		if e == nil {
			fmt.Fprintf(o.out, "%s %s\n", t.Handle, t.ID)
		}
		return e
	}}
	c.Flags().StringVar(&x.Jira, "jira", "", "Jira key")
	c.Flags().StringVar(&x.Handle, "handle", "", "task handle")
	c.Flags().StringVar(&x.Status, "status", "incoming", "initial status")
	c.Flags().StringVar(&x.Parent, "parent", "", "parent task")
	return c
}
func listCmd(o *options) *cobra.Command {
	var all, j bool
	var statuses []string
	c := &cobra.Command{Use: "list", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
func target(c *cobra.Command, v string) error {
	if v == "" {
		return notInTasktreeError{}
	}
	return nil
}
func infoCmd(o *options) *cobra.Command {
	var ref string
	var j bool
	c := &cobra.Command{Use: "info", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		if e := target(cmd, ref); e != nil {
			return e
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		t, e := a.Info(ref)
		if e != nil {
			return e
		}
		if j {
			return json.NewEncoder(o.out).Encode(t)
		}
		writeInfo(o.out, t)
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&j, "json", false, "emit JSON")
	return c
}
func noteCmd(o *options) *cobra.Command {
	var ref, kind string
	c := &cobra.Command{Use: "note <text>", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		if e := target(cmd, ref); e != nil {
			return e
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		t, e := a.Note(cmd.Context(), ref, args[0], kind)
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
	c := &cobra.Command{Use: "mark <status>", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		if e := target(cmd, ref); e != nil {
			return e
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		t, e := a.Mark(cmd.Context(), ref, args[0])
		if e == nil {
			fmt.Fprintf(o.out, "%s marked %s\n", t.Handle, t.Status)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}
func writeInfo(w io.Writer, t record.Task) {
	fmt.Fprintf(w, "ID: %s\nHandle: %s\nTitle: %s\nStatus: %s\n", t.ID, t.Handle, t.Title, t.Status)
	if t.Parent != nil {
		fmt.Fprintf(w, "Parent: %s\n", *t.Parent)
	}
	fmt.Fprintln(w, "Refs:")
	for _, r := range t.Refs {
		fmt.Fprintf(w, "  %s: %s\n", r.Type, r.Value)
	}
	fmt.Fprintf(w, "Description:\n%s\nLog:\n", t.Description)
	for _, e := range t.Log {
		fmt.Fprintf(w, "  %s %s %s@%s: %s\n", e.Timestamp.Format("2006-01-02T15:04:05Z"), e.Kind, e.Author, e.Machine, e.Body)
	}
}
func Execute(ctx context.Context, args []string, out, errOut io.Writer) error {
	r := NewRoot(out, errOut)
	r.SetArgs(args)
	return r.ExecuteContext(ctx)
}
