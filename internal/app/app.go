package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/fsx"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type App struct {
	Home           home.Home
	Hub            hub.Hub
	Git            gitx.Client
	Now            func() time.Time
	AuthorOverride string
	WarningWriter  io.Writer
}

func New(h home.Home, verboseWriter interface{ Write([]byte) (int, error) }) App {
	g := gitx.NewClient()
	if verboseWriter != nil {
		g.SetVerboseWriter(verboseWriter)
	}
	return App{Home: h, Hub: hub.New(h.GitPath(), g), Git: g, Now: time.Now, WarningWriter: os.Stderr}
}

type SetupOptions struct {
	TasksRoot, Author, Machine, Remote             string
	TasksRootSet, AuthorSet, MachineSet, RemoteSet bool
}

func (a App) MigrateV1Aliases() (bool, error) {
	dest := a.Home.AliasesPath()
	if _, err := os.Stat(dest); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	source := os.Getenv("TASKTREE_V1_ALIASES_FILE")
	if source == "" {
		u, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		source = filepath.Join(u, ".config", "tasktree", "repos.yml")
	}
	b, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read V1 aliases: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func (a App) Setup(ctx context.Context, o SetupOptions) (home.Config, error) {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return c, err
	}
	if o.TasksRootSet {
		c.TasksRoot = o.TasksRoot
	}
	if o.AuthorSet {
		c.Author = o.Author
	}
	if o.MachineSet {
		c.Machine = o.Machine
	}
	if o.RemoteSet {
		c.Remote = o.Remote
	}
	if err := a.Home.SaveConfig(c); err != nil {
		return c, err
	}
	if err := a.Hub.Init(ctx, c.Author, c.Machine); err != nil {
		return c, err
	}
	return c, nil
}
func (a App) ConfigGet(key string) (string, error) {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return "", err
	}
	switch key {
	case "remote":
		return c.Remote, nil
	case "tasksRoot":
		return c.TasksRoot, nil
	case "author":
		return c.Author, nil
	case "machine":
		return c.Machine, nil
	case "jira.baseUrl":
		return c.Jira.BaseURL, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}
