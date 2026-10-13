package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tinmancoding/tasktree/v2/internal/product"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinmancoding/tasktree/v2/internal/app"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/output"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"github.com/tinmancoding/tasktree/v2/internal/template"
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
	var validation app.ValidationError
	if errors.As(err, &ue) || errors.As(err, &validation) {
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
	root := &cobra.Command{Use: product.Name, Short: "Task management backed by git", SilenceUsage: true, SilenceErrors: true, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
	root.AddCommand(setupCmd(o), configCmd(o), newCmd(o), listCmd(o), infoCmd(o), noteCmd(o), markCmd(o), refCmd(o), labelCmd(o), historyCmd(o), openCmd(o), addCmd(o), removeCmd(o), modeCmd(o), bootstrapCmd(o), focusCmd(o), pushCmd(o), commitCmd(o), importCmd(o), moveCmd(o), doctorCmd(o), templateCmd(o), rootCmd(o), syncCmd(o), resolveCmd(o))
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
		return a.ConfigSetContext(cmd.Context(), args[0], args[1])
	}})
	return root
}
func newCmd(o *options) *cobra.Command {
	var x app.NewOptions
	var open, isolated bool
	var path, templateName string
	c := &cobra.Command{Use: "new [title] [k=v ...]", Short: "Create a task (optionally from a template or open its tasktree)", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		varArgs := []string{}
		if templateName != "" {
			if len(args) > 0 && !strings.Contains(args[0], "=") {
				x.Title = args[0]
				varArgs = args[1:]
			} else {
				varArgs = args
			}
		} else {
			if len(args) != 1 {
				return usageError{product.Name + " new requires one title unless --template is used"}
			}
			x.Title = args[0]
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		if templateName != "" {
			t, e := a.NewFromTemplate(cmd.Context(), app.TemplateApplyOptions{Name: templateName, Title: x.Title, Parent: x.Parent, Vars: varArgs})
			if e != nil {
				return e
			}
			if !open {
				fmt.Fprintf(o.out, "%s %s\n", t.Handle, t.ID)
				return nil
			}
			p, e := a.Open(cmd.Context(), t.ID, app.OpenOptions{Path: path}, o.err)
			if e == nil {
				fmt.Fprintln(o.out, p)
			}
			return e
		}
		if isolated && x.Parent == "" {
			return usageError{"--isolated requires --parent"}
		}
		if open && isolated {
			return usageError{"--open and --isolated are mutually exclusive"}
		}
		if isolated {
			handle := x.Handle
			if handle == "" {
				if x.Jira != "" {
					handle = x.Jira
				} else {
					handle = app.Slug(x.Title)
				}
			}
			if e := a.PreflightIsolated(cmd.Context(), x.Parent, handle, app.OpenOptions{Path: path}); e != nil {
				return e
			}
		}
		t, e := a.NewTask(cmd.Context(), x)
		if e != nil {
			return e
		}
		if !open && !isolated {
			fmt.Fprintf(o.out, "%s %s\n", t.Handle, t.ID)
			return nil
		}
		fmt.Fprintf(o.err, "%s %s\n", t.Handle, t.ID)
		p, e := a.Open(cmd.Context(), t.ID, app.OpenOptions{Path: path}, o.err)
		if e != nil && strings.Contains(e.Error(), "does not exist in parent") {
			return usageError{e.Error()}
		}
		if e == nil {
			fmt.Fprintln(o.out, p)
		}
		return e
	}}
	c.Flags().StringVar(&x.Jira, "jira", "", "Jira key")
	c.Flags().StringVar(&x.Handle, "handle", "", "task handle")
	c.Flags().StringVar(&x.Status, "status", "incoming", "initial status")
	c.Flags().StringVar(&x.Parent, "parent", "", "parent task")
	c.Flags().StringVar(&templateName, "template", "", "create from a template")
	c.Flags().BoolVar(&open, "open", false, "create and open a tasktree")
	c.Flags().BoolVar(&isolated, "isolated", false, "create an isolated subtask tasktree")
	c.Flags().StringVar(&path, "path", "", "checkout path")
	return c
}
func listCmd(o *options) *cobra.Command {
	var all, j bool
	var statuses, labels []string
	c := &cobra.Command{Use: "list", Short: "List tasks (open tasks by default)", Long: "List tasks and local definition/checkout state. A ! suffix on STATUS marks unacknowledged sync conflicts; info shows their fields. Acknowledge a field with info set, or description with info edit.", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		tasks, e := a.ListFiltered(all, statuses, labels)
		if e != nil {
			return e
		}
		if j {
			payload := make([]map[string]any, 0, len(tasks))
			local, _ := a.Home.LoadLocal()
			for _, t := range tasks {
				b, _ := json.Marshal(t)
				var row map[string]any
				_ = json.Unmarshal(b, &row)
				state := "none"
				if a.DefinitionExists(cmd.Context(), t.ID) {
					state = "defined"
				}
				if c, ok := local.Checkouts[t.ID]; ok {
					if st, x := os.Stat(c.Path); x == nil && st.IsDir() {
						state = "here"
					}
				}
				row["tasktree"] = state
				payload = append(payload, row)
			}
			return json.NewEncoder(o.out).Encode(payload)
		}
		allTasks, e := a.List(true, nil)
		if e != nil {
			return e
		}
		byID := map[string]record.Task{}
		children := map[string][]record.Task{}
		for _, t := range allTasks {
			byID[t.ID] = t
			if t.Parent != nil {
				children[*t.Parent] = append(children[*t.Parent], t)
			}
		}
		visible := map[string]bool{}
		for _, t := range tasks {
			visible[t.ID] = true
		}
		rows := []output.TaskRow{}
		local, _ := a.Home.LoadLocal()
		var visit func(record.Task, int)
		visit = func(t record.Task, depth int) {
			if !visible[t.ID] {
				return
			}
			parent := ""
			if t.Parent != nil {
				parent = byID[*t.Parent].Handle
			}
			progress := ""
			if t.Progress != nil {
				progress = fmt.Sprintf("%d/%d", t.Progress.Closed, t.Progress.Total)
			}
			treeState := "-"
			if a.DefinitionExists(cmd.Context(), t.ID) {
				treeState = "defined"
			}
			if c, ok := local.Checkouts[t.ID]; ok {
				if st, x := os.Stat(c.Path); x == nil && st.IsDir() {
					treeState = "here"
				}
			}
			status := t.Status
			if len(t.Conflict) > 0 {
				status += "!"
			}
			rows = append(rows, output.TaskRow{Handle: strings.Repeat("  ", depth) + t.Handle, Status: status, Title: t.Title, ID: t.ID, Parent: parent, Progress: progress, Tree: treeState})
			for _, child := range children[t.ID] {
				visit(child, depth+1)
			}
		}
		for _, t := range tasks {
			if t.Parent == nil || !visible[*t.Parent] {
				visit(t, 0)
			}
		}
		return output.WriteTaskTable(o.out, rows)
	}}
	c.Flags().BoolVar(&all, "all", false, "include closed tasks")
	c.Flags().StringSliceVar(&statuses, "status", nil, "filter by status (repeatable)")
	c.Flags().StringSliceVar(&labels, "label", nil, "filter by label (repeatable, AND)")
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
		var definition *app.DefinitionInfo
		if root != "" {
			_, rootCfg, discoverErr := app.DiscoverRoot(root)
			if discoverErr != nil {
				return discoverErr
			}
			if _, statErr := os.Stat(a.Home.ClonePath(rootCfg.TaskID)); statErr == nil {
				x, inspectErr := a.InspectTasktree(cmd.Context(), rootCfg.TaskID, root)
				if inspectErr != nil {
					return inspectErr
				}
				tree = &x
			}
		} else if d, defErr := a.Definition(cmd.Context(), t.ID); defErr != nil {
			return defErr
		} else if d.State == "defined" {
			definition = &d
		}
		allTasks, listErr := a.List(true, nil)
		if listErr != nil {
			return listErr
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
			cfg, configErr := a.Home.LoadConfig()
			if configErr != nil {
				return configErr
			}
			refs := make([]map[string]any, 0, len(t.Refs))
			for _, r := range t.Refs {
				x := map[string]any{"type": r.Type, "value": r.Value}
				if u := app.RefURL(r, cfg.Jira.BaseURL); u != "" {
					x["url"] = u
				}
				refs = append(refs, x)
			}
			payload["refs"] = refs
			subtasks := make([]map[string]any, 0)
			for _, candidate := range allTasks {
				if candidate.Parent != nil && *candidate.Parent == t.ID {
					subtasks = append(subtasks, map[string]any{"id": candidate.ID, "handle": candidate.Handle, "status": candidate.Status})
				}
			}
			payload["subtasks"] = subtasks
			if root != "" {
				payload["focus"] = nil
				if ft, ok := a.FocusForRoot(root); ok {
					payload["focus"] = ft.Handle
				}
			}
			if tree != nil {
				payload["tasktree"] = map[string]any{"state": "here", "path": tree.Path, "rootDirty": tree.RootDirty, "repos": tree.Repos}
			} else if definition != nil {
				payload["tasktree"] = definition
			} else {
				payload["tasktree"] = map[string]any{"state": "none", "repos": []any{}}
			}
			return json.NewEncoder(o.out).Encode(payload)
		}
		parent := "-"
		if t.Parent != nil {
			parent = *t.Parent
			for _, candidate := range allTasks {
				if candidate.ID == *t.Parent {
					parent = candidate.Handle
					break
				}
			}
		}
		children := []record.Task{}
		for _, candidate := range allTasks {
			if candidate.Parent != nil && *candidate.Parent == t.ID {
				children = append(children, candidate)
			}
		}
		cfg, configErr := a.Home.LoadConfig()
		if configErr != nil {
			return configErr
		}
		focus := ""
		if root != "" {
			if ft, ok := a.FocusForRoot(root); ok {
				focus = ft.Handle
			}
		}
		writeInfo(o.out, t, parent, children, cfg.Jira.BaseURL, focus)
		if tree != nil {
			return writeRepoInfo(o.out, *tree)
		}
		if definition != nil {
			fmt.Fprintln(o.out, "Tasktree: defined (not checked out here)")
			tw := newTable(o.out)
			fmt.Fprintln(tw, "NAME\tMODE\tBRANCH\tGITLINK")
			for _, r := range definition.Repos {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Name, r.Mode, r.Branch, r.Gitlink)
			}
			return tw.Flush()
		}
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&j, "json", false, "emit JSON")
	c.AddCommand(infoEditCmd(o), infoSetCmd(o))
	return c
}

