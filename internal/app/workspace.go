package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/cache"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/repoalias"
	"gopkg.in/yaml.v3"
)

type GateError struct{ Message string }

func (e GateError) Error() string { return e.Message }

type TasktreeConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	TaskID     string `yaml:"taskId"`
}

func DiscoverRoot(start string) (string, TasktreeConfig, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", TasktreeConfig{}, err
		}
	}
	p, err := filepath.Abs(start)
	if err != nil {
		return "", TasktreeConfig{}, err
	}
	if st, statErr := os.Stat(p); statErr == nil && !st.IsDir() {
		p = filepath.Dir(p)
	}
	for {
		marker := filepath.Join(p, ".tasktree", "config.yml")
		if b, readErr := os.ReadFile(marker); readErr == nil {
			var c TasktreeConfig
			if err := yaml.Unmarshal(b, &c); err != nil {
				return "", c, fmt.Errorf("parse %s: %w", marker, err)
			}
			if c.TaskID == "" {
				return "", c, fmt.Errorf("%s has no taskId", marker)
			}
			return p, c, nil
		} else if !os.IsNotExist(readErr) {
			return "", TasktreeConfig{}, readErr
		}
		next := filepath.Dir(p)
		if next == p {
			break
		}
		p = next
	}
	return "", TasktreeConfig{}, os.ErrNotExist
}

func rootEnv(h home.Home, id, root string) map[string]string {
	return map[string]string{"GIT_DIR": h.ClonePath(id), "GIT_WORK_TREE": root}
}
func rootGit(ctx context.Context, g gitx.Client, h home.Home, id, root string, args ...string) (string, string, error) {
	return g.Run(ctx, root, rootEnv(h, id, root), args...)
}
func git(ctx context.Context, g gitx.Client, dir string, args ...string) (string, string, error) {
	return g.Run(ctx, dir, nil, args...)
}

func (a App) ResolveTarget(explicit, start string) (string, string, error) {
	if explicit != "" {
		t, err := a.Info(explicit)
		if err != nil {
			return "", "", err
		}
		s, err := a.Home.LoadLocal()
		if err != nil {
			return "", "", err
		}
		if c, ok := s.Checkouts[t.ID]; ok {
			return t.ID, c.Path, nil
		}
		return t.ID, "", nil
	}
	root, cfg, err := DiscoverRoot(start)
	if err != nil {
		return "", "", err
	}
	return cfg.TaskID, root, nil
}

type RepoInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Mode     string `json:"mode"`
	Branch   string `json:"branch"`
	Head     string `json:"head"`
	Dirty    bool   `json:"dirty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Unpushed int    `json:"unpushed"`
}
type TasktreeInfo struct {
	Path      string     `json:"path"`
	RootDirty int        `json:"rootDirty"`
	Repos     []RepoInfo `json:"repos"`
}

type module struct{ Name, Path, URL, Branch, Mode string }

func modules(root string) ([]module, error) {
	p := filepath.Join(root, ".gitmodules")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return []module{}, nil
	}
	g := gitx.NewClient()
	out, _, err := g.Run(context.Background(), "", nil, "config", "-f", p, "--get-regexp", `^submodule\..*\.(path|url|branch|tasktree-mode)$`)
	if err != nil {
		return nil, err
	}
	by := map[string]*module{}
	order := []string{}
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 2 {
			continue
		}
		key := f[0]
		val := strings.Join(f[1:], " ")
		rest := strings.TrimPrefix(key, "submodule.")
		i := strings.LastIndex(rest, ".")
		if i < 0 {
			continue
		}
		name, k := rest[:i], rest[i+1:]
		m := by[name]
		if m == nil {
			m = &module{Name: name}
			by[name] = m
			order = append(order, name)
		}
		switch k {
		case "path":
			m.Path = val
		case "url":
			m.URL = val
		case "branch":
			m.Branch = val
		case "tasktree-mode":
			m.Mode = val
		}
	}
	outm := make([]module, 0, len(order))
	for _, n := range order {
		outm = append(outm, *by[n])
	}
	return outm, s.Err()
}

func (a App) InspectTasktree(ctx context.Context, id, root string) (TasktreeInfo, error) {
	g := gitx.NewClient()
	result := TasktreeInfo{Path: root, Repos: []RepoInfo{}}
	out, _, err := rootGit(ctx, g, a.Home, id, root, "status", "--porcelain")
	if err != nil {
		return result, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" && !(len(line) > 3 && strings.HasPrefix(line[3:], ".")) {
			result.RootDirty++
		}
	}
	ms, err := modules(root)
	if err != nil {
		return result, err
	}
	for _, m := range ms {
		p := filepath.Join(root, m.Path)
		branch, _ := g.CurrentBranch(ctx, p)
		sha, _, _ := git(ctx, g, p, "rev-parse", "--short=12", "HEAD")
		dirty, _ := g.IsDirty(ctx, p)
		ri := RepoInfo{Name: m.Name, Path: m.Path, Mode: m.Mode, Branch: branch, Head: strings.TrimSpace(sha), Dirty: dirty}
		if branch == "" {
			ri.Branch = "(detached)"
		} else {
			counts, _, e := git(ctx, g, p, "rev-list", "--left-right", "--count", "origin/"+branch+"..."+branch)
			if e == nil {
				fmt.Sscanf(counts, "%d %d", &ri.Behind, &ri.Ahead)
			}
		}
		un, _, e := git(ctx, g, p, "rev-list", "--count", "--branches", "--not", "--remotes=origin")
		if e == nil {
			fmt.Sscanf(un, "%d", &ri.Unpushed)
		}
		result.Repos = append(result.Repos, ri)
	}
	return result, nil
}

func ensureEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("destination %s exists and is not empty", path)
	}
	return nil
}

func (a App) Open(ctx context.Context, ref, requested string, progress interface{ Write([]byte) (int, error) }) (string, error) {
	t, err := a.Info(ref)
	if err != nil {
		return "", err
	}
	if !recordOpen(t.Status) {
		return "", GateError{fmt.Sprintf("task %s is %s; mark active before opening", t.Handle, t.Status)}
	}
	local, err := a.Home.LoadLocal()
	if err != nil {
		return "", err
	}
	if c, ok := local.Checkouts[t.ID]; ok {
		if st, e := os.Stat(c.Path); e == nil && st.IsDir() {
			return c.Path, nil
		}
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return "", err
	}
	path := requested
	if path == "" {
		path = filepath.Join(cfg.TasksRoot, t.Handle)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := ensureEmpty(path); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	_ = os.RemoveAll(a.Home.ClonePath(t.ID))
	_ = os.Remove(path)
	if err := os.MkdirAll(a.Home.ClonesPath(), 0o755); err != nil {
		return "", err
	}
	g := gitx.NewClient()
	branch := "task/" + t.ID
	_, _, existsErr := git(ctx, g, a.Home.GitPath(), "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	configureIdentity := func() {
		name := cfg.Author
		if name == "" {
			name = "tasktree"
		}
		machine := cfg.Machine
		if machine == "" {
			machine = "localhost"
		}
		_, _, _ = rootGit(ctx, g, a.Home, t.ID, path, "config", "user.name", name)
		_, _, _ = rootGit(ctx, g, a.Home, t.ID, path, "config", "user.email", name+"@"+machine)
	}
	if existsErr == nil {
		_, _, err = git(ctx, g, "", "clone", "--separate-git-dir="+a.Home.ClonePath(t.ID), "--branch", branch, a.Home.GitPath(), path)
		if err == nil {
			configureIdentity()
		}
	} else {
		_, _, err = git(ctx, g, "", "clone", "--no-checkout", "--separate-git-dir="+a.Home.ClonePath(t.ID), a.Home.GitPath(), path)
		if err == nil {
			configureIdentity()
		}
		if err == nil {
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "checkout", "--orphan", branch)
		}
		if err == nil {
			err = os.MkdirAll(filepath.Join(path, ".tasktree"), 0o755)
		}
		if err == nil {
			b, _ := yaml.Marshal(TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: t.ID})
			err = os.WriteFile(filepath.Join(path, ".tasktree", "config.yml"), b, 0o644)
		}
		if err == nil {
			err = os.WriteFile(filepath.Join(path, ".tasktree", ".gitignore"), []byte("task.md\n"), 0o644)
		}
		if err == nil {
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "add", "-A")
		}
		if err == nil {
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "commit", "-m", fmt.Sprintf("tasktree(%s): create", t.ID))
		}
		if err == nil {
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "push", "-u", "origin", branch)
		}
	}
	if err != nil {
		_ = os.RemoveAll(path)
		_ = os.RemoveAll(a.Home.ClonePath(t.ID))
		return "", err
	}
	_ = os.Remove(filepath.Join(path, ".git"))
	if err := a.materialize(ctx, t.ID, path, progress); err != nil {
		return "", err
	}
	local.Checkouts[t.ID] = home.Checkout{Path: path}
	if err := a.Home.SaveLocal(local); err != nil {
		return "", err
	}
	return path, nil
}
func recordOpen(s string) bool { return s != "done" && s != "dropped" }

func cacheManager(g gitx.Client) (cache.Manager, error) {
	root, err := cache.DefaultRoot()
	if err != nil {
		return cache.Manager{}, err
	}
	return cache.NewManager(root, g), nil
}
func (a App) materialize(ctx context.Context, id, root string, progress interface{ Write([]byte) (int, error) }) error {
	ms, err := modules(root)
	if err != nil {
		return err
	}
	g := gitx.NewClient()
	mgr, e := cacheManager(g)
	if e != nil {
		return e
	}
	for _, m := range ms {
		cp, e := mgr.Ensure(ctx, m.URL)
		if e != nil {
			cp = mgr.PathForURL(m.URL)
			if _, st := os.Stat(cp); st != nil {
				return e
			}
			if progress != nil {
				fmt.Fprintf(progress, "warning: fetch %s failed; using cache\n", m.Name)
			}
		}
		rootGit(ctx, g, a.Home, id, root, "config", "submodule."+m.Name+".url", cp)
		_, _, e = rootGit(ctx, g, a.Home, id, root, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--", m.Path)
		rootGit(ctx, g, a.Home, id, root, "config", "submodule."+m.Name+".url", m.URL)
		if e != nil {
			return e
		}
		p := filepath.Join(root, m.Path)
		g.RemoteSetURL(ctx, p, "origin", m.URL)
		if err := rewriteSubmodule(a.Home, id, root, m); err != nil {
			return err
		}
		if m.Mode == "pin" || m.Branch == "" {
			git(ctx, g, p, "checkout", "--detach")
			continue
		}
		exists, _ := g.BranchExists(ctx, p, m.Branch)
		if exists {
			if _, _, e = git(ctx, g, p, "checkout", m.Branch); e != nil {
				return e
			}
		} else {
			remote := "origin/" + m.Branch
			if _, _, re := git(ctx, g, p, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote); re == nil {
				_, _, e = git(ctx, g, p, "checkout", "-b", m.Branch, "--track", remote)
			} else {
				_, _, e = git(ctx, g, p, "checkout", "-b", m.Branch)
			}
			if e != nil {
				return e
			}
		}
	}
	return nil
}
func rewriteSubmodule(h home.Home, id, root string, m module) error {
	gitdir := filepath.Join(h.ClonePath(id), "modules", m.Name)
	dot := filepath.Join(root, m.Path, ".git")
	if err := os.WriteFile(dot, []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		return err
	}
	g := gitx.NewClient()
	_, _, err := g.Run(context.Background(), "", nil, "--git-dir", gitdir, "config", "core.worktree", filepath.Join(root, m.Path))
	return err
}

type AddOptions struct{ URL, Branch, From, Name, Mode string }
type AddResult struct {
	Name, Mode, Branch string
	Messages           []string
}

func deriveName(url string) string {
	s := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
func (a App) Add(ctx context.Context, id, root string, o AddOptions) (AddResult, error) {
	r := AddResult{}
	store := repoalias.NewStore(a.Home.AliasesPath())
	if u, ok, e := store.Resolve(o.URL); e != nil {
		return r, e
	} else if ok {
		o.URL = u
	}
	if o.Name == "" {
		o.Name = deriveName(o.URL)
	}
	r.Name = o.Name
	if o.Name == "" || strings.ContainsAny(o.Name, "/\\") {
		return r, fmt.Errorf("invalid repository name %q", o.Name)
	}
	dest := filepath.Join(root, o.Name)
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		return r, fmt.Errorf("destination %s exists", dest)
	}
	g := gitx.NewClient()
	mgr, e := cacheManager(g)
	if e != nil {
		return r, e
	}
	cp, e := mgr.Ensure(ctx, o.URL)
	if e != nil {
		return r, e
	}
	if _, _, e = git(ctx, g, "", "clone", cp, dest); e != nil {
		return r, e
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(dest)
		}
	}()
	if e = g.RemoteSetURL(ctx, dest, "origin", o.URL); e != nil {
		return r, e
	}
	def, e := g.DefaultBranch(ctx, dest)
	if e != nil {
		return r, e
	}
	branch, pathKind := o.Branch, ""
	if branch != "" {
		local, _ := g.BranchExists(ctx, dest, branch)
		if local {
			e = g.Checkout(ctx, dest, branch)
			pathKind = "local"
		} else if _, _, x := git(ctx, g, dest, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch); x == nil {
			e = g.CreateTrackingBranch(ctx, dest, branch, "origin/"+branch)
			pathKind = "remote"
		} else {
			base := o.From
			if base == "" {
				base = def
			}
			if _, x := g.ResolveCommit(ctx, dest, base); x != nil {
				return r, x
			}
			e = g.Checkout(ctx, dest, base)
			if e == nil {
				e = g.CreateBranch(ctx, dest, branch)
			}
			pathKind = "created"
		}
	} else if o.From != "" {
		if _, x := g.ResolveCommit(ctx, dest, o.From); x == nil {
			e = g.Checkout(ctx, dest, o.From)
			pathKind = "headless"
		} else if _, _, x := git(ctx, g, dest, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+o.From); x == nil {
			e = g.CreateTrackingBranch(ctx, dest, o.From, "origin/"+o.From)
			branch = o.From
			pathKind = "remote"
		} else {
			return r, x
		}
	} else {
		branch = def
		e = g.Checkout(ctx, dest, def)
		pathKind = "default"
	}
	if e != nil {
		return r, e
	}
	mode := o.Mode
	if mode == "" {
		if o.Branch != "" {
			mode = "work"
		} else if o.From != "" && pathKind == "headless" {
			mode = "pin"
		} else {
			mode = "follow"
		}
	}
	if mode != "follow" && mode != "work" && mode != "pin" {
		return r, fmt.Errorf("invalid mode %q", mode)
	}
	_, _, e = rootGit(ctx, g, a.Home, id, root, "-c", "protocol.file.allow=always", "submodule", "add", "--force", o.URL, o.Name)
	if e != nil {
		return r, e
	}
	_, _, e = rootGit(ctx, g, a.Home, id, root, "submodule", "absorbgitdirs", o.Name)
	if e != nil {
		return r, e
	}
	m := module{Name: o.Name, Path: o.Name, URL: o.URL, Branch: branch, Mode: mode}
	if e = rewriteSubmodule(a.Home, id, root, m); e != nil {
		return r, e
	}
	gm := filepath.Join(root, ".gitmodules")
	if branch != "" {
		git(ctx, g, "", "config", "-f", gm, "submodule."+o.Name+".branch", branch)
	}
	git(ctx, g, "", "config", "-f", gm, "submodule."+o.Name+".tasktree-mode", mode)
	registerAliases(store, o.URL, &r)
	if e = a.commitRoot(ctx, id, root, fmt.Sprintf("tasktree(%s): add %s", id, o.Name)); e != nil {
		return r, e
	}
	cleanup = false
	r.Mode = mode
	r.Branch = branch
	r.Messages = append(r.Messages, "branch resolution: "+pathKind)
	return r, nil
}
func registerAliases(s repoalias.Store, url string, r *AddResult) {
	f, e := s.Load()
	if e != nil {
		return
	}
	base := deriveName(url)
	owner := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	parts := strings.FieldsFunc(owner, func(x rune) bool { return x == '/' || x == ':' })
	aliases := []string{base}
	if len(parts) > 1 {
		aliases = append(aliases, parts[len(parts)-2]+"-"+base)
	}
	for _, alias := range aliases {
		conflict := ""
		idx := -1
		for i, x := range f.Repos {
			if x.URL == url {
				idx = i
			}
			for _, a := range x.Aliases {
				if a == alias && x.URL != url {
					conflict = x.URL
				}
			}
		}
		if conflict != "" {
			r.Messages = append(r.Messages, fmt.Sprintf("alias %s conflicts with %s", alias, conflict))
			continue
		}
		if idx < 0 {
			f.Repos = append(f.Repos, repoalias.Repo{URL: url})
			idx = len(f.Repos) - 1
		}
		found := false
		for _, a := range f.Repos[idx].Aliases {
			found = found || a == alias
		}
		if !found {
			f.Repos[idx].Aliases = append(f.Repos[idx].Aliases, alias)
			r.Messages = append(r.Messages, "registered alias "+alias)
		}
	}
	_ = s.Save(f)
}
func (a App) commitRoot(ctx context.Context, id, root, msg string) error {
	g := gitx.NewClient()
	if _, _, e := rootGit(ctx, g, a.Home, id, root, "add", "-A"); e != nil {
		return e
	}
	if _, _, e := rootGit(ctx, g, a.Home, id, root, "commit", "-m", msg); e != nil {
		return e
	}
	_, _, e := rootGit(ctx, g, a.Home, id, root, "push", "origin", "task/"+id)
	return e
}

func (a App) Commit(ctx context.Context, id, root, msg string) (bool, []string, error) {
	g := gitx.NewClient()
	warnings := []string{}
	ms, e := modules(root)
	if e != nil {
		return false, nil, e
	}
	for _, m := range ms {
		dirty, _ := g.IsDirty(ctx, filepath.Join(root, m.Path))
		if dirty {
			warnings = append(warnings, m.Name+" has uncommitted changes, not recorded")
		}
	}
	if _, _, e = rootGit(ctx, g, a.Home, id, root, "add", "-A"); e != nil {
		return false, warnings, e
	}
	_, _, e = rootGit(ctx, g, a.Home, id, root, "diff", "--cached", "--quiet")
	if e == nil {
		return false, warnings, nil
	}
	if msg == "" {
		msg = fmt.Sprintf("tasktree(%s): checkpoint", id)
	}
	if _, _, e = rootGit(ctx, g, a.Home, id, root, "commit", "-m", msg); e != nil {
		return false, warnings, e
	}
	_, _, e = rootGit(ctx, g, a.Home, id, root, "push", "origin", "task/"+id)
	return true, warnings, e
}

func (a App) Remove(ctx context.Context, id, root, name string, force bool) ([]string, error) {
	ms, e := modules(root)
	if e != nil {
		return nil, e
	}
	var m *module
	for i := range ms {
		if ms[i].Name == name {
			m = &ms[i]
		}
	}
	if m == nil {
		return nil, fmt.Errorf("repository %q not found", name)
	}
	p := filepath.Join(root, m.Path)
	g := gitx.NewClient()
	loss := []string{}
	if dirty, _ := g.IsDirty(ctx, p); dirty {
		loss = append(loss, name+" has uncommitted changes")
	}
	out, _, _ := git(ctx, g, p, "rev-list", "--oneline", "--branches", "--not", "--remotes=origin")
	if strings.TrimSpace(out) != "" {
		loss = append(loss, name+" has unpushed commits:\n"+strings.TrimSpace(out))
	}
	if len(loss) > 0 && !force {
		return loss, GateError{strings.Join(loss, "\n")}
	}
	rootGit(ctx, g, a.Home, id, root, "submodule", "deinit", "-f", "--", m.Path)
	if _, _, e = rootGit(ctx, g, a.Home, id, root, "rm", "-f", "--", m.Path); e != nil {
		return loss, e
	}
	_ = os.RemoveAll(filepath.Join(a.Home.ClonePath(id), "modules", m.Name))
	e = a.commitRoot(ctx, id, root, fmt.Sprintf("tasktree(%s): remove %s", id, name))
	return loss, e
}

func SortedRepos(in []RepoInfo) {
	sort.Slice(in, func(i, j int) bool { return in[i].Name < in[j].Name })
}

var _ = errors.Is
