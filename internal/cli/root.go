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
	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
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
	var nc app.NoCheckoutError
	if errors.As(err, &nc) {
		return 4
	}
	var nf hub.NotFoundError
	var appNF app.NotFoundError
	var tplNF template.NotFoundError
	if errors.As(err, &nf) || errors.As(err, &appNF) || errors.As(err, &tplNF) {
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
	if strings.HasPrefix(s, "unknown command ") || strings.HasPrefix(s, "unknown flag:") || strings.HasPrefix(s, "unknown shorthand flag:") || strings.HasPrefix(s, "flag needs an argument:") || strings.HasPrefix(s, "invalid argument ") {
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
	root := &cobra.Command{Use: product.Name, Short: "Task management backed by git", Long: `Manage tasks and the multi-repository workspaces for them. Git is the only state store.

A task is a Markdown record with a status and a log. A tasktree is the definition of the repositories, files and symlinks that belong to one task. A checkout is a tasktree materialized in a directory on this machine.

Most commands act on the task of the tasktree you are in. Use -t <handle|id> to name a task from anywhere. Inside a tasktree, record commands also honor TASKTREE_TASK and the focused subtask.

Exit codes: 0 success; 1 other error; 2 usage or validation error; 3 not found; 4 not in a tasktree, or the task has no checkout; 5 conflict; 6 refused by a safety gate.`, SilenceUsage: true, SilenceErrors: true, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
	root.PersistentFlags().StringVar(&o.author, "author", "", "author for log entries (overrides TASKTREE_AUTHOR and the author config key)")
	root.AddCommand(setupCmd(o), configCmd(o), newCmd(o), listCmd(o), infoCmd(o), noteCmd(o), markCmd(o), refCmd(o), labelCmd(o), historyCmd(o), openCmd(o), addCmd(o), removeCmd(o), modeCmd(o), bootstrapCmd(o), focusCmd(o), pushCmd(o), commitCmd(o), importCmd(o), moveCmd(o), doctorCmd(o), templateCmd(o), aliasCmd(o), rootCmd(o), syncCmd(o), resolveCmd(o), genDocsCmd(o))
	for _, c := range root.Commands() {
		requireKnownSubcommand(c)
	}
	return root
}

// requireKnownSubcommand makes a command group reject an unknown subcommand
// with a usage error (exit 2) instead of silently printing help.
func requireKnownSubcommand(c *cobra.Command) {
	if !c.HasSubCommands() || c.Runnable() {
		return
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usageError{fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())}
		}
		return cmd.Help()
	}
	for _, sub := range c.Commands() {
		requireKnownSubcommand(sub)
	}
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
	c := &cobra.Command{Use: "setup", Short: "Initialize ~/.tasktree and machine config", Long: `Create the tasks repository in ~/.tasktree (or $TASKTREE_HOME) and write config.yml. Safe to repeat: only the flags you pass change the config.

With --remote on a machine that has no tasks repository, setup clones it from the remote. With --remote on an existing repository it behaves like "config set remote".

When aliases.yml does not exist yet, setup copies V1 repository aliases into it if it finds them.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&x.TasksRoot, "tasks-root", "", "directory for new checkouts (default ~/Developer/tasks)")
	c.Flags().StringVar(&x.Author, "author", "", "author name for log entries")
	c.Flags().StringVar(&x.Machine, "machine", "", "machine name for log entries (default: hostname)")
	c.Flags().StringVar(&x.Remote, "remote", "", "URL of the tasks remote")
	return c
}
func configCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Read or change machine config", Long: `Read or change ~/.tasktree/config.yml.

Keys: remote, tasksRoot, author, machine, jira.baseUrl, sync.jobs, sync.timeout.

Setting remote adds or replaces origin on the tasks repository and pushes main and every task branch. Setting it to "" removes origin.`}
	root.AddCommand(&cobra.Command{Use: "get <key>", Short: "Print a config value", Long: "Print one config value. Keys: remote, tasksRoot, author, machine, jira.baseUrl, sync.jobs, sync.timeout. Unset values print an empty line.", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		v, e := a.ConfigGet(args[0])
		if e == nil {
			fmt.Fprintln(o.out, v)
		}
		return e
	}}, &cobra.Command{Use: "set <key> <value>", Short: "Set a config value", Long: "Set one config key. sync.jobs must be a positive integer. sync.timeout must be a positive duration such as 60s. See \"config\" for the keys.", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
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
	var path, templateName, branch string
	var repos []string
	c := &cobra.Command{Use: "new [title] [k=v ...]", Short: "Create a task (optionally from a template or open its tasktree)", Long: `Create a task record and print "<handle> <id>". The handle is --handle, else the --jira key, else a slug of the title.

A task starts without a tasktree. Add repositories with "add -t <handle>", or create the task from a template. With --open, the tasktree is created and checked out, and its path is printed.

With --template, the title is optional and k=v arguments set template parameters. --jira, --handle, --status and the title override the template values.

With --parent, the new task is a subtask of the parent. It shares the parent's tasktree unless you pass --isolated, which creates a tasktree of its own with the parent's work repositories.`, Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
		if templateName != "" && isolated {
			return usageError{"--isolated cannot be used with --template"}
		}
		if !isolated && (len(repos) > 0 || branch != "") {
			return usageError{"--repos and --branch require --isolated"}
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		if templateName != "" {
			tpl := app.TemplateApplyOptions{Name: templateName, Title: x.Title, Parent: x.Parent, Vars: varArgs, Jira: x.Jira, Handle: x.Handle}
			if cmd.Flags().Changed("status") {
				tpl.Status = x.Status
			}
			t, e := a.NewFromTemplate(cmd.Context(), tpl)
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
			if e := a.PreflightIsolated(cmd.Context(), x.Parent, handle, app.OpenOptions{Path: path, Repos: repos, Branch: branch}); e != nil {
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
		p, e := a.Open(cmd.Context(), t.ID, app.OpenOptions{Path: path, Repos: repos, Branch: branch}, o.err)
		if e != nil && strings.Contains(e.Error(), "does not exist in parent") {
			return usageError{e.Error()}
		}
		if e == nil {
			fmt.Fprintln(o.out, p)
		}
		return e
	}}
	c.Flags().StringVar(&x.Jira, "jira", "", "Jira key; becomes the handle and a jira ref")
	c.Flags().StringVar(&x.Handle, "handle", "", "task handle (default: Jira key, else slug of the title)")
	c.Flags().StringVar(&x.Status, "status", "incoming", "initial status: incoming, next, active, blocked, done or dropped")
	c.Flags().StringVar(&x.Parent, "parent", "", "make this a subtask of a task (handle or id)")
	c.Flags().StringVar(&templateName, "template", "", "create from a template name or file")
	c.Flags().BoolVar(&open, "open", false, "also create and check out a tasktree; prints its path")
	c.Flags().BoolVar(&isolated, "isolated", false, "give the subtask its own tasktree (requires --parent)")
	c.Flags().StringVar(&path, "path", "", "checkout path (default <tasksRoot>/<handle>)")
	c.Flags().StringSliceVar(&repos, "repos", nil, "parent repositories to copy into an isolated subtask (default: the parent's work repositories)")
	c.Flags().StringVar(&branch, "branch", "", "branch for the subtask's work repositories (default <parent-branch>-<subtask-handle>)")
	return c
}
func listCmd(o *options) *cobra.Command {
	var all, j bool
	var statuses, labels, refs []string
	c := &cobra.Command{Use: "list", Short: "List tasks (open tasks by default)", Long: `List tasks, oldest first. Without --all or --status, only open tasks (incoming, next, active, blocked) are listed. Subtasks are indented under their parent when the parent is listed too.

The TREE column is "here" when the task has a checkout on this machine, "defined" when it has a tasktree branch but no checkout here, and "-" otherwise. A "!" after the status marks unacknowledged sync conflicts; "info" shows the fields. Acknowledge one with "info ack <field>"; "info set" and "info edit" clear the field they change.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		tasks, e := a.ListFilteredRefs(all, statuses, labels, refs)
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
	c.Flags().BoolVar(&all, "all", false, "include done and dropped tasks")
	c.Flags().StringSliceVar(&statuses, "status", nil, "only tasks with this status (repeatable; may be a closed status)")
	c.Flags().StringSliceVar(&labels, "label", nil, "only tasks with this label (repeatable; all must match)")
	c.Flags().StringArrayVar(&refs, "ref", nil, "only tasks with this ref, as <jira|pr|url>:<value> (repeatable; all must match)")
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
	c := &cobra.Command{Use: "info", Short: "Show a task record and, inside a tasktree, its repositories", Long: `Show the task record: fields, refs, subtasks, description and log. For a task with a checkout, also show the root's uncommitted changes and a table of its repositories (mode, branch, HEAD, state, ahead/behind, unpushed). For a task with a definition but no checkout here, show the defined repositories and their recorded commits.

The task is -t, else TASKTREE_TASK, else the focused subtask, else the task of the tasktree you are in.

With --json the output is one JSON object; see the JSON output reference.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.AddCommand(infoEditCmd(o), infoSetCmd(o), infoAckCmd(o))
	return c
}

func infoEditCmd(o *options) *cobra.Command {
	var ref, file string
	var stdin bool
	c := &cobra.Command{Use: "edit", Short: "Replace a task description", Long: `Replace the whole description. The new text comes from --file, from --stdin, or from your editor ($VISUAL, then $EDITOR, then vi) on a temporary file holding the current text.

A description may not contain the reserved headings "## Description" or "## Log". If the text is unchanged, nothing is committed.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&file, "file", "", "read the new description from this file")
	c.Flags().BoolVar(&stdin, "stdin", false, "read the new description from standard input")
	return c
}
func infoSetCmd(o *options) *cobra.Command {
	var ref string
	var clear bool
	c := &cobra.Command{Use: "set <field> [value]", Short: "Set a task title, handle, or parent (acknowledging that field's conflict)", Long: `Set one record field: title, handle or parent. A handle must be unique and match [A-Za-z0-9._-], at most 64 characters. A parent is a handle or id; --clear removes it. Reparenting only changes the record; branch names of existing subtask checkouts stay as they are.

Status is not settable here; use "mark".`, Args: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().BoolVar(&clear, "clear", false, "remove the parent (only valid for the parent field)")
	return c
}
func infoAckCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "ack <field>", Short: "Acknowledge a conflict flag without changing the field", Long: "Remove one field from the record's conflict list. Sync sets the list when both machines changed the same field; the remote value is kept in a comment entry in the log. Exits 3 if the field is not flagged.", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		resolved, _, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		t, e := a.AckConflict(cmd.Context(), resolved, args[0])
		if e != nil {
			return e
		}
		fmt.Fprintf(o.out, "conflict on %s acknowledged for %s\n", args[0], t.Handle)
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}
func noteCmd(o *options) *cobra.Command {
	var ref, kind string
	c := &cobra.Command{Use: "note <text>", Short: "Append a note, decision, comment, or ref to a task log", Long: `Append an entry to the task log. The entry records the author, machine and time. The text may not contain reserved headings or a log entry header line.

The author is --author, else TASKTREE_AUTHOR, else the author config key. It must not contain spaces or "@".`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&kind, "kind", "note", "entry kind: note, comment, decision or ref")
	return c
}
func markCmd(o *options) *cobra.Command {
	var ref string
	var markOptions app.MarkOptions
	c := &cobra.Command{Use: "mark <status>", Short: "Set task status; closing tears down the local tasktree", Long: `Set the status to incoming, next, active, blocked, done or dropped. This is the only command that changes status.

Closing a task (done or dropped) that has a checkout on this machine runs the close flow:
  1. Refuse if the task has open subtasks (see --cascade).
  2. Refuse if work would be lost: uncommitted changes, commits not on origin or a stash in any repository, ignored files or nested git repositories in the root, or a repository whose HEAD is older than the commit recorded for it (see --force).
  3. Commit the root as "close: <status>" and push the task branch to the local tasks repository.
  4. Record the status, then push main and the task branch to the remote, best effort, if one is configured.
  5. Delete the checkout directory, its hidden git directory and its local.yml entry.
The task branch stays, so the task can be reopened later.

Closing a task without a checkout only changes the record.`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		for _, removed := range append([]string{result.RemovedAt}, result.Removed...) {
			if removed != "" && pathWithin(cwd, removed) {
				fmt.Fprintln(o.err, "tasktree removed; run: cd ..")
				break
			}
		}
		if e == nil {
			fmt.Fprintf(o.out, "%s marked %s\n", result.Task.Handle, result.Task.Status)
		}
		return e
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	c.Flags().BoolVar(&markOptions.Keep, "keep", false, "change status only; keep the checkout")
	c.Flags().BoolVar(&markOptions.Force, "force", false, "close even if work would be lost; the loss is listed on stderr")
	c.Flags().BoolVar(&markOptions.Cascade, "cascade", false, "close all open subtasks first, deepest first; all gates are checked before any change")
	return c
}

func refCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "ref", Short: "Manage typed task references", Long: "Add or remove references to external items. Types: jira (a key such as PROJ-123), pr (owner/repo#N) and url. References are stored on the record; tasktree never calls the external system."}
	for _, action := range []string{"add", "rm"} {
		action := action
		root.AddCommand(&cobra.Command{Use: action + " <type> <value>", Short: map[string]string{"add": "Add a task reference (types: jira, pr, url)", "rm": "Remove a task reference (exits 3 if absent)"}[action], Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
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
	root := &cobra.Command{Use: "label", Short: "Manage task labels", Long: "Add or remove labels. A label matches [a-z0-9][a-z0-9._-]*."}
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
	c := &cobra.Command{Use: "history", Short: "Show a task record's git history", Long: "Show the commits that changed the task record, newest first: time, git author and subject. This is the record's changelog; there is no separate stored one.", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	within := func(path, root string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return within(path, root) || within(realPath(path), realPath(root))
}

// realPath resolves symlinks in the longest existing prefix of path, so a
// directory that was just deleted still compares equal to its former alias.
func realPath(path string) string {
	rest := ""
	for p := path; ; {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
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
		if r.Uninitialized {
			state = "uninitialized"
		}
		rows = append(rows, output.RepoRow{Name: r.Name, Mode: r.Mode, Branch: r.Branch, Head: r.Head, State: state, Ahead: r.Ahead, Behind: r.Behind, Unpushed: r.Unpushed})
	}
	return output.WriteRepoTable(w, rows)
}

func openCmd(o *options) *cobra.Command {
	var x app.OpenOptions
	var repos []string
	c := &cobra.Command{Use: "open <handle|id>", Short: "Materialize or locate a task's tasktree; prints its path", Long: `Print the path of the task's checkout, creating it first when this machine has none.

If a remote is configured and the task has no checkout here, open first fetches main and the task branch, best effort. The checkout is created at <tasksRoot>/<handle> unless --path is given. Repositories are cloned from the local cache, attached to their branches, and bootstrap steps run unless --skip-bootstrap is set. A repository whose recorded commit is not available yet is left uninitialized and reported; sync or the next open retries it.

If the task already has a checkout here, open only prints its path. A closed task cannot be opened. A subtask without its own tasktree becomes an isolated subtask: it gets a tasktree with the parent's work repositories (or --repos), on branch <parent-branch>-<subtask-handle> (or --branch). The parent must be checked out.`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&x.Path, "path", "", "checkout path (default <tasksRoot>/<handle>)")
	c.Flags().BoolVar(&x.Force, "force", false, "replace an existing hidden git directory for this task; work in it is listed first")
	c.Flags().BoolVar(&x.SkipBootstrap, "skip-bootstrap", false, "do not run the bootstrap steps")
	c.Flags().StringSliceVar(&repos, "repos", nil, "parent repositories to copy when promoting a subtask")
	c.Flags().StringVar(&x.Branch, "branch", "", "branch for the work repositories when promoting a subtask")
	return c
}

// noCheckout reports a structural command aimed at a task without a checkout
// on this machine. An empty id means no task was targeted at all.
// templateLoadError keeps a missing template a typed not-found (exit 3) and any
// other load failure a usage error.
func templateLoadError(e error) error {
	var nf template.NotFoundError
	if errors.As(e, &nf) {
		return e
	}
	return usageError{e.Error()}
}

func noCheckout(a app.App, id string) error {
	if id == "" {
		return notInTasktreeError{}
	}
	t, err := a.Info(id)
	if err != nil {
		return err
	}
	return app.NoCheckoutError{Handle: t.Handle}
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
		return "", "", noCheckout(a, id)
	}
	return id, root, nil
}
func addCmd(o *options) *cobra.Command {
	var ref string
	var x app.AddOptions
	c := &cobra.Command{Use: "add <alias|url>", Short: "Add a repository to the tasktree", Long: `Add a repository as a full clone and commit the change. The argument is an alias from aliases.yml or a clone URL. The clone comes from the local cache, which is fetched first. Aliases for the URL are registered automatically.

Branch and mode follow from the flags:

- Neither --branch nor --from: follow the remote default branch (mode follow).
- --branch B, B exists: check out B (mode work).
- --branch B, B is new: create B from --from, else from the default branch (mode work).
- --from F, F is a branch: follow F (mode follow).
- --from F, F is a tag or commit: detached at F (mode pin).

--mode overrides the derived mode. Modes follow and work need a branch.

With -t and no checkout on this machine, the repository is added to the task's definition and cloned on the next open.`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&x.Branch, "branch", "", "branch to work on; created if it does not exist")
	c.Flags().StringVar(&x.From, "from", "", "branch, tag or commit to start from")
	c.Flags().StringVar(&x.Name, "name", "", "directory name (default: last part of the URL)")
	c.Flags().StringVar(&x.Mode, "mode", "", "follow, work or pin (default derived from --branch and --from)")
	return c
}
func removeCmd(o *options) *cobra.Command {
	var ref string
	var force bool
	c := &cobra.Command{Use: "remove <name>", Short: "Remove a repository from the tasktree", Long: `Delete the repository directory and its hidden git directory, and commit the removal. Refuses when the repository has uncommitted changes, commits not on origin, or a stash; --force lists them and proceeds. With -t and no checkout here, only the definition changes.`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().BoolVar(&force, "force", false, "remove even if work would be lost; the loss is listed on stderr")
	return c
}
func commitCmd(o *options) *cobra.Command {
	var ref, msg string
	var allowRewind bool
	c := &cobra.Command{Use: "commit", Short: "Record root files and repository commits on the task branch", Long: `Commit everything in the tasktree root (files, symlinks and the current commit of every repository) on the task branch, and push it to the local tasks repository. Prints "nothing to commit" when the root is clean. Never commits inside repositories; repositories with uncommitted changes produce a warning.

Refuses to record a repository commit that is older than the one already recorded, unless --allow-rewind is set.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVarP(&msg, "message", "m", "", "commit message (default: tasktree(<id>): checkpoint)")
	c.Flags().BoolVar(&allowRewind, "allow-rewind", false, "allow recording a repository commit older than the one already recorded")
	return c
}

type doctorFindingsError struct{}

func (doctorFindingsError) Error() string { return "doctor found errors" }

func doctorCmd(o *options) *cobra.Command {
	var fix, jsonOut bool
	c := &cobra.Command{Use: "doctor", Short: "Check tasktree state and safely repair known problems", Long: `Check the tasks repository, records, checkouts and hidden git directories. Each finding has a code, a severity (error or warning) and a flag saying whether --fix can repair it.

--fix applies only repairs that cannot discard work, then checks again. Exits 1 when errors remain; warnings alone exit 0.`, Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().BoolVar(&fix, "fix", false, "apply the repairs that cannot lose work, then check again")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return c
}

func moveCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "move <new-path>", Short: "Relocate a tasktree checkout atomically", Long: `Move a checkout to a new path on the same filesystem, rewrite the repository paths git keeps, update local.yml and verify that every repository is unchanged. On any failure the move is rolled back. The target must not exist or must be empty. Shells that sit in the old directory keep the old path; cd to the new one.`, Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	root := &cobra.Command{Use: "template", Short: "List, show, or validate task templates", Long: "Templates create a task and, optionally, its tasktree. They come from ~/.tasktree/templates and from the built-in set (bugfix, feature, investigation). A template in your directory replaces a built-in one with the same name."}
	var listJSON, showJSON bool
	list := &cobra.Command{Use: "list", Short: "List available templates", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		all, e := a.TemplateStore().List()
		if e != nil {
			return e
		}
		if listJSON {
			rows := make([]app.TemplateSummary, 0, len(all))
			for _, s := range all {
				rows = append(rows, app.SummarizeTemplate(s))
			}
			return json.NewEncoder(o.out).Encode(rows)
		}
		tw := newTable(o.out)
		fmt.Fprintln(tw, "NAME\tSOURCE\tDESCRIPTION")
		for _, s := range all {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Metadata.Name, s.Source, s.Metadata.Description)
		}
		return tw.Flush()
	}}
	list.Flags().BoolVar(&listJSON, "json", false, "emit JSON")
	show := &cobra.Command{Use: "show <name|file>", Short: "Show template parameters and raw YAML", Long: "Print the parameter table and the template YAML. The argument is a template name or a path to a file.", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		s, e := a.TemplateStore().Load(args[0])
		if e != nil {
			return templateLoadError(e)
		}
		if showJSON {
			return json.NewEncoder(o.out).Encode(app.DescribeTemplate(s))
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
	}}
	show.Flags().BoolVar(&showJSON, "json", false, "emit JSON")
	root.AddCommand(list, show,
		&cobra.Command{Use: "validate <name|file>", Short: "Validate a template", Long: "Check apiVersion, kind, metadata.name and that every {{variable}} is declared. The argument is a template name or a path to a file. Exits 2 when invalid, 3 when not found.", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
			a, e := o.application()
			if e != nil {
				return e
			}
			s, e := a.TemplateStore().Load(args[0])
			if e != nil {
				return templateLoadError(e)
			}
			if e = template.Validate(s); e != nil {
				return usageError{e.Error()}
			}
			fmt.Fprintf(o.out, "template %s is valid\n", s.Metadata.Name)
			return nil
		}})
	return root
}

func aliasCmd(o *options) *cobra.Command {
	root := &cobra.Command{Use: "alias", Short: "Manage repository aliases", Long: "Aliases map short names to clone URLs in ~/.tasktree/aliases.yml. add registers aliases for each repository automatically."}
	root.AddCommand(&cobra.Command{Use: "add <alias> <url>", Short: "Register a repository alias", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		changed, e := a.AddAlias(args[0], args[1])
		if e != nil {
			return e
		}
		if changed {
			fmt.Fprintf(o.out, "added alias %s -> %s\n", args[0], args[1])
		} else {
			fmt.Fprintf(o.out, "alias %s -> %s unchanged\n", args[0], args[1])
		}
		return nil
	}}, &cobra.Command{Use: "rm <alias>", Short: "Remove a repository alias", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		if e = a.RemoveAlias(args[0]); e != nil {
			return e
		}
		fmt.Fprintf(o.out, "removed alias %s\n", args[0])
		return nil
	}})
	var j bool
	list := &cobra.Command{Use: "list", Short: "List repository aliases", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		aliases, e := a.ListAliases()
		if e != nil {
			return e
		}
		if j {
			enc := json.NewEncoder(o.out)
			enc.SetEscapeHTML(false)
			return enc.Encode(aliases)
		}
		tw := newTable(o.out)
		fmt.Fprintln(tw, "ALIAS\tURL")
		for _, x := range aliases {
			fmt.Fprintf(tw, "%s\t%s\n", x.Alias, x.URL)
		}
		return tw.Flush()
	}}
	list.Flags().BoolVar(&j, "json", false, "emit JSON")
	root.AddCommand(list)
	return root
}

func rootCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "root", Short: "Print the tasktree root path", Long: "Print the checkout directory of the task. Exits 4 if the task has no checkout on this machine.", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := resolveTarget(a, ref)
		if e != nil {
			return e
		}
		if root == "" {
			return noCheckout(a, id)
		}
		fmt.Fprintln(o.out, root)
		return nil
	}}
	c.Flags().StringVarP(&ref, "task", "t", "", "task handle or id")
	return c
}

func modeCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "mode <repo> <follow|work|pin>", Short: "Change a repository mode", Long: `Set the mode of a repository and commit the change. Modes: follow (fast-forward to origin/<branch>), work (fast-forward your branch; never rebase) and pin (never moves). Switching to follow or work records the current branch of the repository; switching to pin removes the recorded branch.`, Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
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
	runSteps := func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := requireTree(a, ref)
		if e != nil {
			return e
		}
		return a.Bootstrap(cmd.Context(), id, root, o.err)
	}
	c := &cobra.Command{Use: "bootstrap", Short: "Run, add, remove, or list the tasktree bootstrap steps", Long: `Bootstrap steps are shell commands stored in .tasktree/config.yml. They run in order, stop at the first failure, and run on "open" when a checkout is created. Without a subcommand, "bootstrap" runs them now. Each step gets TASKTREE_ROOT set to the tasktree root.`, Args: exact(0), RunE: runSteps}
	c.PersistentFlags().StringVarP(&ref, "task", "t", "", "task handle or id")
	// target resolves a structural command to a checkout or, with -t, a
	// definition without one (D45, D47).
	target := func(a app.App) (string, string, error) {
		id, root, e := a.ResolveTreeTarget(ref, "")
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				return "", "", notInTasktreeError{}
			}
			return "", "", e
		}
		if root == "" && ref == "" {
			return "", "", notInTasktreeError{}
		}
		return id, root, nil
	}
	run := &cobra.Command{Use: "run", Short: "Run the tasktree bootstrap steps", Args: exact(0), RunE: runSteps}
	var workdir string
	var envPairs []string
	add := &cobra.Command{Use: "add <name> <run>", Short: "Add a bootstrap step and commit it", Long: "Append a step. The name must be unique. The command runs with sh -c.", Args: exact(2), RunE: func(cmd *cobra.Command, args []string) error {
		env, e := app.ParseEnvAssignments(envPairs)
		if e != nil {
			return e
		}
		step := bootstrap.Step{Name: args[0], Run: args[1], Workdir: workdir, Env: env}
		if e = app.ValidateBootstrapStep(step); e != nil {
			return e
		}
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := target(a)
		if e != nil {
			return e
		}
		if e = a.BootstrapAdd(cmd.Context(), id, root, step); e != nil {
			return e
		}
		fmt.Fprintf(o.out, "added bootstrap step %s\n", args[0])
		return nil
	}}
	add.Flags().StringVar(&workdir, "workdir", "", "directory to run in, relative to the tasktree root (default: the root)")
	add.Flags().StringArrayVar(&envPairs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	rm := &cobra.Command{Use: "rm <name>", Short: "Remove a bootstrap step and commit the change", Args: exact(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := target(a)
		if e != nil {
			return e
		}
		if e = a.BootstrapRemove(cmd.Context(), id, root, args[0]); e != nil {
			return e
		}
		fmt.Fprintf(o.out, "removed bootstrap step %s\n", args[0])
		return nil
	}}
	var j bool
	list := &cobra.Command{Use: "list", Short: "List bootstrap steps", Args: exact(0), RunE: func(cmd *cobra.Command, args []string) error {
		a, e := o.application()
		if e != nil {
			return e
		}
		id, root, e := target(a)
		if e != nil {
			return e
		}
		steps, e := a.BootstrapSteps(cmd.Context(), id, root)
		if e != nil {
			return e
		}
		if j {
			return json.NewEncoder(o.out).Encode(steps)
		}
		tw := newTable(o.out)
		fmt.Fprintln(tw, "NAME\tWORKDIR\tRUN")
		for _, s := range steps {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Workdir, s.Run)
		}
		return tw.Flush()
	}}
	list.Flags().BoolVar(&j, "json", false, "emit JSON")
	c.AddCommand(run, add, rm, list)
	return c
}
func focusCmd(o *options) *cobra.Command {
	var clear bool
	c := &cobra.Command{Use: "focus [handle|id]", Short: "Focus a shared descendant in this tasktree", Long: `Make record commands (info, note, ref, label, mark, history) in this checkout target a shared subtask instead of the task that owns the tasktree. Structural commands are not affected. Focus is stored per checkout in local.yml and clears when the subtask is closed. Use TASKTREE_TASK instead when several agents share one checkout.`, Args: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().BoolVar(&clear, "clear", false, "clear the focus")
	return c
}
func pushCmd(o *options) *cobra.Command {
	var ref string
	c := &cobra.Command{Use: "push [repo...]", Short: "Push repository branches and print PR hints", Long: `Push repository branches to origin. Without arguments, only repositories in work mode are pushed. Name follow or pin repositories to push them too. All selected repositories are checked before any push: a detached HEAD, and the branches main, master, release/* and the remote default branch are refused.

For an isolated subtask, push checks that the parent's branch on origin contains the subtask's fork point, warns if not, and prints a "gh pr create" line with the parent branch as base. It never opens a pull request.`, Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
