package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
}

func New(h home.Home, verboseWriter interface{ Write([]byte) (int, error) }) App {
	g := gitx.NewClient()
	if verboseWriter != nil {
		g.SetVerboseWriter(verboseWriter)
	}
	return App{Home: h, Hub: hub.New(h.GitPath(), g), Git: g, Now: time.Now}
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
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return record.Task{}, err
	}
	d, err := hub.Resolve(docs, ref)
	if err != nil {
		return record.Task{}, err
	}
	return d.Task, nil
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
	return a.Hub.Update(ctx, ref, fmt.Sprintf("%s added", kind), author, machine, func(d *record.Document) error {
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
	})
}
func (a App) Mark(ctx context.Context, ref, status string) (record.Task, error) {
	if err := record.ValidateStatus(status); err != nil {
		return record.Task{}, err
	}
	c, err := a.Home.LoadConfig()
	if err != nil {
		return record.Task{}, err
	}
	old, err := a.Info(ref)
	if err != nil {
		return record.Task{}, err
	}
	return a.Hub.Update(ctx, ref, fmt.Sprintf("status %s → %s", old.Status, status), c.Author, c.Machine, func(d *record.Document) error { return d.SetStatus(status, a.Now().UTC()) })
}
