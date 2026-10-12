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

	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
	"github.com/tinmancoding/tasktree/v2/internal/cache"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"github.com/tinmancoding/tasktree/v2/internal/repoalias"
	"gopkg.in/yaml.v3"
)

type GateError struct{ Message string }

func (e GateError) Error() string { return e.Message }

type TasktreeConfig struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	TaskID     string           `yaml:"taskId"`
	Bootstrap  []bootstrap.Step `yaml:"bootstrap,omitempty"`
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
			// Only a file declaring kind TasktreeConfig marks a tasktree root.
			// ~/.tasktree/config.yml is the machine config (TASKTREE_HOME) and
			// must not be mistaken for a marker, so non-markers are skipped.
			if err := yaml.Unmarshal(b, &c); err == nil && c.Kind == "TasktreeConfig" {
				if c.TaskID == "" {
					return "", c, fmt.Errorf("%s has no taskId", marker)
				}
				return p, c, nil
			}
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
	ref := explicit
	source := ""
	if ref == "" {
		ref = os.Getenv("TASKTREE_TASK")
		source = "TASKTREE_TASK"
	}
	if ref != "" {
		t, err := a.Info(ref)
		if err != nil {
			if source != "" {
				return "", "", fmt.Errorf("%s=%q: %w", source, ref, err)
			}
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
	local, err := a.Home.LoadLocal()
	if err != nil {
		return "", "", err
	}
	if checkout, ok := local.Checkouts[cfg.TaskID]; ok && checkout.Focus != "" {
		if _, err := a.Info(checkout.Focus); err == nil {
			return checkout.Focus, root, nil
		}
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
		// git config exits 1 when no key matches, e.g. an empty .gitmodules
		// left behind after removing the last submodule.
		var ce gitx.CommandError
		if errors.As(err, &ce) && ce.ExitCode == 1 {
			return []module{}, nil
		}
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
	g := a.Git.WithDefaults()
	result := TasktreeInfo{Path: root, Repos: []RepoInfo{}}
	out, _, err := rootGit(ctx, g, a.Home, id, root, "status", "--porcelain")
	if err != nil {
		return result, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p := line
		if len(line) > 3 {
			p = line[3:]
		}
		if arrow := strings.LastIndex(p, " -> "); arrow >= 0 {
			p = p[arrow+4:]
		}
		if p != ".tasktree/task.md" {
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

type OpenOptions struct {
	Path                 string
	Force, SkipBootstrap bool
	Repos                []string
	Branch               string
}

func (a App) Open(ctx context.Context, ref string, o OpenOptions, progress interface{ Write([]byte) (int, error) }) (string, error) {
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
			a.refreshTaskRecord(ctx, t.ID)
			return c.Path, nil
		}
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return "", err
	}
	// A subtask without a task branch is promoted to an isolated tasktree.
	g := a.Git.WithDefaults()
	branch := "task/" + t.ID
	_, _, branchErr := git(ctx, g, a.Home.GitPath(), "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if t.Parent != nil && branchErr != nil {
		return a.openIsolated(ctx, t, o, progress)
	}
	path := o.Path
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
	clonePath := a.Home.ClonePath(t.ID)
	if _, statErr := os.Stat(clonePath); statErr == nil {
		msg := fmt.Sprintf("hidden git dir for %s already exists at %s; the checkout may have been moved or local.yml lost", t.Handle, clonePath)
		if !o.Force {
			return "", GateError{msg}
		}
		loss, lossErr := lossInModules(ctx, a.Git.WithDefaults(), clonePath)
		if lossErr != nil {
			loss = append(loss, lossErr.Error())
		}
		if progress != nil {
			for _, item := range loss {
				fmt.Fprintln(progress, item)
			}
		}
		if err := os.RemoveAll(clonePath); err != nil {
			return "", err
		}
	}
	_ = os.Remove(path)
	if err := os.MkdirAll(a.Home.ClonesPath(), 0o755); err != nil {
		return "", err
	}
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
			// checkout --orphan seeds the index and work tree from its start point.
			// Remove that seed so task branches never inherit records from main.
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "rm", "-rf", "--ignore-unmatch", ".")
		}
		if err == nil {
			_, _, err = rootGit(ctx, g, a.Home, t.ID, path, "clean", "-fdx")
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
			err = os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte(starterAgents(t)), 0o644)
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
		_ = os.RemoveAll(path)
		_ = os.RemoveAll(a.Home.ClonePath(t.ID))
		return "", err
	}
	if err := a.Home.UpdateLocal(func(current *home.LocalState) error {
		current.Checkouts[t.ID] = home.Checkout{Path: path}
		return nil
	}); err != nil {
		return "", err
	}
	a.refreshTaskRecord(ctx, t.ID)
	a.addParentRemotes(ctx, t, path)
	if !o.SkipBootstrap {
		if err := a.Bootstrap(ctx, t.ID, path, progress); err != nil {
			return "", err
		}
	}
	return path, nil
}