func (a App) ConfigSet(key, value string) error {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return err
	}
	switch key {
	case "remote":
		c.Remote = value
	case "tasksRoot":
		c.TasksRoot = value
	case "author":
		c.Author = value
	case "machine":
		c.Machine = value
	case "jira.baseUrl":
		c.Jira.BaseURL = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return a.Home.SaveConfig(c)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(title string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

type NewOptions struct{ Title, Jira, Handle, Status, Parent string }

func (a App) NewTask(ctx context.Context, o NewOptions) (record.Task, error) {
	if !a.Hub.IsSetup() {
		return record.Task{}, hub.NotSetupError{}
	}
	if o.Title == "" {
		return record.Task{}, fmt.Errorf("title must not be empty")
	}
	if o.Status == "" {
		o.Status = "incoming"
	}
	if err := record.ValidateStatus(o.Status); err != nil {
		return record.Task{}, err
	}
	if o.Handle == "" {
		if o.Jira != "" {
			o.Handle = o.Jira
		} else {
			o.Handle = Slug(o.Title)
		}
	}
	if o.Handle == "" {
		return record.Task{}, fmt.Errorf("could not derive a handle; pass --handle")
	}
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return record.Task{}, err
	}
	for _, d := range docs {
		if d.Task.Handle == o.Handle {
			return record.Task{}, hub.ConflictError{Message: fmt.Sprintf("handle %q already exists; try %q", o.Handle, nextHandle(o.Handle, docs))}
		}
	}
	var parent *string
	if o.Parent != "" {
		p, err := hub.Resolve(docs, o.Parent)
		if err != nil {
			return record.Task{}, err
		}
		v := p.Task.ID
		parent = &v
	}
	id, err := record.NewID()
	if err != nil {
		return record.Task{}, err
	}
	now := a.Now().UTC()
	refs := []record.Ref{}
	if o.Jira != "" {
		refs = append(refs, record.Ref{Type: "jira", Value: o.Jira})
	}
	d, err := record.New(record.Task{ID: id, Handle: o.Handle, Title: o.Title, Status: o.Status, Parent: parent, Labels: []string{}, Refs: refs, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return record.Task{}, err
	}
	c, _ := a.Home.LoadConfig()
	if err := a.Hub.Create(ctx, d, fmt.Sprintf("create %q", o.Title), c.Author, c.Machine); err != nil {
		return record.Task{}, err
	}
	return d.Task, nil
}
func nextHandle(base string, docs []*record.Document) string {
	for n := 2; ; n++ {
		v := fmt.Sprintf("%s-%d", base, n)
		ok := true
		for _, d := range docs {
			if d.Task.Handle == v {
				ok = false
				break
			}
		}
		if ok {
			return v
		}
	}
}
func (a App) List(all bool, statuses []string) ([]record.Task, error) {
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, s := range statuses {
		if err := record.ValidateStatus(s); err != nil {
			return nil, err
		}
		wanted[s] = true
	}
	out := []record.Task{}
	for _, d := range docs {
		include := all || len(wanted) > 0 && wanted[d.Task.Status] || len(wanted) == 0 && record.IsOpen(d.Task.Status)
		if include {
			out = append(out, d.Task)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (a App) Info(ref string) (record.Task, error) {
	d, err := a.Hub.Read(ref)
	if err != nil {
		return record.Task{}, err
	}
	return d.Task, nil
}

func (a App) warnf(format string, args ...any) {
	if a.WarningWriter != nil {
		_, _ = fmt.Fprintf(a.WarningWriter, "warning: "+format+"\n", args...)
	}
}

// writeGeneratedTaskRecord updates the gitignored task view for a valid live
// checkout. It deliberately only warns: task records have already committed in
// the hub and remain the source of truth if a checkout cannot be refreshed.
func (a App) writeGeneratedTaskRecord(id string, raw []byte) {
	local, err := a.Home.LoadLocal()
	if err != nil {
		a.warnf("refresh .tasktree/task.md for %s: %v", id, err)
		return
	}
	checkout, ok := local.Checkouts[id]
	if !ok {
		return
	}
	if st, err := os.Stat(checkout.Path); err != nil || !st.IsDir() {
		return
	}
	markerID, err := markerTaskID(checkout.Path)
	if err != nil || markerID != id {
		return
	}
	content := append([]byte(fmt.Sprintf("<!-- generated by tt2 from tasks/%s.md; do not edit — use tt2 info / tt2 note -->\n\n", id)), raw...)
	if err := fsx.AtomicWriteFile(filepath.Join(checkout.Path, ".tasktree", "task.md"), content, 0o644); err != nil {
		a.warnf("refresh %s/.tasktree/task.md: %v", checkout.Path, err)
	}
}

// refreshTaskRecord serializes the snapshot with record writes so a delayed
// open cannot replace a newer generated view.
func (a App) refreshTaskRecord(ctx context.Context, id string) {
	d, err := a.Hub.ReadSerialized(ctx, id)
	if err != nil {
		a.warnf("refresh .tasktree/task.md for %s: %v", id, err)
		return
	}
	a.writeGeneratedTaskRecord(id, d.Render())
}
func (a App) author() (string, string, error) {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return "", "", err
	}
	author := a.AuthorOverride
	if author == "" {
		author = os.Getenv("TASKTREE_AUTHOR")
	}
	if author == "" {
		author = c.Author
	}
	if author == "" {
		return "", "", fmt.Errorf("author is not configured; pass --author or set TASKTREE_AUTHOR")
	}
	if strings.ContainsAny(author, "@ \t\r\n") || strings.ContainsAny(c.Machine, " \t\r\n") || c.Machine == "" {
		return "", "", fmt.Errorf("author and machine must be non-empty and contain no spaces or @")
	}
	return author, c.Machine, nil
}
func (a App) Note(ctx context.Context, ref, text, kind string) (record.Task, error) {
	if !a.Hub.IsSetup() {
		return record.Task{}, hub.NotSetupError{}
	}
	if kind == "" {
		kind = "note"
	}
	if err := record.ValidateKind(kind); err != nil {
		return record.Task{}, err
	}
	author, machine, err := a.author()
	if err != nil {
		return record.Task{}, err
	}
	now := a.Now().UTC()
	return a.Hub.UpdateWithPostCommit(ctx, ref, fmt.Sprintf("%s added", kind), author, machine, func(d *record.Document) error {
		for {
			id, err := record.NewID()
			if err != nil {
				return err
			}
			duplicate := false
			for _, entry := range d.Task.Log {
				if entry.ID == id {
					duplicate = true
					break
				}
			}
			if !duplicate {
				return d.Append(record.Entry{ID: id, Timestamp: now, Kind: kind, Author: author, Machine: machine, Body: text})
			}
		}
	}, func(task record.Task, raw []byte) {
		a.writeGeneratedTaskRecord(task.ID, raw)
	})
}

type MarkOptions struct {
	Keep  bool
	Force bool
}

type MarkResult struct {
	Task      record.Task
	Loss      []string
	RemovedAt string
	Remaining []string
}

func (a App) updateStatus(ctx context.Context, ref, status string) (record.Task, error) {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return record.Task{}, err
	}
	old, err := a.Info(ref)
	if err != nil {
		return record.Task{}, err
	}
	return a.Hub.UpdateWithPostCommit(ctx, ref, fmt.Sprintf("status %s → %s", old.Status, status), c.Author, c.Machine, func(d *record.Document) error { return d.SetStatus(status, a.Now().UTC()) }, func(task record.Task, raw []byte) {
		a.writeGeneratedTaskRecord(task.ID, raw)
	})
}

func (a App) Mark(ctx context.Context, ref, status string, o MarkOptions) (MarkResult, error) {
	result := MarkResult{}
	if err := record.ValidateStatus(status); err != nil {
		return result, err
	}
	task, err := a.Info(ref)
	if err != nil {
		return result, err
	}
	closing := !record.IsOpen(status)
	if closing {
		docs, readErr := a.Hub.ReadAll()
		if readErr != nil {
			return result, readErr
		}
		openChildren := []string{}
		for _, d := range docs {
			if d.Task.Parent != nil && *d.Task.Parent == task.ID && record.IsOpen(d.Task.Status) {
				openChildren = append(openChildren, fmt.Sprintf("%s (%s)", d.Task.Handle, d.Task.Status))
			}
		}
		if len(openChildren) > 0 {
			sort.Strings(openChildren)
			return result, GateError{"task has open subtasks:\n" + strings.Join(openChildren, "\n")}
		}
	}
	if !closing || o.Keep {
		result.Task, err = a.updateStatus(ctx, task.ID, status)
		return result, err
	}
	local, err := a.Home.LoadLocal()
	if err != nil {
		return result, err
	}
	checkout, checkedOut := local.Checkouts[task.ID]
	if !checkedOut {
		result.Task, err = a.updateStatus(ctx, task.ID, status)
		return result, err
	}
	if st, statErr := os.Stat(checkout.Path); statErr != nil || !st.IsDir() {
		result.Task, err = a.updateStatus(ctx, task.ID, status)
		return result, err
	}
	result.Loss, err = a.tasktreeLoss(ctx, task.ID, checkout.Path)
	if err != nil {
		return result, err
	}
	if len(result.Loss) > 0 && !o.Force {
		return result, GateError{strings.Join(result.Loss, "\n")}
	}
	if _, _, err = a.Commit(ctx, task.ID, checkout.Path, fmt.Sprintf("tasktree(%s): close: %s", task.ID, status)); err != nil {
		return result, err
	}
	// Close is complete only when the hub task branch exactly matches the clone.
	g := a.Git.WithDefaults()
	cloneHead, _, e1 := rootGit(ctx, g, a.Home, task.ID, checkout.Path, "rev-parse", "HEAD")
	hubHead, _, e2 := git(ctx, g, a.Home.GitPath(), "rev-parse", "refs/heads/task/"+task.ID)
	if e1 != nil || e2 != nil || strings.TrimSpace(cloneHead) != strings.TrimSpace(hubHead) {
		return result, GateError{"task branch synchronization could not be verified; rerun the command"}
	}
	result.Task, err = a.updateStatus(ctx, task.ID, status)
	if err != nil {
		return result, err
	}
	// Recheck immediately before destructive teardown. A concurrent write after
	// the close commit must be retained for the user to recover.
	lateLoss, lateErr := a.tasktreeLoss(ctx, task.ID, checkout.Path)
	if lateErr != nil {
		return result, GateError{"task closed but loss recheck failed: " + lateErr.Error() + "; push and rerun tt2 mark " + status}
	}
	if len(lateLoss) > 0 && !o.Force {
		result.Loss = append(result.Loss, lateLoss...)
		return result, GateError{strings.Join(lateLoss, "\n") + "\ntask is closed; push and rerun tt2 mark " + status}
	}
	result.RemovedAt = checkout.Path
	result.Remaining = a.teardownTasktree(task.ID, checkout.Path, local)
	if len(result.Remaining) > 0 {
		return result, fmt.Errorf("task closed, but teardown incomplete; remains: %s", strings.Join(result.Remaining, ", "))
	}
	return result, nil
}
