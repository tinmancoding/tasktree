package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"github.com/tinmancoding/tasktree/v2/internal/repoalias"
	tpl "github.com/tinmancoding/tasktree/v2/internal/template"
	"github.com/tinmancoding/tasktree/v2/internal/variable"
)

type TemplateApplyOptions struct {
	Name, Title, Parent string
	Vars                []string
}
type RenderedTemplate struct {
	Spec   tpl.Spec
	Values map[string]string
}

func (a App) TemplateStore() tpl.Store { return tpl.New(filepath.Join(a.Home.Root, "templates")) }
func renderTemplate(s tpl.Spec, values map[string]string) tpl.Spec {
	r := variable.Render
	rout := s
	rout.Task.Title = r(s.Task.Title, values)
	rout.Task.Handle = r(s.Task.Handle, values)
	rout.Task.Jira = r(s.Task.Jira, values)
	rout.Task.Status = r(s.Task.Status, values)
	rout.Task.Description = r(s.Task.Description, values)
	for i, v := range s.Task.Labels {
		rout.Task.Labels[i] = r(v, values)
	}
	if s.Tasktree != nil {
		x := *s.Tasktree
		x.Repos = append([]tpl.Repo(nil), x.Repos...)
		for i := range x.Repos {
			x.Repos[i].Repo = r(x.Repos[i].Repo, values)
			x.Repos[i].Name = r(x.Repos[i].Name, values)
			x.Repos[i].Branch = r(x.Repos[i].Branch, values)
			x.Repos[i].From = r(x.Repos[i].From, values)
			x.Repos[i].Mode = r(x.Repos[i].Mode, values)
		}
		x.Files = append([]tpl.File(nil), x.Files...)
		for i := range x.Files {
			x.Files[i].Path = r(x.Files[i].Path, values)
			x.Files[i].Content = r(x.Files[i].Content, values)
			x.Files[i].Mode = r(x.Files[i].Mode, values)
		}
		x.Symlinks = append([]tpl.Symlink(nil), x.Symlinks...)
		for i := range x.Symlinks {
			x.Symlinks[i].Path = r(x.Symlinks[i].Path, values)
			x.Symlinks[i].Target = r(x.Symlinks[i].Target, values)
		}
		x.Bootstrap = append([]bootstrap.Step(nil), x.Bootstrap...)
		for i := range x.Bootstrap {
			x.Bootstrap[i].Name = r(x.Bootstrap[i].Name, values)
			x.Bootstrap[i].Run = r(x.Bootstrap[i].Run, values)
			x.Bootstrap[i].Workdir = r(x.Bootstrap[i].Workdir, values)
			env := map[string]string{}
			for k, v := range x.Bootstrap[i].Env {
				env[r(k, values)] = r(v, values)
			}
			x.Bootstrap[i].Env = env
		}
		rout.Tasktree = &x
	}
	return rout
}
func validTemplatePath(p string) bool {
	if p == "" || filepath.IsAbs(p) || filepath.Clean(p) != p {
		return false
	}
	for _, x := range strings.Split(filepath.ToSlash(p), "/") {
		if x == ".." || x == "" {
			return false
		}
	}
	return p != ".git" && !strings.HasPrefix(p, ".git/") && p != ".tasktree" && !strings.HasPrefix(p, ".tasktree/")
}
func (a App) RenderTemplate(name string, args []string) (RenderedTemplate, error) {
	s, e := a.TemplateStore().Load(name)
	if e != nil {
		return RenderedTemplate{}, ValidationError{e.Error()}
	}
	if e = tpl.Validate(s); e != nil {
		return RenderedTemplate{}, ValidationError{e.Error()}
	}
	cli, e := variable.ParseKVArgs(args)
	if e != nil {
		return RenderedTemplate{}, ValidationError{e.Error()}
	}
	decl := map[string]bool{}
	defaults := map[string]string{}
	required := map[string]bool{}
	for _, p := range s.Parameters {
		decl[p.Name] = true
		if p.Default != "" {
			defaults[p.Name] = p.Default
		}
		required[p.Name] = p.Required
	}
	for k := range cli {
		if !decl[k] {
			return RenderedTemplate{}, ValidationError{fmt.Sprintf("variable %q is not declared in parameters", k)}
		}
	}
	refs := tpl.Refs(s)
	for _, p := range s.Parameters {
		refs = append(refs, variable.Ref{Name: p.Name})
	}
	values := variable.Resolve(refs, cli, defaults)
	missing := []string{}
	for n, req := range required {
		if req {
			if _, ok := values[n]; !ok {
				missing = append(missing, n)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return RenderedTemplate{}, ValidationError{"unresolved required parameters: " + strings.Join(missing, ", ")}
	}
	rendered := renderTemplate(s, values)
	for _, x := range tpl.Refs(rendered) {
		return RenderedTemplate{}, ValidationError{fmt.Sprintf("unresolved variable %q", x.Name)}
	}
	if rendered.Task.Status == "" {
		rendered.Task.Status = "incoming"
	}
	if e = record.ValidateStatus(rendered.Task.Status); e != nil {
		return RenderedTemplate{}, ValidationError{e.Error()}
	}
	if rendered.Task.Handle != "" && !handlePattern.MatchString(rendered.Task.Handle) {
		return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid handle %q", rendered.Task.Handle)}
	}
	if e = record.ValidateDescription(rendered.Task.Description); e != nil {
		return RenderedTemplate{}, ValidationError{e.Error()}
	}
	for _, label := range rendered.Task.Labels {
		if !labelPattern.MatchString(label) {
			return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid label %q", label)}
		}
	}
	if rendered.Tasktree != nil {
		names := map[string]bool{}
		paths := map[string]bool{}
		for _, r := range rendered.Tasktree.Repos {
			name := r.Name
			if name == "" {
				name = deriveName(r.Repo)
			}
			if !validRepoName(name) || names[name] {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid or duplicate repository name %q", name)}
			}
			names[name] = true
			paths[name] = true
		}
		collides := func(path string) bool {
			if paths[path] {
				return true
			}
			for name := range names {
				if strings.HasPrefix(filepath.ToSlash(path), name+"/") {
					return true
				}
			}
			return false
		}
		for _, f := range rendered.Tasktree.Files {
			if !validTemplatePath(f.Path) {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid template file path %q", f.Path)}
			}
			if collides(f.Path) {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("template path %q collides with another entry", f.Path)}
			}
			paths[f.Path] = true
			if f.Mode != "" && f.Mode != "0644" && f.Mode != "0755" {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid file mode %q", f.Mode)}
			}
		}
		for _, l := range rendered.Tasktree.Symlinks {
			if !validTemplatePath(l.Path) {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid template symlink path %q", l.Path)}
			}
			if collides(l.Path) {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("duplicate template path %q", l.Path)}
			}
			paths[l.Path] = true
		}
		steps := map[string]bool{}
		for _, step := range rendered.Tasktree.Bootstrap {
			if step.Name == "" || step.Run == "" {
				return RenderedTemplate{}, ValidationError{"bootstrap steps require name and run"}
			}
			if steps[step.Name] {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("duplicate bootstrap step %q", step.Name)}
			}
			steps[step.Name] = true
			if step.Workdir != "" && !validTemplatePath(step.Workdir) {
				return RenderedTemplate{}, ValidationError{fmt.Sprintf("invalid bootstrap workdir %q", step.Workdir)}
			}
		}
	}
	return RenderedTemplate{rendered, values}, nil
}
func (a App) NewFromTemplate(ctx context.Context, o TemplateApplyOptions) (record.Task, error) {
	r, e := a.RenderTemplate(o.Name, o.Vars)
	if e != nil {
		return record.Task{}, e
	}
	s := r.Spec
	if o.Title != "" {
		s.Task.Title = o.Title
	}
	if s.Task.Title == "" {
		return record.Task{}, ValidationError{"template task.title is required when no title argument is passed"}
	}
	if o.Parent != "" && s.Tasktree != nil {
		return record.Task{}, ValidationError{"--parent cannot be used with a template that defines tasktree"}
	}
	id, e := record.NewID()
	if e != nil {
		return record.Task{}, e
	}
	handle := s.Task.Handle
	if handle == "" {
		if s.Task.Jira != "" {
			handle = s.Task.Jira
		} else {
			handle = Slug(s.Task.Title)
		}
	}
	docs, e := a.Hub.ReadAll()
	if e != nil {
		return record.Task{}, e
	}
	for _, d := range docs {
		if d.Task.Handle == handle {
			return record.Task{}, hub.ConflictError{Message: fmt.Sprintf("handle %q already exists", handle)}
		}
	}
	var parent *string
	if o.Parent != "" {
		p, x := hub.Resolve(docs, o.Parent)
		if x != nil {
			return record.Task{}, x
		}
		v := p.Task.ID
		parent = &v
	}
	now := a.Now().UTC()
	refs := []record.Ref{}
	if s.Task.Jira != "" {
		refs = append(refs, record.Ref{Type: "jira", Value: s.Task.Jira})
	}
	task := record.Task{ID: id, Handle: handle, Title: s.Task.Title, Status: s.Task.Status, Parent: parent, Labels: s.Task.Labels, Refs: refs, Description: s.Task.Description, CreatedAt: now, UpdatedAt: now}
	doc, e := record.New(task)
	if e != nil {
		return record.Task{}, e
	}
	entryID, e := record.NewID()
	if e != nil {
		return record.Task{}, e
	}
	author, machine, e := a.author()
	if e != nil {
		return record.Task{}, e
	}
	keys := make([]string, 0, len(r.Values))
	for k := range r.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var body strings.Builder
	fmt.Fprintf(&body, "Created from template %s", s.Metadata.Name)
	for _, k := range keys {
		fmt.Fprintf(&body, "\n%s=%s", k, r.Values[k])
	}
	if e = doc.Append(record.Entry{ID: entryID, Timestamp: now, Kind: "note", Author: author, Machine: machine, Body: body.String()}); e != nil {
		return record.Task{}, e
	}
	aliasURLs := []string{}
	if s.Tasktree != nil {
		resolved := make([]struct {
			m   module
			sha string
		}, 0, len(s.Tasktree.Repos))
		for _, repo := range s.Tasktree.Repos {
			m, sha, x := a.resolveDefinitionRepo(ctx, AddOptions{URL: repo.Repo, Name: repo.Name, Branch: repo.Branch, From: repo.From, Mode: repo.Mode})
			if x != nil {
				return record.Task{}, x
			}
			resolved = append(resolved, struct {
				m   module
				sha string
			}{m, sha})
			aliasURLs = append(aliasURLs, m.URL)
		}
		e = a.editDefinition(ctx, id, fmt.Sprintf("tasktree(%s): create from template %s", id, s.Metadata.Name), func(ms []module, f map[string]string) ([]module, map[string]string, error) {
			cfg := TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: id, Bootstrap: s.Tasktree.Bootstrap}
			b, _ := yamlMarshal(cfg)
			f[".tasktree/config.yml"] = string(b)
			for _, x := range resolved {
				ms = append(ms, x.m)
				f["gitlink:"+x.m.Path] = x.sha
			}
			for _, x := range s.Tasktree.Files {
				key := x.Path
				if x.Mode == "0755" {
					key = "100755:" + key
				}
				f[key] = x.Content
			}
			for _, x := range s.Tasktree.Symlinks {
				f["120000:"+x.Path] = x.Target
			}
			if _, ok := f["AGENTS.md"]; !ok {
				f["AGENTS.md"] = starterAgents(task)
			}
			return ms, f, nil
		})
		if e != nil {
			return record.Task{}, e
		}
	}
	// TASKTREE_TEST_RECORD_CREATE_FAIL is a test-only hook that verifies branch rollback.
	if os.Getenv("TASKTREE_TEST_RECORD_CREATE_FAIL") != "" {
		e = fmt.Errorf("test-only injected record creation failure")
	} else {
		cfg, _ := a.Home.LoadConfig()
		e = a.Hub.Create(ctx, doc, "create from template "+s.Metadata.Name, cfg.Author, cfg.Machine)
	}
	if e != nil {
		if s.Tasktree != nil {
			_ = a.Hub.WithLock(ctx, func() error {
				_, _, x := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "update-ref", "-d", "refs/heads/task/"+id)
				return x
			})
		}
		return record.Task{}, e
	}
	for _, url := range aliasURLs {
		result := AddResult{}
		registerAliases(repoalias.NewStore(a.Home.AliasesPath()), url, &result)
	}
	return doc.Task, nil
}
