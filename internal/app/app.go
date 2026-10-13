package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/fsx"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

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
	if !a.Hub.IsSetup() && c.Remote != "" {
		if err := a.Hub.Clone(ctx, c.Remote); err != nil {
			return c, err
		}
		refs, err := a.Hub.Refs(ctx, "refs/remotes/origin/task/")
		if err != nil {
			return c, err
		}
		for ref, sha := range refs {
			if err := a.Hub.MoveRef(ctx, strings.Replace(ref, "refs/remotes/origin/", "refs/heads/", 1), sha, ""); err != nil {
				return c, err
			}
		}
	} else {
		if err := a.Hub.Init(ctx, c.Author, c.Machine); err != nil {
			return c, err
		}
		if o.RemoteSet {
			if err := a.Hub.ConfigureRemote(ctx, c.Remote); err != nil {
				return c, err
			}
		}
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
	case "sync.jobs":
		return strconv.Itoa(c.Sync.Jobs), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}
func (a App) ConfigSet(key, value string) error {
	return a.ConfigSetContext(context.Background(), key, value)
}
func (a App) ConfigSetContext(ctx context.Context, key, value string) error {
	c, err := a.Home.LoadConfig()
	if err != nil {
		return err
	}
	switch key {
	case "remote":
		if err := a.Hub.ConfigureRemote(ctx, value); err != nil {
			return err
		}
		c.Remote = value
	case "tasksRoot":
		c.TasksRoot = value
	case "author":
		c.Author = value
	case "machine":
		c.Machine = value
	case "jira.baseUrl":
		c.Jira.BaseURL = value
	case "sync.jobs":
		jobs, err := strconv.Atoi(value)
		if err != nil || jobs < 1 {
			return ValidationError{"sync.jobs must be a positive integer"}
		}
		c.Sync.Jobs = jobs
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	if err := a.Home.SaveConfig(c); err != nil {
		return err
	}
	if key == "remote" && value != "" {
		return a.pushInitialRemote(ctx)
	}
	return nil
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
	return a.ListFiltered(all, statuses, nil)
}

// ListFiltered returns matching tasks with direct-subtask progress derived from
// all records, rather than just the filtered result set.
func (a App) ListFiltered(all bool, statuses, labels []string) ([]record.Task, error) {
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return nil, err
	}
	if err := validateParentGraph(docs); err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, s := range statuses {
		if err := record.ValidateStatus(s); err != nil {
			return nil, err
		}
		wanted[s] = true
	}
	progress := progressFor(docs)
	out := []record.Task{}
	for _, d := range docs {
		include := all || len(wanted) > 0 && wanted[d.Task.Status] || len(wanted) == 0 && record.IsOpen(d.Task.Status)
		for _, label := range labels {
			found := false
			for _, have := range d.Task.Labels {
				if have == label {
					found = true
					break
				}
			}
			if !found {
				include = false
				break
			}
		}
		if include {
			t := d.Task
			t.Progress = progress[t.ID]
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func validateParentGraph(docs []*record.Document) error {
	byID := map[string]record.Task{}
	for _, d := range docs {
		byID[d.Task.ID] = d.Task
	}
	for _, d := range docs {
		seen := map[string]bool{}
		id := d.Task.ID
		for id != "" {
			if seen[id] {
				return fmt.Errorf("task parent cycle detected at %s", id)
			}
			seen[id] = true
			t, ok := byID[id]
			if !ok || t.Parent == nil {
				break
			}
			id = *t.Parent
		}
	}
	return nil
}

func progressFor(docs []*record.Document) map[string]*record.Progress {
	result := map[string]*record.Progress{}
	for _, d := range docs {
		if d.Task.Parent == nil {
			continue
		}
		p := result[*d.Task.Parent]
		if p == nil {
			p = &record.Progress{}
			result[*d.Task.Parent] = p
		}
		p.Total++
		if !record.IsOpen(d.Task.Status) {
			p.Closed++
		}
	}
	return result
}
func (a App) Info(ref string) (record.Task, error) {
	d, err := a.Hub.Read(ref)
	if err != nil {
		return record.Task{}, err
	}
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return record.Task{}, err
	}
	if err := validateParentGraph(docs); err != nil {
		return record.Task{}, err
	}
	d.Task.Progress = progressFor(docs)[d.Task.ID]
	return d.Task, nil
}

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var prPattern = regexp.MustCompile(`^[^/[:space:]]+/[^/#[:space:]]+#[1-9][0-9]*$`)

func (a App) EditDescription(ctx context.Context, ref, text string) (record.Task, bool, error) {
	if err := record.ValidateDescription(text); err != nil {
		return record.Task{}, false, err
	}
	current, err := a.Info(ref)
	if err != nil {
		return record.Task{}, false, err
	}
	if current.Description == text && !hasConflict(current, "description") {
		return current, false, nil
	}
	author, machine, err := a.author()
	if err != nil {
		return record.Task{}, false, err
	}
	t, err := a.Hub.UpdateWithPostCommit(ctx, current.ID, "description updated", author, machine, func(d *record.Document) error {
		if err := d.ClearConflict("description"); err != nil {
			return err
		}
		return d.SetDescription(text, a.Now().UTC())
	}, func(task record.Task, raw []byte) { a.writeGeneratedTaskRecord(task.ID, raw) })
	return t, err == nil, err
}

func (a App) SetInfo(ctx context.Context, ref, field, value string, clear bool) (record.Task, bool, error) {
	current, err := a.Info(ref)
	if err != nil {
		return record.Task{}, false, err
	}
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return record.Task{}, false, err
	}
	var summary string
	switch field {
	case "title":
		if value == "" {
			return record.Task{}, false, ValidationError{"title must not be empty"}
		}
		if current.Title == value && !hasConflict(current, "title") {
			return current, false, nil
		}
		summary = "title updated"
	case "handle":
		if !handlePattern.MatchString(value) {
			return record.Task{}, false, ValidationError{fmt.Sprintf("invalid handle %q (expected [A-Za-z0-9._-], max 64 characters)", value)}
		}
		if current.Handle == value && !hasConflict(current, "handle") {
			return current, false, nil
		}
		for _, d := range docs {
			if d.Task.ID != current.ID && d.Task.Handle == value {
				return record.Task{}, false, hub.ConflictError{Message: fmt.Sprintf("handle %q already exists", value)}
			}
		}
		summary = "handle updated"
	case "status":
		if err := record.ValidateStatus(value); err != nil {
			return record.Task{}, false, ValidationError{err.Error()}
		}
		if current.Status == value && !hasConflict(current, "status") {
			return current, false, nil
		}
		summary = "status acknowledged"
	case "parent":
		var parent *string
		if !clear {
			if value == "" {
				return record.Task{}, false, ValidationError{"parent requires a handle or id, or --clear"}
			}
			p, e := hub.Resolve(docs, value)
			if e != nil {
				return record.Task{}, false, e
			}
			if p.Task.ID == current.ID {
				return record.Task{}, false, ValidationError{"a task cannot be its own parent"}
			}
			parentID := p.Task.ID
			for parentID != "" {
				if parentID == current.ID {
					return record.Task{}, false, ValidationError{"parent would create a cycle"}
				}
				var next *string
				for _, d := range docs {
					if d.Task.ID == parentID {
						next = d.Task.Parent
						break
					}
				}
				if next == nil {
					break
				}
				parentID = *next
			}
			parent = &p.Task.ID
		}
		if !hasConflict(current, "parent") && ((current.Parent == nil && parent == nil) || (current.Parent != nil && parent != nil && *current.Parent == *parent)) {
			return current, false, nil
		}
		if parent == nil {
			summary = "parent cleared"
		} else {
			summary = "parent set to " + *parent
		}
	default:
		return record.Task{}, false, ValidationError{fmt.Sprintf("unknown info field %q (expected title, handle, status, or parent)", field)}
	}
	author, machine, err := a.author()
	if err != nil {
		return record.Task{}, false, err
	}
	t, err := a.Hub.UpdateWithPostCommit(ctx, current.ID, summary, author, machine, func(d *record.Document) error {
		if err := d.ClearConflict(field); err != nil {
			return err
		}
		latest, readErr := a.Hub.ReadAll()
		if readErr != nil {
			return readErr
		}
		switch field {
		case "status":
			return d.SetStatus(value, a.Now().UTC())
		case "title":
			return d.SetFrontString(field, value, a.Now().UTC())
		case "handle":
			for _, candidate := range latest {
				if candidate.Task.ID != d.Task.ID && candidate.Task.Handle == value {
					return hub.ConflictError{Message: fmt.Sprintf("handle %q already exists", value)}
				}
			}
			return d.SetFrontString(field, value, a.Now().UTC())
		default:
			if clear {
				return d.SetParent(nil, a.Now().UTC())
			}
			p, resolveErr := hub.Resolve(latest, value)
			if resolveErr != nil {
				return resolveErr
			}
			byID := map[string]record.Task{}
			for _, candidate := range latest {
				byID[candidate.Task.ID] = candidate.Task
			}
			seen := map[string]bool{}
			id := p.Task.ID
			for id != "" {
				if id == d.Task.ID {
					return ValidationError{"parent would create a cycle"}
				}
				if seen[id] {
					return ValidationError{fmt.Sprintf("task parent cycle detected at %s", id)}
				}
				seen[id] = true
				next, ok := byID[id]
				if !ok || next.Parent == nil {
					break
				}
				id = *next.Parent
			}
			id = p.Task.ID
			return d.SetParent(&id, a.Now().UTC())
		}
	}, func(task record.Task, raw []byte) { a.writeGeneratedTaskRecord(task.ID, raw) })
	return t, err == nil, err
}

func (a App) Ref(ctx context.Context, ref, action, typ, value string) (record.Task, bool, error) {
	if typ != "jira" && typ != "pr" && typ != "url" {
		return record.Task{}, false, ValidationError{fmt.Sprintf("invalid ref type %q", typ)}
	}
	if typ == "pr" && !prPattern.MatchString(value) {
		return record.Task{}, false, ValidationError{fmt.Sprintf("invalid GitHub PR reference %q (expected owner/repo#N)", value)}
	}
	current, err := a.Info(ref)
	if err != nil {
		return record.Task{}, false, err
	}
	found := false
	for _, r := range current.Refs {
		if r.Type == typ && r.Value == value {
			found = true
			break
		}
	}
	if action == "add" && found {
		return current, false, nil
	}
	if action == "rm" && !found {
		return record.Task{}, false, hub.NotFoundError{Ref: typ + ": " + value}
	}
	author, machine, err := a.author()
	if err != nil {
		return record.Task{}, false, err
	}
	summary := "ref added"
	if action == "rm" {
		summary = "ref removed"
	}
	t, err := a.Hub.UpdateWithPostCommit(ctx, current.ID, summary, author, machine, func(d *record.Document) error {
		refs := append([]record.Ref(nil), d.Task.Refs...)
		found := -1
		for i, r := range refs {
			if r.Type == typ && r.Value == value {
				found = i
				break
			}
		}
		if action == "add" {
			if found < 0 {
				refs = append(refs, record.Ref{Type: typ, Value: value})
			}
		} else if found >= 0 {
			refs = append(refs[:found], refs[found+1:]...)
		}
		return d.SetRefs(refs, a.Now().UTC())
	}, func(task record.Task, raw []byte) { a.writeGeneratedTaskRecord(task.ID, raw) })
	return t, err == nil, err
}

func (a App) Labels(ctx context.Context, ref, action string, values []string) (record.Task, bool, error) {
	for _, v := range values {
		if !labelPattern.MatchString(v) {
			return record.Task{}, false, ValidationError{fmt.Sprintf("invalid label %q", v)}
		}
	}
	current, err := a.Info(ref)
	if err != nil {
		return record.Task{}, false, err
	}
	set := map[string]bool{}
	for _, v := range current.Labels {
		set[v] = true
	}
	changed := false
	for _, v := range values {
		if action == "add" && !set[v] {
			changed = true
		}
		if action != "add" && set[v] {
			changed = true
		}
	}
	if !changed {
		return current, false, nil
	}
	author, machine, err := a.author()
	if err != nil {
		return record.Task{}, false, err
	}
	summary := "labels added"
	if action == "rm" {
		summary = "labels removed"
	}
	t, err := a.Hub.UpdateWithPostCommit(ctx, current.ID, summary, author, machine, func(d *record.Document) error {
		set := map[string]bool{}
		for _, v := range d.Task.Labels {
			set[v] = true
		}
		for _, v := range values {
			if action == "add" {
				set[v] = true
			} else {
				delete(set, v)
			}
		}
		labels := make([]string, 0, len(set))
		for v := range set {
			labels = append(labels, v)
		}
		sort.Strings(labels)
		return d.SetLabels(labels, a.Now().UTC())
	}, func(task record.Task, raw []byte) { a.writeGeneratedTaskRecord(task.ID, raw) })
	return t, err == nil, err
}

func RefURL(r record.Ref, jiraBase string) string {
	switch r.Type {
	case "jira":
		if jiraBase != "" {
			return strings.TrimRight(jiraBase, "/") + "/browse/" + r.Value
		}
	case "pr":
		if prPattern.MatchString(r.Value) {
			p := strings.SplitN(r.Value, "#", 2)
			return "https://github.com/" + p[0] + "/pull/" + p[1]
		}
	case "url":
		return r.Value
	}
	return ""
}

type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author"`
	Subject   string    `json:"subject"`
}