func infoEditCmd(o *options) *cobra.Command {
	var ref, file string
	var stdin bool
	c := &cobra.Command{Use: "edit", Short: "Replace a task description", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		if file != "" && stdin {
			return usageError{"--file and --stdin are mutually exclusive"}
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		current, e := a.Info(resolved)
		if e != nil {
			return e
		}
		var text string
		switch {
		case file != "":
			b, readErr := os.ReadFile(file)
			if readErr != nil {
				return readErr
			}
			text = string(b)
		case stdin:
			b, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				return readErr
			}
			text = string(b)
		default:
			f, createErr := os.CreateTemp("", "tt2-description-*")
			if createErr != nil {
				return createErr
			}
			name := f.Name()
			defer os.Remove(name)
			if _, writeErr := f.WriteString(current.Description); writeErr != nil {
				f.Close()
				return writeErr
			}
			if closeErr := f.Close(); closeErr != nil {
				return closeErr
			}
			editor := os.Getenv("VISUAL")
			if editor == "" {
				editor = os.Getenv("EDITOR")
			}
			if editor == "" {
				editor = "vi"
			}
			command := exec.Command("sh", "-c", "exec \"$1\" \"$2\"", product.Name+" editor", editor, name)
			command.Stdin, command.Stdout, command.Stderr = os.Stdin, o.out, o.err
			if runErr := command.Run(); runErr != nil {
				return fmt.Errorf("run editor: %w", runErr)
			}
			b, readErr := os.ReadFile(name)
			if readErr != nil {
				return readErr
			}
			text = strings.TrimRight(string(b), "\r\n")
		}
		if e := record.ValidateDescription(text); e != nil {
			return usageError{e.Error()}
		}
		t, changed, e := a.EditDescription(cmd.Context(), resolved, text)
		if e != nil {
			return e
		}
		if changed {
			fmt.Fprintf(o.out, "description updated for %s\n", t.Handle)
		} else {
			fmt.Fprintln(o.out, "description unchanged; no commit made")
		}
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVar(&file, "file", "", "read description from file")
	c.Flags().BoolVar(&stdin, "stdin", false, "read description from standard input")
	return c
}
func infoSetCmd(o *options) *cobra.Command {
	var ref string
	var clear bool
	c := &cobra.Command{Use: "set <field> [value]", Short: "Set a task title, handle, status, or parent (acknowledging conflicts)", Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return usageError{"info set accepts a field and optional value"}
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		if clear && args[0] != "parent" {
			return usageError{"--clear is only valid for parent"}
		}
		if clear && len(args) != 1 {
			return usageError{"--clear does not accept a value"}
		}
		if !clear && len(args) != 2 {
			return usageError{"info set requires a value"}
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		value := ""
		if len(args) == 2 {
			value = args[1]
		}
		t, changed, e := a.SetInfo(cmd.Context(), resolved, args[0], value, clear)
		if e != nil {
			return e
		}
		if changed {
			fmt.Fprintf(o.out, "%s updated\n", t.Handle)
		} else {
			fmt.Fprintln(o.out, "unchanged; no commit made")
		}
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&clear, "clear", false, "clear the parent")
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
	c.Flags().BoolVar(&markOptions.Cascade, "cascade", false, "close all open descendants first")
	return c
}

func refCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "ref", Short: "Manage typed task references"}
	for _, action := range []string{"add", "rm"} {
		action := action
		root.AddCommand(&cobra.Command{Use: action + " <type> <value>", Short: map[string]string{"add": "Add a task reference", "rm": "Remove a task reference"}[action], Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "jira" && args[0] != "pr" && args[0] != "url" {
				return usageError{fmt.Sprintf("invalid ref type %q", args[0])}
			}
			ref, _ := cmd.Flags().GetString("task")
			a, e := o.application()
			if e != nil {
				return e
			}
			resolved, _, e := resolveTarget(a, ref)
			if e != nil {
				return e
			}
			t, changed, e := a.Ref(cmd.Context(), resolved, action, args[0], args[1])
			if e != nil {
				return e
			}
			if changed {
				fmt.Fprintf(o.out, "ref %s for %s\n", map[string]string{"add": "added", "rm": "removed"}[action], t.Handle)
			} else {
				fmt.Fprintln(o.out, "ref unchanged; no commit made")
			}
			return nil
		}})
	}
	for _, c := range root.Commands() {
		c.Flags().StringP("task", "t", "", "task handle or id")
	}
	return root
}
func labelCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "label", Short: "Manage task labels"}
	for _, action := range []string{"add", "rm"} {
		action := action
		root.AddCommand(&cobra.Command{Use: action + " <label>...", Short: map[string]string{"add": "Add task labels", "rm": "Remove task labels"}[action], Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError{"label " + action + " requires at least one label"}
			}
			return nil
		}, RunE: func(cmd *cobra.Command, args []string) error {
			ref, _ := cmd.Flags().GetString("task")
			a, e := o.application()
			if e != nil {
				return e
			}
			resolved, _, e := resolveTarget(a, ref)
			if e != nil {
				return e
			}
			t, changed, e := a.Labels(cmd.Context(), resolved, action, args)
			if e != nil {
				return e
			}
			if changed {
				fmt.Fprintf(o.out, "labels %s for %s\n", map[string]string{"add": "updated", "rm": "updated"}[action], t.Handle)
			} else {
				fmt.Fprintln(o.out, "labels unchanged; no commit made")
			}
			return nil
		}})
	}
	for _, c := range root.Commands() {
		c.Flags().StringP("task", "t", "", "task handle or id")
	}
	return root
}
func historyCmd(o *options) *cobra.Command {
	var ref string
	var asJSON bool
	c := &cobra.Command{Use: "history", Short: "Show a task record's git history", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		entries, e := a.History(cmd.Context(), resolved)
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(o.out).Encode(entries)
		}
		tw := newTable(o.out)
		for _, entry := range entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", entry.Timestamp.Format(time.RFC3339), entry.Author, entry.Subject)
		}
		return tw.Flush()
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
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

