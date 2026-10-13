package app

import (
	"bufio"
	"context"
	"fmt"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"os"
	"strconv"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/repoalias"
	"gopkg.in/yaml.v3"
)

type DefinitionRepo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Branch  string `json:"branch"`
	Gitlink string `json:"gitlink"`
}
type DefinitionInfo struct {
	State string           `json:"state"`
	Repos []DefinitionRepo `json:"repos"`
}

func (a App) DefinitionExists(ctx context.Context, id string) bool {
	_, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "show-ref", "--verify", "--quiet", "refs/heads/task/"+id)
	return e == nil
}
func (a App) Definition(ctx context.Context, id string) (DefinitionInfo, error) {
	r := DefinitionInfo{State: "none", Repos: []DefinitionRepo{}}
	if !a.DefinitionExists(ctx, id) {
		return r, nil
	}
	r.State = "defined"
	gm, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "show", "task/"+id+":.gitmodules")
	if e != nil {
		return r, nil
	}
	f, e := os.CreateTemp("", "tt2-gm-*")
	if e != nil {
		return r, e
	}
	name := f.Name()
	defer os.Remove(name)
	_, _ = f.WriteString(gm)
	_ = f.Close()
	ms, e := modulesFromFile(name)
	if e != nil {
		return r, e
	}
	for _, m := range ms {
		sha, _, x := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "rev-parse", "task/"+id+":"+m.Path)
		if x != nil {
			return r, x
		}
		r.Repos = append(r.Repos, DefinitionRepo{Name: m.Name, Path: m.Path, Mode: m.Mode, Branch: m.Branch, Gitlink: strings.TrimSpace(sha)})
	}
	return r, nil
}
func modulesFromFile(path string) ([]module, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []module
	var current *module
	s := bufio.NewScanner(strings.NewReader(string(b)))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[submodule ") {
			q := strings.TrimSuffix(strings.TrimPrefix(line, "[submodule "), "]")
			name, e := strconv.Unquote(q)
			if e != nil {
				name = strings.Trim(q, "\"")
			}
			out = append(out, module{Name: name})
			current = &out[len(out)-1]
			continue
		}
		if current == nil {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if u, e := strconv.Unquote(v); e == nil {
			v = u
		}
		switch k {
		case "path":
			current.Path = v
		case "url":
			current.URL = v
		case "branch":
			current.Branch = v
		case "tasktree-mode":
			current.Mode = v
		}
	}
	return out, s.Err()
}

func validRepoName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\")
}
func quoteGitConfig(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `\n`, `\\n`).Replace(s) + `"`
}
func renderModules(ms []module) []byte {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "[submodule %s]\n\tpath = %s\n\turl = %s\n", quoteGitConfig(m.Name), m.Path, m.URL)
		if m.Branch != "" {
			fmt.Fprintf(&b, "\tbranch = %s\n", m.Branch)
		}
		fmt.Fprintf(&b, "\ttasktree-mode = %s\n", m.Mode)
	}
	return []byte(b.String())
}

func (a App) resolveDefinitionRepo(ctx context.Context, o AddOptions) (module, string, error) {
	store := repoalias.NewStore(a.Home.AliasesPath())
	if u, ok, e := store.Resolve(o.URL); e != nil {
		return module{}, "", e
	} else if ok {
		o.URL = u
	}
	if o.Name == "" {
		o.Name = deriveName(o.URL)
	}
	if !validRepoName(o.Name) {
		return module{}, "", ValidationError{fmt.Sprintf("invalid repository name %q", o.Name)}
	}
	if o.Branch != "" {
		if e := a.Git.ValidateBranchName(ctx, o.Branch); e != nil {
			return module{}, "", ValidationError{e.Error()}
		}
	}
	mgr, e := cacheManager(a.Git.WithDefaults())
	if e != nil {
		return module{}, "", e
	}
	cp, e := mgr.Ensure(ctx, o.URL)
	if e != nil {
		return module{}, "", e
	}
	g := a.Git.WithDefaults()
	resolve := func(ref string) (string, error) {
		v, _, x := git(ctx, g, cp, "rev-parse", ref+"^{commit}")
		return strings.TrimSpace(v), x
	}
	headBranch, _, _ := git(ctx, g, cp, "symbolic-ref", "--short", "HEAD")
	headBranch = strings.TrimSpace(strings.TrimPrefix(headBranch, "refs/heads/"))
	if headBranch == "" {
		headBranch = "main"
	}
	var sha, branch, kind string
	if o.Branch != "" {
		branch = o.Branch
		if v, x := resolve("refs/heads/" + o.Branch); x == nil {
			sha = v
			kind = "branch"
		} else {
			base := o.From
			if base == "" {
				base = headBranch
			}
			if v, x = resolveRefInCache(ctx, g, cp, base); x != nil {
				return module{}, "", x
			} else {
				sha = v
			}
			kind = "created"
		}
	}
	if o.Branch == "" && o.From != "" {
		if v, x := resolve("refs/heads/" + o.From); x == nil {
			sha = v
			branch = o.From
			kind = "branch"
		} else if v, x := resolveRefInCache(ctx, g, cp, o.From); x == nil {
			sha = v
			kind = "headless"
		} else {
			return module{}, "", x
		}
	}
	if o.Branch == "" && o.From == "" {
		v, x := resolve("refs/heads/" + headBranch)
		if x != nil {
			return module{}, "", x
		}
		sha = v
		branch = headBranch
		kind = "default"
	}
	mode := o.Mode
	if mode == "" {
		if o.Branch != "" {
			mode = "work"
		} else if kind == "headless" {
			mode = "pin"
		} else {
			mode = "follow"
		}
	}
	if mode != "work" && mode != "follow" && mode != "pin" {
		return module{}, "", ValidationError{fmt.Sprintf("invalid mode %q", mode)}
	}
	if mode == "pin" {
		branch = ""
	} else if branch == "" {
		return module{}, "", ValidationError{fmt.Sprintf("mode %s requires a branch", mode)}
	}
	return module{Name: o.Name, Path: o.Name, URL: o.URL, Branch: branch, Mode: mode}, sha, nil
}
func resolveRefInCache(ctx context.Context, g interface {
	Run(context.Context, string, map[string]string, ...string) (string, string, error)
}, cp, ref string) (string, error) {
	v, _, e := g.Run(ctx, cp, nil, "rev-parse", ref+"^{commit}")
	return strings.TrimSpace(v), e
}