func (a App) History(ctx context.Context, ref string) ([]HistoryEntry, error) {
	t, err := a.Info(ref)
	if err != nil {
		return nil, err
	}
	out, _, err := a.Git.Run(ctx, a.Home.GitPath(), nil, "log", "--format=%aI%x00%an%x00%s", "--", "tasks/"+t.ID+".md")
	if err != nil {
		return nil, err
	}
	var history []HistoryEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		p := strings.Split(line, "\x00")
		if len(p) != 3 {
			continue
		}
		ts, e := time.Parse(time.RFC3339, p[0])
		if e != nil {
			return nil, e
		}
		history = append(history, HistoryEntry{Timestamp: ts.UTC(), Author: p[1], Subject: p[2]})
	}
	return history, nil
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
	Keep    bool
	Force   bool
	Cascade bool
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
	t, err := a.Hub.UpdateWithPostCommit(ctx, ref, fmt.Sprintf("status %s → %s", old.Status, status), c.Author, c.Machine, func(d *record.Document) error { return d.SetStatus(status, a.Now().UTC()) }, func(task record.Task, raw []byte) {
		a.writeGeneratedTaskRecord(task.ID, raw)
	})
	if err == nil && !record.IsOpen(status) {
		a.clearFocus(t.ID)
	}
	return t, err
}