func writeInfo(w io.Writer, t record.Task, parent string, children []record.Task, jiraBase, focus string) {
	fmt.Fprintf(w, "ID: %s\nHandle: %s\nTitle: %s\nStatus: %s\n", t.ID, t.Handle, t.Title, t.Status)
	if len(t.Conflict) > 0 {
		fmt.Fprintf(w, "Conflict: %s\n", strings.Join(t.Conflict, ", "))
	}
	if focus != "" {
		fmt.Fprintf(w, "Focus: %s\n", focus)
	}
	fmt.Fprintf(w, "Parent: %s\n", parent)
	if t.Progress != nil {
		fmt.Fprintf(w, "Progress: %d/%d\n", t.Progress.Closed, t.Progress.Total)
	}
	fmt.Fprintf(w, "Labels: %s\nCreated: %s\nUpdated: %s\n", strings.Join(t.Labels, ", "), t.CreatedAt.Format("2006-01-02T15:04:05Z"), t.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintln(w, "Refs:")
	for _, r := range t.Refs {
		if u := app.RefURL(r, jiraBase); u != "" && r.Type != "url" {
			fmt.Fprintf(w, "  %s: %s (%s)\n", r.Type, r.Value, u)
		} else {
			fmt.Fprintf(w, "  %s: %s\n", r.Type, r.Value)
		}
	}
	if len(children) > 0 {
		fmt.Fprintln(w, "Subtasks:")
		for _, child := range children {
			fmt.Fprintf(w, "  %s: %s\n", child.Handle, child.Status)
		}
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
	var x app.OpenOptions
	var repos []string
	c := &cobra.Command{Use: "open <handle|id>", Short: "Materialize or locate a task's tasktree; prints its path", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		x.Repos = repos
		p, e := a.Open(cmd.Context(), args[0], x, o.err)
		if e != nil && strings.Contains(e.Error(), "does not exist in parent") {
			return usageError{e.Error()}
		}
		if e == nil {
			fmt.Fprintln(o.out, p)
		}
		return e
	}}
	c.Flags().StringVar(&x.Path, "path", "", "checkout path")
	c.Flags().BoolVar(&x.Force, "force", false, "replace an orphaned hidden git directory after reporting loss")
	c.Flags().BoolVar(&x.SkipBootstrap, "skip-bootstrap", false, "do not run bootstrap steps")
	c.Flags().StringSliceVar(&repos, "repos", nil, "parent repositories to include when promoting a subtask")
	c.Flags().StringVar(&x.Branch, "branch", "", "work repository branch override")
	return c
}
func requireTree(a app.App, ref string) (string, string, error) {
	id, root, e := a.ResolveTreeTarget(ref, "")
	if e != nil {
		if errors.Is(e, os.ErrNotExist) {
			return "", "", notInTasktreeError{}
		}
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
		id, root, e := a.ResolveTreeTarget(ref, "")
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				return notInTasktreeError{}
			}
			return e
		}
		if root == "" && ref == "" {
			return notInTasktreeError{}
		}
		var r app.AddResult
		if root != "" {
			r, e = a.Add(cmd.Context(), id, root, x)
		} else {
			r, e = a.AddDefined(cmd.Context(), id, x)
		}
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
		id, root, e := a.ResolveTreeTarget(ref, "")
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				return notInTasktreeError{}
			}
			return e
		}
		if root == "" && ref == "" {
			return notInTasktreeError{}
		}
		var loss []string
		if root != "" {
			loss, e = a.Remove(cmd.Context(), id, root, args[0], force)
		} else {
			e = a.RemoveDefined(cmd.Context(), id, args[0])
		}
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
	var allowRewind bool
	c := &cobra.Command{Use: "commit", Short: "Record root files and repository commits on the task branch", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		committed, warnings, e := a.CommitWithRewind(cmd.Context(), id, root, msg, allowRewind)
		for _, w := range warnings {
			fmt.Fprintln(o.err, "warning:", w)
		}
		if e == nil {
			if committed {
				sha, e := a.RootShortHead(cmd.Context(), id, root)
				if e != nil {
					return e
				}
				msg := "committed " + sha + " and pushed to the local hub"
				if cfg, e := a.Home.LoadConfig(); e == nil && cfg.Remote != "" {
					msg += "; run " + product.Name + " sync to publish"
				}
				fmt.Fprintln(o.out, msg)
			} else {
				fmt.Fprintln(o.out, "nothing to commit")
			}
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	c.Flags().BoolVar(&allowRewind, "allow-rewind", false, "allow recording a repository commit older than the recorded gitlink")
	return c
}

type doctorFindingsError struct{}

func (doctorFindingsError) Error() string { return "doctor found errors" }

func doctorCmd(o *options) *cobra.Command {
	var fix, jsonOut bool
	c := &cobra.Command{Use: "doctor", Short: "Check tasktree state and safely repair known problems", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		r, e := a.Doctor(cmd.Context(), fix)
		if e != nil {
			return e
		}
		if jsonOut {
			enc := json.NewEncoder(o.out)
			enc.SetEscapeHTML(false)
			if e = enc.Encode(r); e != nil {
				return e
			}
		} else {
			tw := newTable(o.out)
			fmt.Fprintln(tw, "SEVERITY\tCODE\tSUBJECT\tMESSAGE")
			fixable := 0
			for _, f := range r.Findings {
				state := ""
				if f.Fixed {
					state = " (fixed)"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s%s\n", strings.ToUpper(f.Severity), f.Code, f.Subject, f.Message, state)
				if f.Fixable && !f.Fixed {
					fixable++
				}
			}
			if e = tw.Flush(); e != nil {
				return e
			}
			fmt.Fprintf(o.out, "%d errors, %d warnings, %d fixable\n", r.Errors, r.Warnings, fixable)
		}
		if r.Errors > 0 {
			return doctorFindingsError{}
		}
		return nil
	}}
	c.Flags().BoolVar(&fix, "fix", false, "apply safe repairs and check again")
	c.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return c
}

func moveCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "move <new-path>", Short: "Relocate a tasktree checkout atomically", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		p, e := a.Move(cmd.Context(), id, root, args[0])
		if e != nil {
			return e
		}
		fmt.Fprintln(o.out, p)
		fmt.Fprintln(o.err, "hint: open shells keep the old $PWD; cd to the new path")
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}

func templateCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "template", Short: "List, show, or validate task templates"}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List available templates", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		all, e := a.TemplateStore().List()
		if e != nil {
			return e
		}
		tw := newTable(o.out)
		fmt.Fprintln(tw, "NAME\tSOURCE\tDESCRIPTION")
		for _, s := range all {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Metadata.Name, s.Source, s.Metadata.Description)
		}
		return tw.Flush()
	}},
		&cobra.Command{Use: "show <name>", Short: "Show template parameters and raw YAML", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
			a, e := o.application()
			if e != nil {
				return e
			}
			s, e := a.TemplateStore().Load(args[0])
			if e != nil {
				return usageError{e.Error()}
			}
			tw := newTable(o.out)
			fmt.Fprintln(tw, "PARAMETER\tREQUIRED\tDEFAULT\tDESCRIPTION")
			for _, p := range s.Parameters {
				fmt.Fprintf(tw, "%s\t%t\t%s\t%s\n", p.Name, p.Required, p.Default, p.Description)
			}
			if e = tw.Flush(); e != nil {
				return e
			}
			fmt.Fprintln(o.out, "---")
			_, e = o.out.Write(s.Raw)
			return e
		}},
		&cobra.Command{Use: "validate <name|file>", Short: "Validate a template", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
			a, e := o.application()
			if e != nil {
				return e
			}
			s, e := a.TemplateStore().Load(args[0])
			if e != nil {
				return usageError{e.Error()}
			}
			if e = template.Validate(s); e != nil {
				return usageError{e.Error()}
			}
			fmt.Fprintf(o.out, "template %s is valid\n", s.Metadata.Name)
			return nil
		}})
	return root
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

func modeCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "mode <repo> <follow|work|pin>", Short: "Change a repository mode", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
		if args[1] != "follow" && args[1] != "work" && args[1] != "pin" {
			return usageError{"mode must be follow, work, or pin"}
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := a.ResolveTreeTarget(ref, "")
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				return notInTasktreeError{}
			}
			return e
		}
		if root == "" && ref == "" {
			return notInTasktreeError{}
		}
		if root != "" {
			e = a.Mode(cmd.Context(), id, root, args[0], args[1])
		} else {
			e = a.ModeDefined(cmd.Context(), id, args[0], args[1])
		}
		if e != nil && strings.Contains(e.Error(), "detached") {
			return usageError{e.Error()}
		}
		if e == nil {
			fmt.Fprintf(o.out, "%s mode set to %s\n", args[0], args[1])
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}
func bootstrapCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "bootstrap", Short: "Run the tasktree bootstrap steps", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		return a.Bootstrap(cmd.Context(), id, root, o.err)
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}
func focusCmd(o *options) *cobra.Command {
	var clear bool
	c := &cobra.Command{Use: "focus [handle|id]", Short: "Focus a shared descendant in this tasktree", Args: func(cmd *cobra.Command, args []string) error {
		if clear && len(args) == 0 {
			return nil
		}
		if !clear && len(args) == 1 {
			return nil
		}
		return usageError{"focus requires one task, or --clear"}
	}, RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		root, _, e := app.DiscoverRoot("")
		if e != nil {
			return notInTasktreeError{}
		}
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		}
		t, e := a.Focus(root, ref, clear)
		if e == nil {
			if clear {
				fmt.Fprintln(o.out, "focus cleared")
			} else {
				fmt.Fprintf(o.out, "focused %s\n", t.Handle)
			}
		}
		return e
	}}
	c.Flags().BoolVar(&clear, "clear", false, "clear the checkout focus")
	return c
}
func pushCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "push [repo...]", Short: "Push repository branches and print PR hints", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		r, e := a.Push(cmd.Context(), id, root, args)
		if e != nil && strings.Contains(e.Error(), "detached") {
			return usageError{e.Error()}
		}
		for _, x := range r {
			fmt.Fprintf(o.err, "%s: %s\n", x.Name, x.Message)
			if x.Hint != "" {
				fmt.Fprintln(o.out, x.Hint)
			}
		}
		if e == nil && len(r) == 0 && len(args) == 0 {
			fmt.Fprintln(o.out, "no work repos to push (name follow/pin repos explicitly)")
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}