type treeEdit func([]module, map[string]string) ([]module, map[string]string, error)

func (a App) editDefinition(ctx context.Context, id, message string, edit treeEdit) error {
	return a.Hub.WithLock(ctx, func() error {
		g := a.Git.WithDefaults()
		ref := "refs/heads/task/" + id
		for attempt := 0; attempt < 3; attempt++ {
			old, _, oldErr := git(ctx, g, a.Home.GitPath(), "rev-parse", "--verify", ref)
			old = strings.TrimSpace(old)
			exists := oldErr == nil
			idx, e := os.CreateTemp("", "tt2-index-*")
			if e != nil {
				return e
			}
			idxPath := idx.Name()
			idx.Close()
			os.Remove(idxPath)
			defer os.Remove(idxPath)
			env := map[string]string{"GIT_INDEX_FILE": idxPath}
			if exists {
				_, _, e = g.Run(ctx, a.Home.GitPath(), env, "read-tree", old)
			} else {
				_, _, e = g.Run(ctx, a.Home.GitPath(), env, "read-tree", "--empty")
			}
			if e != nil {
				return e
			}
			ms := []module{}
			if exists {
				if gm, _, x := git(ctx, g, a.Home.GitPath(), "show", old+":.gitmodules"); x == nil {
					p, _ := os.CreateTemp("", "tt2-gm-*")
					pn := p.Name()
					p.WriteString(gm)
					p.Close()
					ms, x = modulesFromFile(pn)
					os.Remove(pn)
					if x != nil {
						return x
					}
				}
			}
			oldModules := append([]module(nil), ms...)
			files := map[string]string{}
			ms, files, e = edit(ms, files)
			if e != nil {
				return e
			}
			kept := map[string]bool{}
			for _, m := range ms {
				kept[m.Path] = true
			}
			for _, m := range oldModules {
				if !kept[m.Path] {
					_, _, _ = g.Run(ctx, a.Home.GitPath(), env, "update-index", "--force-remove", m.Path)
				}
			}
			if !exists {
				cfg := TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: id}
				b, _ := yamlMarshal(cfg)
				if _, ok := files[".tasktree/config.yml"]; !ok {
					files[".tasktree/config.yml"] = string(b)
				}
				files[".tasktree/.gitignore"] = "task.md\n"
				if _, ok := files["AGENTS.md"]; !ok {
					t, _ := a.Info(id)
					files["AGENTS.md"] = starterAgents(t)
				}
			}
			if len(ms) > 0 {
				files[".gitmodules"] = string(renderModules(ms))
			} else if exists {
				_, _, _ = g.Run(ctx, a.Home.GitPath(), env, "update-index", "--force-remove", ".gitmodules")
			}
			for path, content := range files {
				if strings.HasPrefix(path, "gitlink:") {
					continue
				}
				mode := "100644"
				if strings.HasPrefix(path, "120000:") {
					mode = "120000"
					path = strings.TrimPrefix(path, "120000:")
				} else if strings.HasPrefix(path, "100755:") {
					mode = "100755"
					path = strings.TrimPrefix(path, "100755:")
				}
				if e = putIndexBlob(ctx, g, a.Home.GitPath(), env, path, content, mode); e != nil {
					return e
				}
			}
			for _, m := range ms {
				sha := files["gitlink:"+m.Path]
				if sha != "" {
					if _, _, e = g.Run(ctx, a.Home.GitPath(), env, "update-index", "--add", "--cacheinfo", "160000,"+sha+","+m.Path); e != nil {
						return e
					}
				}
			}
			tree, _, e := g.Run(ctx, a.Home.GitPath(), env, "write-tree")
			if e != nil {
				return e
			}
			cfg, _ := a.Home.LoadConfig()
			parent := ""
			if exists {
				parent = old
			}
			commit, _, e := g.CommitTree(ctx, a.Home.GitPath(), nil, cfg.Author, cfg.Machine, strings.TrimSpace(tree), parent, message)
			if e != nil {
				return e
			}
			update := []string{"update-ref", ref, strings.TrimSpace(commit)}
			if exists {
				update = append(update, old)
			} else {
				update = append(update, strings.Repeat("0", 40))
			}
			if _, _, e = g.Run(ctx, a.Home.GitPath(), nil, update...); e == nil {
				return nil
			}
		}
		return fmt.Errorf("tasktree definition changed concurrently; retry")
	})
}
func yamlMarshal(v any) ([]byte, error) { return yaml.Marshal(v) }
func putIndexBlob(ctx context.Context, g interface {
	Run(context.Context, string, map[string]string, ...string) (string, string, error)
}, repo string, env map[string]string, path, content, mode string) error {
	f, e := os.CreateTemp("", "tt2-blob-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if _, e = f.WriteString(content); e != nil {
		return e
	}
	f.Close()
	sha, _, e := g.Run(ctx, repo, nil, "hash-object", "-w", n)
	if e != nil {
		return e
	}
	_, _, e = g.Run(ctx, repo, env, "update-index", "--add", "--cacheinfo", mode+","+strings.TrimSpace(sha)+","+path)
	return e
}