func starterAgents(t record.Task) string {
	return fmt.Sprintf("# %s (%s)\n\nThis folder is a tasktree for task `%s` (id `%s`), managed by `tt2`.\n\n## Rules for agents\n\n- The task record is the source of truth for status, notes, decisions, and references. Read it with `tt2 info` or `.tasktree/task.md` (a generated copy — never edit it).\n- Record progress with `tt2 note \"...\"`; use `--kind decision`, `--kind comment`, or `--kind ref` when it fits. Set `TASKTREE_AUTHOR` to your agent name.\n- Never run git in this folder's root; it is intentionally not a git repository. Record root files and repository commits with `tt2 commit -m \"...\"`.\n- Inside each repository folder, git works normally: branch, commit, push.\n- Add or remove repositories with `tt2 add <alias|url>` and `tt2 remove <name>`. Never `git clone` into this folder.\n- Change task status (`tt2 mark ...`) only when asked.\n\n## Notes\n", t.Title, t.Handle, t.Handle, t.ID)
}
func recordOpen(s string) bool { return s != "done" && s != "dropped" }

func markerTaskID(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, ".tasktree", "config.yml"))
	if err != nil {
		return "", err
	}
	var cfg TasktreeConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return "", err
	}
	return cfg.TaskID, nil
}