func importCmd(o *options) *cobra.Command {
	var x app.ImportOptions
	var all, recoverImport bool
	c := &cobra.Command{Use: "import <v1-path>", Short: "Import V1 workspaces without recloning repositories", Args: func(cmd *cobra.Command, args []string) error {
		if recoverImport && (all || len(args) != 1) {
			return usageError{product.Name + " import --recover accepts exactly one path and cannot be combined with --all"}
		}
		if all && len(args) != 0 {
			return usageError{product.Name + " import --all accepts no path"}
		}
		if !all && len(args) != 1 {
			return usageError{product.Name + " import accepts one path or --all"}
		}
		if cmd.Flags().Changed("handle") {
			if e := app.ValidateHandle(x.Handle); e != nil {
				return usageError{e.Error()}
			}
		}
		if all && (cmd.Flags().Changed("handle") || cmd.Flags().Changed("status")) {
			return usageError{"--handle and --status cannot be used with --all"}
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		a, err := o.application()
		if err != nil {
			return err
		}
		if recoverImport {
			message, e := a.RecoverImport(cmd.Context(), args[0])
			if e == nil {
				fmt.Fprintln(o.out, message)
			}
			return e
		}
		paths := args
		if all {
			registry := os.Getenv("TASKTREE_V1_REGISTRY")
			if registry == "" {
				u, e := os.UserHomeDir()
				if e != nil {
					return e
				}
				registry = filepath.Join(u, ".local", "state", "tasktree", "registry.toml")
			}
			paths, err = app.ReadV1Registry(registry)
			if err != nil {
				return err
			}
		}
		imported, refused, failed, skipped := 0, 0, 0, 0
		for _, path := range paths {
			if all {
				info, statErr := os.Stat(path)
				if os.IsNotExist(statErr) {
					skipped++
					fmt.Fprintf(o.out, "%s: skipped: path missing\n", path)
					continue
				}
				if statErr != nil {
					failed++
					fmt.Fprintf(o.err, "%s: failed: %v\n", path, statErr)
					fmt.Fprintf(o.out, "summary: imported=%d refused=%d failed=%d skipped=%d\n", imported, refused, failed, skipped)
					return fmt.Errorf("import stopped after failure: %w", statErr)
				}
				if !info.IsDir() {
					skipped++
					fmt.Fprintf(o.out, "%s: skipped: not a V1 workspace\n", path)
					continue
				}
				if _, legacyErr := os.Stat(filepath.Join(path, "Tasktree.yml")); os.IsNotExist(legacyErr) {
					if _, markerErr := os.Stat(filepath.Join(path, ".tasktree", "config.yml")); os.IsNotExist(markerErr) {
						skipped++
						fmt.Fprintf(o.out, "%s: skipped: not a V1 workspace\n", path)
						continue
					}
				}
			}
			result, e := a.Import(cmd.Context(), path, x)
			app.PrintImportPlan(o.out, result.Plan)
			if e != nil {
				var gate app.GateError
				if errors.As(e, &gate) {
					refused++
					fmt.Fprintf(o.err, "%s: refused: %v\n", path, e)
				} else {
					failed++
					fmt.Fprintf(o.err, "%s: failed: %v\n", path, e)
				}
				if !all {
					return e
				}
				// Refusals are safe to skip; an operational failure may have left a
				// journal and must stop the batch immediately.
				if !errors.As(e, &gate) {
					fmt.Fprintf(o.out, "summary: imported=%d refused=%d failed=%d skipped=%d\n", imported, refused, failed, skipped)
					return fmt.Errorf("import stopped after failure: %w", e)
				}
				continue
			}
			if result.Skipped {
				skipped++
				fmt.Fprintln(o.out, "already imported")
			} else if x.DryRun {
				fmt.Fprintln(o.out, "dry-run: no changes")
			} else {
				imported++
				fmt.Fprintf(o.out, "imported %s as %s (%s)\n", path, result.Task.Handle, result.Task.ID)
				if x.Move {
					fmt.Fprintln(o.out, "workspace moved; existing shells keep their old $PWD — cd to", result.Path)
				}
				fmt.Fprintln(o.out, "V1 registry unchanged; run `tasktree prune` to drop its entry")
			}
		}
		if all {
			fmt.Fprintf(o.out, "summary: imported=%d refused=%d failed=%d skipped=%d\n", imported, refused, failed, skipped)
			if refused > 0 {
				return app.GateError{Message: "one or more workspaces were refused"}
			}
			if failed > 0 {
				return fmt.Errorf("one or more workspaces failed")
			}
		}
		return nil
	}}
	c.Flags().BoolVar(&all, "all", false, "import all entries from the V1 registry")
	c.Flags().BoolVar(&recoverImport, "recover", false, "recover an interrupted import from its journal")
	c.Flags().StringVar(&x.Handle, "handle", "", "V2 task handle")
	c.Flags().StringVar(&x.Status, "status", "", "V2 task status (default active)")
	c.Flags().BoolVar(&x.Move, "move", false, "atomically move under tasksRoot after import")
	c.Flags().BoolVar(&x.DryRun, "dry-run", false, "print the plan without changing anything")
	c.Flags().BoolVar(&x.AllowLarge, "allow-large", false, "allow committing root files larger than 50 MB")
	return c
}

func Execute(ctx context.Context, args []string, out, errOut io.Writer) error {
	r := NewRoot(out, errOut)
	r.SetArgs(args)
	return r.ExecuteContext(ctx)
}

// newTable returns an aligned column writer for human-readable tables.
func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}