func (a App) AddDefined(ctx context.Context, id string, o AddOptions) (AddResult, error) {
	ctx = gitx.Explicit(ctx) // D59: explicit transfer, stall detection only
	m, sha, e := a.resolveDefinitionRepo(ctx, o)
	if e != nil {
		return AddResult{}, e
	}
	e = a.editDefinition(ctx, id, fmt.Sprintf("tasktree(%s): add %s", id, m.Name), func(ms []module, f map[string]string) ([]module, map[string]string, error) {
		for _, x := range ms {
			if x.Name == m.Name || x.Path == m.Path {
				return nil, nil, ValidationError{fmt.Sprintf("repository %q already exists", m.Name)}
			}
		}
		ms = append(ms, m)
		f["gitlink:"+m.Path] = sha
		return ms, f, nil
	})
	r := AddResult{Name: m.Name, Mode: m.Mode, Branch: m.Branch}
	if e == nil {
		registerAliases(repoalias.NewStore(a.Home.AliasesPath()), m.URL, &r)
	}
	return r, e
}
func (a App) RemoveDefined(ctx context.Context, id, name string) error {
	return a.editDefinition(ctx, id, fmt.Sprintf("tasktree(%s): remove %s", id, name), func(ms []module, f map[string]string) ([]module, map[string]string, error) {
		out := ms[:0]
		found := false
		for _, m := range ms {
			if m.Name == name {
				found = true
				continue
			}
			out = append(out, m)
			sha, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "rev-parse", "task/"+id+":"+m.Path)
			if e != nil {
				return nil, nil, e
			}
			f["gitlink:"+m.Path] = strings.TrimSpace(sha)
		}
		if !found {
			return nil, nil, ValidationError{fmt.Sprintf("repository %q not found", name)}
		}
		return out, f, nil
	})
}
func (a App) ModeDefined(ctx context.Context, id, name, mode string) error {
	return a.editDefinition(ctx, id, fmt.Sprintf("tasktree(%s): mode %s %s", id, name, mode), func(ms []module, f map[string]string) ([]module, map[string]string, error) {
		found := false
		for i := range ms {
			sha, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "rev-parse", "task/"+id+":"+ms[i].Path)
			if e != nil {
				return nil, nil, e
			}
			f["gitlink:"+ms[i].Path] = strings.TrimSpace(sha)
			if ms[i].Name == name {
				found = true
				if mode != "pin" && ms[i].Branch == "" {
					return nil, nil, ValidationError{fmt.Sprintf("repository %s has no branch; follow and work require a branch", name)}
				}
				ms[i].Mode = mode
				if mode == "pin" {
					ms[i].Branch = ""
				}
			}
		}
		if !found {
			return nil, nil, ValidationError{fmt.Sprintf("repository %q not found", name)}
		}
		return ms, f, nil
	})
}