func (a App) Mark(ctx context.Context, ref, status string, o MarkOptions) (MarkResult, error) {
	result := MarkResult{}
	if err := record.ValidateStatus(status); err != nil {
		return result, err
	}
	if o.Cascade && !record.IsOpen(status) {
		return a.markCascade(ctx, ref, status, o)
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
		if err == nil {
			a.pushCloseRemote(ctx, task.ID)
		}
		return result, err
	}
	if st, statErr := os.Stat(checkout.Path); statErr != nil || !st.IsDir() {
		result.Task, err = a.updateStatus(ctx, task.ID, status)
		if err == nil {
			a.pushCloseRemote(ctx, task.ID)
		}
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
	a.pushCloseRemote(ctx, task.ID)
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

// markCascade does every destructive-work preflight before changing any record.
// Descendants are closed depth-first so each parent passes the ordinary ST1 gate.
func (a App) markCascade(ctx context.Context, ref, status string, o MarkOptions) (MarkResult, error) {
	target, err := a.Info(ref)
	if err != nil {
		return MarkResult{}, err
	}
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return MarkResult{}, err
	}
	children := map[string][]record.Task{}
	for _, d := range docs {
		if d.Task.Parent != nil {
			children[*d.Task.Parent] = append(children[*d.Task.Parent], d.Task)
		}
	}
	for id := range children {
		sort.SliceStable(children[id], func(i, j int) bool { return children[id][i].CreatedAt.Before(children[id][j].CreatedAt) })
	}
	ordered := []record.Task{}
	seen := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(record.Task) error
	visit = func(t record.Task) error {
		if visiting[t.ID] {
			return fmt.Errorf("task parent cycle detected at %s", t.Handle)
		}
		if seen[t.ID] {
			return nil
		}
		visiting[t.ID] = true
		for _, c := range children[t.ID] {
			if err := visit(c); err != nil {
				return err
			}
		}
		delete(visiting, t.ID)
		seen[t.ID] = true
		if record.IsOpen(t.Status) {
			ordered = append(ordered, t)
		}
		return nil
	}
	if err := visit(target); err != nil {
		return MarkResult{}, err
	}
	local, err := a.Home.LoadLocal()
	if err != nil {
		return MarkResult{}, err
	}
	refusals := []string{}
	for _, t := range ordered {
		if o.Keep {
			continue
		}
		checkout, ok := local.Checkouts[t.ID]
		if !ok {
			continue
		}
		if st, statErr := os.Stat(checkout.Path); statErr != nil || !st.IsDir() {
			continue
		}
		loss, lossErr := a.tasktreeLoss(ctx, t.ID, checkout.Path)
		if lossErr != nil {
			refusals = append(refusals, fmt.Sprintf("%s: %v", t.Handle, lossErr))
			continue
		}
		if len(loss) > 0 && !o.Force {
			for _, v := range loss {
				refusals = append(refusals, t.Handle+": "+v)
			}
		}
	}
	if len(refusals) > 0 {
		return MarkResult{}, GateError{strings.Join(refusals, "\n")}
	}
	result := MarkResult{Task: target}
	one := o
	one.Cascade = false
	closed := []string{}
	for i, t := range ordered {
		oneResult, markErr := a.Mark(ctx, t.ID, status, one)
		for _, loss := range oneResult.Loss {
			result.Loss = append(result.Loss, t.Handle+": "+loss)
		}
		if oneResult.RemovedAt != "" {
			result.RemovedAt = oneResult.RemovedAt
		}
		if markErr != nil {
			for _, pending := range ordered[i:] {
				result.Remaining = append(result.Remaining, pending.Handle)
			}
			return result, fmt.Errorf("cascade stopped at %s: %w; already closed: %s; remaining: %s", t.Handle, markErr, strings.Join(closed, ", "), strings.Join(result.Remaining, ", "))
		}
		closed = append(closed, t.Handle)
		if t.ID == target.ID {
			result.Task = oneResult.Task
		}
	}
	return result, nil
}