func (a App) teardownTasktree(id, root string, local home.LocalState) []string {
	if markerID, err := markerTaskID(root); err != nil || markerID != id {
		return []string{root + " (task marker missing or mismatched)"}
	}
	if err := os.RemoveAll(root); err != nil {
		return []string{root + " (" + err.Error() + ")"}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		return []string{root}
	}
	clone := a.Home.ClonePath(id)
	if err := os.RemoveAll(clone); err != nil {
		return []string{clone + " (" + err.Error() + ")"}
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		return []string{clone}
	}
	if err := a.Home.UpdateLocal(func(current *home.LocalState) error { delete(current.Checkouts, id); return nil }); err != nil {
		return []string{a.Home.LocalPath() + " entry (" + err.Error() + ")"}
	}
	return nil
}

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
	g := a.Git.WithDefaults()
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
		gitlink, _, ge := rootGit(ctx, g, a.Home, id, root, "rev-parse", "HEAD:"+m.Path)
		if ge != nil {
			return ge
		}
		gitlink = strings.TrimSpace(gitlink)
		g.RemoteSetURL(ctx, p, "origin", m.URL)
		if err := rewriteSubmodule(a.Home, id, root, m); err != nil {
			return err
		}
		if m.Mode == "pin" || m.Branch == "" {
			git(ctx, g, p, "checkout", "--detach")
			continue
		}
		// A cache clone may already have a same-named local branch ahead of the
		// root gitlink. A freshly materialized checkout must start at the gitlink.
		if _, _, e = git(ctx, g, p, "checkout", "-B", m.Branch, gitlink); e != nil {
			return e
		}
		remote := "origin/" + m.Branch
		if _, _, re := git(ctx, g, p, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote); re == nil {
			_, _, _ = git(ctx, g, p, "branch", "--set-upstream-to", remote, m.Branch)
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
	g := a.Git.WithDefaults()
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
	registered := false
	defer func() {
		if !cleanup {
			return
		}
		os.RemoveAll(dest)
		if registered {
			_, _, _ = rootGit(ctx, g, a.Home, id, root, "reset", "HEAD", "--", o.Name, ".gitmodules")
			if _, _, err := rootGit(ctx, g, a.Home, id, root, "cat-file", "-e", "HEAD:.gitmodules"); err == nil {
				_, _, _ = rootGit(ctx, g, a.Home, id, root, "restore", "--source=HEAD", "--worktree", "--", ".gitmodules")
			} else {
				_ = os.Remove(filepath.Join(root, ".gitmodules"))
			}
			_ = os.RemoveAll(filepath.Join(a.Home.ClonePath(id), "modules", o.Name))
		}
	}()
	if e = g.RemoteSetURL(ctx, dest, "origin", o.URL); e != nil {
		return r, e
	}
	def, e := g.DefaultBranch(ctx, dest)
	if e != nil {
		return r, e
	}
	branch, pathKind, effectiveFrom, ignoredFrom := o.Branch, "", "", ""
	if branch != "" {
		local, _ := g.BranchExists(ctx, dest, branch)
		if local {
			e = g.Checkout(ctx, dest, branch)
			pathKind = "local"
			ignoredFrom = o.From
		} else if _, _, x := git(ctx, g, dest, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch); x == nil {
			e = g.CreateTrackingBranch(ctx, dest, branch, "origin/"+branch)
			pathKind = "remote"
			ignoredFrom = o.From
		} else {
			base := o.From
			if base == "" {
				base = def
			}
			effectiveFrom = base
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
	registered = true
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
		// If commit succeeded but push failed, preserve the committed submodule.
		if _, _, committedErr := rootGit(ctx, g, a.Home, id, root, "cat-file", "-e", "HEAD:"+o.Name); committedErr == nil {
			cleanup = false
		}
		return r, e
	}
	cleanup = false
	r.Mode = mode
	r.Branch = branch
	var resolution string
	switch pathKind {
	case "local", "default":
		resolution = fmt.Sprintf("Using existing local branch %q", branch)
	case "remote":
		resolution = fmt.Sprintf("Using existing remote branch %q from origin", branch)
	case "created":
		resolution = fmt.Sprintf("Creating new branch %q from %q", branch, effectiveFrom)
	case "headless":
		resolution = fmt.Sprintf("Checking out %q without creating a branch", o.From)
	}
	if ignoredFrom != "" {
		resolution += fmt.Sprintf("; ignoring --from %q", ignoredFrom)
	}
	if resolution != "" {
		r.Messages = append(r.Messages, resolution+".")
	}
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
	g := a.Git.WithDefaults()
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
	g := a.Git.WithDefaults()
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
		_, _, e = rootGit(ctx, g, a.Home, id, root, "push", "origin", "task/"+id)
		return false, warnings, e
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

func repoLoss(ctx context.Context, g gitx.Client, name, path string, bare bool) ([]string, error) {
	loss := []string{}
	args := []string{}
	dir := path
	if bare {
		args, dir = append(args, "--git-dir", path), ""
	}
	run := func(a ...string) (string, error) {
		out, _, err := git(ctx, g, dir, append(args, a...)...)
		return out, err
	}
	if !bare {
		out, err := run("status", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("cannot check %s for loss: %w", name, err)
		}
		if strings.TrimSpace(out) != "" {
			loss = append(loss, name+" has uncommitted changes")
		}
	}
	out, err := run("rev-list", "--oneline", "HEAD", "--branches", "--tags", "--not", "--remotes=origin")
	if err != nil {
		return nil, fmt.Errorf("cannot check %s for loss: %w", name, err)
	}
	if strings.TrimSpace(out) != "" {
		loss = append(loss, name+" has unpushed commits:\n"+strings.TrimSpace(out))
	}
	if _, err = run("show-ref", "--verify", "--quiet", "refs/stash"); err == nil {
		loss = append(loss, name+" has a stash")
	}
	return loss, nil
}

func moduleGitDirs(clone string) ([]string, error) {
	base := filepath.Join(clone, "modules")
	var dirs []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if _, e1 := os.Stat(filepath.Join(path, "HEAD")); e1 == nil {
			if _, e2 := os.Stat(filepath.Join(path, "objects")); e2 == nil {
				dirs = append(dirs, path)
				return filepath.SkipDir
			}
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return dirs, err
}
func lossInModules(ctx context.Context, g gitx.Client, clone string) ([]string, error) {
	dirs, err := moduleGitDirs(clone)
	if err != nil {
		return nil, err
	}
	var all []string
	for _, d := range dirs {
		x, e := repoLoss(ctx, g, filepath.Base(d), d, true)
		if e != nil {
			return all, e
		}
		all = append(all, x...)
	}
	return all, nil
}
func (a App) tasktreeLoss(ctx context.Context, id, root string) ([]string, error) {
	ms, err := modules(root)
	if err != nil {
		return nil, err
	}
	g := a.Git.WithDefaults()
	loss := []string{}
	seen := map[string]bool{}
	for _, m := range ms {
		worktree := filepath.Join(root, m.Path)
		gitdir := filepath.Join(a.Home.ClonePath(id), "modules", m.Name)
		_, statErr := os.Stat(worktree)
		x, e := repoLoss(ctx, g, m.Name, worktree, false)
		if statErr != nil {
			x, e = repoLoss(ctx, g, m.Name, gitdir, true)
		}
		if e != nil {
			return loss, e
		}
		loss = append(loss, x...)
		seen[filepath.Clean(gitdir)] = true
	}
	dirs, e := moduleGitDirs(a.Home.ClonePath(id))
	if e != nil {
		return loss, e
	}
	for _, d := range dirs {
		if seen[filepath.Clean(d)] {
			continue
		}
		x, e := repoLoss(ctx, g, filepath.Base(d), d, true)
		if e != nil {
			return loss, e
		}
		loss = append(loss, x...)
	}
	ignored, _, e := rootGit(ctx, g, a.Home, id, root, "ls-files", "--others", "--ignored", "--exclude-standard")
	if e != nil {
		return loss, fmt.Errorf("cannot check root ignored files: %w", e)
	}
	for _, p := range strings.Split(strings.TrimSpace(ignored), "\n") {
		if p != "" && filepath.Base(p) != ".DS_Store" && p != ".tasktree/task.md" {
			loss = append(loss, "root has ignored untracked file "+p)
		}
	}
	registered := map[string]bool{}
	for _, m := range ms {
		registered[filepath.Clean(filepath.Join(root, m.Path))] = true
	}
	e = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && registered[filepath.Clean(p)] {
			return filepath.SkipDir
		}
		if p != root && d.Name() == ".git" {
			loss = append(loss, "root has unregistered nested git repository "+strings.TrimPrefix(p, root+string(filepath.Separator)))
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if e != nil {
		return loss, fmt.Errorf("cannot scan root for nested repositories: %w", e)
	}
	return loss, nil
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
	validRelative := func(v string) bool {
		if v == "" || filepath.IsAbs(v) {
			return false
		}
		for _, part := range strings.FieldsFunc(filepath.ToSlash(v), func(r rune) bool { return r == '/' }) {
			if part == ".." {
				return false
			}
		}
		return true
	}
	if !validRelative(m.Name) || !validRelative(m.Path) {
		return nil, fmt.Errorf("invalid submodule name/path %q/%q", m.Name, m.Path)
	}
	p := filepath.Join(root, m.Path)
	if rel, err := filepath.Rel(root, p); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid submodule checkout path %q", p)
	}
	g := a.Git.WithDefaults()
	loss, lossErr := repoLoss(ctx, g, name, p, false)
	if lossErr != nil && !force {
		return nil, GateError{lossErr.Error()}
	}
	if lossErr != nil {
		loss = append(loss, lossErr.Error())
	}
	if len(loss) > 0 && !force {
		return loss, GateError{strings.Join(loss, "\n")}
	}
	rootGit(ctx, g, a.Home, id, root, "submodule", "deinit", "-f", "--", m.Path)
	if _, _, e = rootGit(ctx, g, a.Home, id, root, "rm", "-f", "--", m.Path); e != nil {
		return loss, e
	}
	modulesBase := filepath.Join(a.Home.ClonePath(id), "modules")
	deletePath := filepath.Join(modulesBase, m.Name)
	rel, relErr := filepath.Rel(modulesBase, deletePath)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return loss, fmt.Errorf("invalid submodule modules path %q", deletePath)
	}
	_ = os.RemoveAll(deletePath)
	e = a.commitRoot(ctx, id, root, fmt.Sprintf("tasktree(%s): remove %s", id, name))
	return loss, e
}

func SortedRepos(in []RepoInfo) {
	sort.Slice(in, func(i, j int) bool { return in[i].Name < in[j].Name })
}

var _ = errors.Is

// Bootstrap executes the committed bootstrap plan for a materialized checkout.
func (a App) Bootstrap(ctx context.Context, id, root string, progress interface{ Write([]byte) (int, error) }) error {
	b, err := os.ReadFile(filepath.Join(root, ".tasktree", "config.yml"))
	if err != nil {
		return err
	}
	var cfg TasktreeConfig
	if err = yaml.Unmarshal(b, &cfg); err != nil {
		return err
	}
	return bootstrap.Run(ctx, root, cfg.Bootstrap, progress)
}

// Mode changes repository update intent and records the structural change.
func (a App) Mode(ctx context.Context, id, root, name, mode string) error {
	if mode != "follow" && mode != "work" && mode != "pin" {
		return fmt.Errorf("invalid mode %q", mode)
	}
	ms, err := modules(root)
	if err != nil {
		return err
	}
	var found *module
	for i := range ms {
		if ms[i].Name == name {
			found = &ms[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("repository %q not found", name)
	}
	g := a.Git.WithDefaults()
	gm := filepath.Join(root, ".gitmodules")
	if mode == "pin" {
		_, _, _ = git(ctx, g, "", "config", "-f", gm, "--unset", "submodule."+name+".branch")
	} else {
		branch, err := g.CurrentBranch(ctx, filepath.Join(root, found.Path))
		if err != nil {
			return err
		}
		if branch == "" {
			return fmt.Errorf("repository %s is detached; follow and work require a branch", name)
		}
		if _, _, err = git(ctx, g, "", "config", "-f", gm, "submodule."+name+".branch", branch); err != nil {
			return err
		}
	}
	if _, _, err = git(ctx, g, "", "config", "-f", gm, "submodule."+name+".tasktree-mode", mode); err != nil {
		return err
	}
	return a.commitRoot(ctx, id, root, fmt.Sprintf("tasktree(%s): mode %s %s", id, name, mode))
}

func (a App) Focus(root, ref string, clear bool) (record.Task, error) {
	_, cfg, err := DiscoverRoot(root)
	if err != nil {
		return record.Task{}, err
	}
	base, err := a.Info(cfg.TaskID)
	if err != nil {
		return record.Task{}, err
	}
	if clear {
		err = a.Home.UpdateLocal(func(s *home.LocalState) error {
			c := s.Checkouts[base.ID]
			c.Focus = ""
			s.Checkouts[base.ID] = c
			return nil
		})
		return base, err
	}
	t, err := a.Info(ref)
	if err != nil {
		return record.Task{}, err
	}
	docs, err := a.Hub.ReadAll()
	if err != nil {
		return record.Task{}, err
	}
	by := map[string]record.Task{}
	for _, d := range docs {
		by[d.Task.ID] = d.Task
	}
	p := t.Parent
	descendant := false
	for p != nil {
		if *p == base.ID {
			descendant = true
			break
		}
		x, ok := by[*p]
		if !ok {
			break
		}
		p = x.Parent
	}
	if !descendant {
		return record.Task{}, GateError{fmt.Sprintf("%s is not a descendant of %s", t.Handle, base.Handle)}
	}
	g := a.Git.WithDefaults()
	_, _, e := git(context.Background(), g, a.Home.GitPath(), "show-ref", "--verify", "--quiet", "refs/heads/task/"+t.ID)
	if e == nil {
		return record.Task{}, GateError{fmt.Sprintf("%s has its own isolated tasktree", t.Handle)}
	}
	err = a.Home.UpdateLocal(func(s *home.LocalState) error {
		c, ok := s.Checkouts[base.ID]
		if !ok {
			return fmt.Errorf("tasktree checkout is not recorded")
		}
		c.Focus = t.ID
		s.Checkouts[base.ID] = c
		return nil
	})
	return t, err
}

func (a App) FocusForRoot(root string) (record.Task, bool) {
	_, cfg, err := DiscoverRoot(root)
	if err != nil {
		return record.Task{}, false
	}
	s, err := a.Home.LoadLocal()
	if err != nil {
		return record.Task{}, false
	}
	c := s.Checkouts[cfg.TaskID]
	if c.Focus == "" {
		return record.Task{}, false
	}
	t, err := a.Info(c.Focus)
	return t, err == nil
}

func (a App) clearFocus(id string) {
	_ = a.Home.UpdateLocal(func(s *home.LocalState) error {
		for k, c := range s.Checkouts {
			if c.Focus == id {
				c.Focus = ""
				s.Checkouts[k] = c
			}
		}
		return nil
	})
}

func (a App) openIsolated(ctx context.Context, t record.Task, o OpenOptions, progress interface{ Write([]byte) (int, error) }) (path string, err error) {
	parent, err := a.Info(*t.Parent)
	if err != nil {
		return "", err
	}
	local, err := a.Home.LoadLocal()
	if err != nil {
		return "", err
	}
	pc, ok := local.Checkouts[parent.ID]
	if !ok {
		return "", GateError{"open " + parent.Handle + " first"}
	}
	if st, e := os.Stat(pc.Path); e != nil || !st.IsDir() {
		return "", GateError{"open " + parent.Handle + " first"}
	}
	pmods, err := modules(pc.Path)
	if err != nil {
		return "", err
	}
	selected := map[string]bool{}
	if len(o.Repos) > 0 {
		for _, n := range o.Repos {
			selected[n] = true
		}
	} else {
		for _, m := range pmods {
			if m.Mode == "work" {
				selected[m.Name] = true
			}
		}
	}
	for n := range selected {
		found := false
		for _, m := range pmods {
			found = found || m.Name == n
		}
		if !found {
			return "", fmt.Errorf("repository %q does not exist in parent", n)
		}
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return "", err
	}
	path = o.Path
	if path == "" {
		path = filepath.Join(cfg.TasksRoot, t.Handle)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err = ensureEmpty(path); err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	_ = os.Remove(path)
	if err = os.MkdirAll(a.Home.ClonesPath(), 0755); err != nil {
		return "", err
	}
	g := a.Git.WithDefaults()
	taskBranch := "task/" + t.ID
	defer func() {
		if err != nil {
			_ = os.RemoveAll(path)
			_ = os.RemoveAll(a.Home.ClonePath(t.ID))
		}
	}()
	if _, _, err = git(ctx, g, "", "clone", "--no-checkout", "--separate-git-dir="+a.Home.ClonePath(t.ID), a.Home.GitPath(), path); err != nil {
		return "", err
	}
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
	if _, _, err = rootGit(ctx, g, a.Home, t.ID, path, "checkout", "--orphan", taskBranch); err != nil {
		return "", err
	}
	_, _, _ = rootGit(ctx, g, a.Home, t.ID, path, "rm", "-rf", "--ignore-unmatch", ".")
	_, _, _ = rootGit(ctx, g, a.Home, t.ID, path, "clean", "-fdx")
	if err = os.MkdirAll(filepath.Join(path, ".tasktree"), 0755); err != nil {
		return "", err
	}
	b, _ := yaml.Marshal(TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: t.ID})
	if err = os.WriteFile(filepath.Join(path, ".tasktree", "config.yml"), b, 0644); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(path, ".tasktree", ".gitignore"), []byte("task.md\n"), 0644); err != nil {
		return "", err
	}
	agents := starterAgents(t)
	intro := fmt.Sprintf("This folder is a tasktree for task `%s` (id `%s`), managed by `tt2`.\n", t.Handle, t.ID)
	agents = strings.Replace(agents, intro, intro+fmt.Sprintf("\nParent task: `%s` — read it with `tt2 info -t %s`.\n", parent.Handle, parent.Handle), 1)
	if err = os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte(agents), 0644); err != nil {
		return "", err
	}
	mgr, e := cacheManager(g)
	if e != nil {
		return "", e
	}
	for _, m := range pmods {
		if !selected[m.Name] {
			continue
		}
		pp := filepath.Join(pc.Path, m.Path)
		head, _, e := git(ctx, g, pp, "rev-parse", "HEAD")
		if e != nil {
			return "", e
		}
		head = strings.TrimSpace(head)
		dirty, _ := g.IsDirty(ctx, pp)
		if dirty && progress != nil {
			fmt.Fprintf(progress, "warning: %s: parent has uncommitted changes; forking from its last commit\n", m.Name)
		}
		actual, _ := g.CurrentBranch(ctx, pp)
		if m.Branch != "" && actual != m.Branch && progress != nil {
			fmt.Fprintf(progress, "warning: %s: parent HEAD is on %s, not .gitmodules branch %s; forking from HEAD\n", m.Name, actual, m.Branch)
		}
		cp, e := mgr.Ensure(ctx, m.URL)
		if e != nil {
			return "", e
		}
		dest := filepath.Join(path, m.Path)
		if _, _, e = git(ctx, g, "", "clone", cp, dest); e != nil {
			return "", e
		}
		if e = g.RemoteSetURL(ctx, dest, "origin", m.URL); e != nil {
			return "", e
		}
		parentGit := filepath.Join(a.Home.ClonePath(parent.ID), "modules", m.Name)
		_, _, _ = git(ctx, g, dest, "remote", "remove", "parent")
		if _, _, e = git(ctx, g, dest, "remote", "add", "parent", parentGit); e != nil {
			return "", e
		}
		if _, _, e = git(ctx, g, dest, "fetch", "parent"); e != nil {
			return "", e
		}
		branch := m.Branch
		switch m.Mode {
		case "work":
			if o.Branch != "" {
				branch = o.Branch
			} else {
				branch = m.Branch + "-" + t.Handle
			}
			if e = g.ValidateBranchName(ctx, branch); e != nil {
				return "", e
			}
			ref, _, x := git(ctx, g, cp, "rev-parse", "--verify", "refs/heads/"+branch)
			if x == nil && strings.TrimSpace(ref) != head {
				return "", hub.ConflictError{Message: fmt.Sprintf("branch %s already exists at a different commit", branch)}
			}
			if _, _, e = git(ctx, g, dest, "checkout", "-B", branch, head); e != nil {
				return "", e
			}
		case "follow":
			if _, _, e = git(ctx, g, dest, "checkout", "-B", branch, head); e != nil {
				return "", e
			}
		case "pin":
			branch = ""
			if _, _, e = git(ctx, g, dest, "checkout", "--detach", head); e != nil {
				return "", e
			}
		default:
			return "", fmt.Errorf("invalid parent mode %q", m.Mode)
		}
		if _, _, e = rootGit(ctx, g, a.Home, t.ID, path, "-c", "protocol.file.allow=always", "submodule", "add", "--force", m.URL, m.Path); e != nil {
			return "", e
		}
		if _, _, e = rootGit(ctx, g, a.Home, t.ID, path, "submodule", "absorbgitdirs", m.Path); e != nil {
			return "", e
		}
		nm := module{Name: m.Name, Path: m.Path, URL: m.URL, Branch: branch, Mode: m.Mode}
		if e = rewriteSubmodule(a.Home, t.ID, path, nm); e != nil {
			return "", e
		}
		gm := filepath.Join(path, ".gitmodules")
		if branch != "" {
			_, _, _ = git(ctx, g, "", "config", "-f", gm, "submodule."+m.Name+".branch", branch)
		}
		_, _, _ = git(ctx, g, "", "config", "-f", gm, "submodule."+m.Name+".tasktree-mode", m.Mode)
	}
	if _, _, err = rootGit(ctx, g, a.Home, t.ID, path, "add", "-A"); err != nil {
		return "", err
	}
	if _, _, err = rootGit(ctx, g, a.Home, t.ID, path, "commit", "-m", fmt.Sprintf("tasktree(%s): fork from %s", t.ID, parent.Handle)); err != nil {
		return "", err
	}
	if _, _, err = rootGit(ctx, g, a.Home, t.ID, path, "push", "-u", "origin", taskBranch); err != nil {
		return "", err
	}
	_ = os.Remove(filepath.Join(path, ".git"))
	if err = a.Home.UpdateLocal(func(s *home.LocalState) error { s.Checkouts[t.ID] = home.Checkout{Path: path}; return nil }); err != nil {
		return "", err
	}
	a.refreshTaskRecord(ctx, t.ID)
	if !o.SkipBootstrap {
		if err = a.Bootstrap(ctx, t.ID, path, progress); err != nil {
			return "", err
		}
	}
	return path, nil
}

func (a App) addParentRemotes(ctx context.Context, t record.Task, root string) {
	if t.Parent == nil {
		return
	}
	local, e := a.Home.LoadLocal()
	if e != nil {
		return
	}
	pc, ok := local.Checkouts[*t.Parent]
	if !ok {
		return
	}
	ms, e := modules(root)
	if e != nil {
		return
	}
	g := a.Git.WithDefaults()
	for _, m := range ms {
		p := filepath.Join(a.Home.ClonePath(*t.Parent), "modules", m.Name)
		if _, e := os.Stat(p); e != nil {
			continue
		}
		rp := filepath.Join(root, m.Path)
		_, _, _ = git(ctx, g, rp, "remote", "remove", "parent")
		_, _, _ = git(ctx, g, rp, "remote", "add", "parent", p)
		_, _, _ = git(ctx, g, rp, "fetch", "parent")
		_ = pc
	}
}

type PushResult struct{ Name, Message, Hint string }

func (a App) Push(ctx context.Context, id, root string, names []string) ([]PushResult, error) {
	ms, e := modules(root)
	if e != nil {
		return nil, e
	}
	explicit := len(names) > 0
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	if explicit {
		for n := range wanted {
			found := false
			for _, m := range ms {
				found = found || m.Name == n
			}
			if !found {
				return nil, fmt.Errorf("repository %q not found", n)
			}
		}
	}
	t, e := a.Info(id)
	if e != nil {
		return nil, e
	}
	var parent record.Task
	if t.Parent != nil {
		parent, _ = a.Info(*t.Parent)
	}
	g := a.Git.WithDefaults()
	results := []PushResult{}
	for _, m := range ms {
		if explicit && !wanted[m.Name] {
			continue
		}
		p := filepath.Join(root, m.Path)
		branch, e := g.CurrentBranch(ctx, p)
		if e != nil {
			return results, e
		}
		if branch == "" {
			if explicit {
				return results, fmt.Errorf("repository %s is detached", m.Name)
			}
			results = append(results, PushResult{Name: m.Name, Message: "skipped (detached)"})
			continue
		}
		def, _ := g.DefaultBranch(ctx, p)
		if branch == "main" || branch == "master" || strings.HasPrefix(branch, "release/") || branch == def {
			return results, GateError{fmt.Sprintf("refusing to push protected branch %s in %s", branch, m.Name)}
		}
		if _, _, e = git(ctx, g, p, "push", "-u", "origin", branch); e != nil {
			return results, e
		}
		r := PushResult{Name: m.Name, Message: "pushed " + branch}
		owner := parseGitHubRepo(m.URL)
		repoFlag := ""
		if owner != "" {
			repoFlag = " --repo " + owner
		}
		if t.Parent != nil && m.Mode == "work" {
			pms := a.parentModules(ctx, parent.ID)
			var pm *module
			for i := range pms {
				if pms[i].Name == m.Name {
					pm = &pms[i]
				}
			}
			if pm != nil && pm.Branch != "" {
				_, _, _ = git(ctx, g, p, "fetch", "origin", pm.Branch)
				if _, _, x := git(ctx, g, p, "remote", "get-url", "parent"); x != nil {
					r.Message += "; fork-point check skipped (no parent remote)"
				} else {
					_, _, _ = git(ctx, g, p, "fetch", "parent")
					fork, _, x := git(ctx, g, p, "merge-base", "HEAD", "refs/remotes/parent/"+pm.Branch)
					if x == nil {
						fork = strings.TrimSpace(fork)
						_, _, parentRef := git(ctx, g, p, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+pm.Branch)
						_, _, anc := git(ctx, g, p, "merge-base", "--is-ancestor", fork, "origin/"+pm.Branch)
						if parentRef != nil || anc != nil {
							countArgs := []string{"rev-list", "--count", fork, "--not", "origin/" + pm.Branch}
							if parentRef != nil {
								countArgs = []string{"rev-list", "--count", fork, "--not", "--remotes=origin"}
							}
							n, _, _ := git(ctx, g, p, countArgs...)
							if strings.TrimSpace(n) == "" {
								n = "1"
							}
							r.Message += fmt.Sprintf("; warning: push %s's %s first, or the PR will include %s parent commits", parent.Handle, m.Name, strings.TrimSpace(n))
						}
					}
				}
				r.Hint = "gh pr create" + repoFlag + " --base " + pm.Branch + " --head " + branch
			}
		}
		results = append(results, r)
	}
	return results, nil
}
func (a App) parentModules(ctx context.Context, id string) []module {
	if p := localCheckoutPath(a.Home, id); p != "" {
		if ms, err := modules(p); err == nil {
			return ms
		}
	}
	out, _, err := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "show", "task/"+id+":.gitmodules")
	if err != nil {
		return nil
	}
	dir, err := os.MkdirTemp("", "tt2-parent-modules-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	if os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte(out), 0600) != nil {
		return nil
	}
	ms, _ := modules(dir)
	return ms
}
func localCheckoutPath(h home.Home, id string) string {
	s, e := h.LoadLocal()
	if e != nil {
		return ""
	}
	return s.Checkouts[id].Path
}
func parseGitHubRepo(u string) string {
	s := strings.TrimSuffix(strings.TrimSpace(u), ".git")
	if i := strings.Index(s, "github.com:"); i >= 0 {
		return strings.TrimPrefix(s[i+len("github.com:"):], "/")
	}
	if i := strings.Index(s, "github.com/"); i >= 0 {
		return strings.TrimPrefix(s[i+len("github.com/"):], "/")
	}
	return ""
}
