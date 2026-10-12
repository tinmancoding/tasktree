package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"gopkg.in/yaml.v3"
)

const importLargeLimit = int64(50 * 1024 * 1024)

// ImportOptions controls the deliberately conservative V1 adoption operation.
type ImportOptions struct {
	Handle, Status           string
	Move, DryRun, AllowLarge bool
}

type V1Source struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Path string `yaml:"path,omitempty"`
}
type v1Metadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	CreatedAt   time.Time         `yaml:"createdAt,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}
type v1Spec struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   v1Metadata `yaml:"metadata"`
	Spec       struct {
		Sources []V1Source `yaml:"sources"`
	} `yaml:"spec"`
}

type ImportSourcePlan struct {
	Name, Path, Type, Branch, Mode string
	Dirty, Stashes, Unpushed       int
	Present                        bool
	URL                            string
}
type ImportPlan struct {
	Handle, Path, Target string
	Sources              []ImportSourcePlan
	RootFiles            int
	RootBytes            int64
	Symlinks             []string
	Warnings, Refusals   []string
	AlreadyImported      bool
	spec                 v1Spec
	specBytes            []byte
}
type ImportResult struct {
	Plan    ImportPlan
	Task    record.Task
	Path    string
	Skipped bool
}

type repoSnapshot struct {
	Head, Symbolic, Status, Stashes, Refs, Config string
	configBytes                                   []byte
}
type rootEntry struct {
	Size, Mtime int64
	Mode        os.FileMode
	Link, Hash  string
}
type importState struct {
	plan               ImportPlan
	id                 string
	before             map[string]repoSnapshot
	rootBefore         map[string]rootEntry
	generatedOriginals map[string][]byte
	absorbed           []ImportSourcePlan
	starter            bool
}

func readV1(root string) (v1Spec, []byte, error) {
	var s v1Spec
	b, err := os.ReadFile(filepath.Join(root, "Tasktree.yml"))
	if err != nil {
		return s, nil, err
	}
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, nil, fmt.Errorf("parse Tasktree.yml: %w", err)
	}
	if s.APIVersion != "tasktree.dev/v1" || s.Kind != "Tasktree" {
		return s, nil, fmt.Errorf("Tasktree.yml must declare apiVersion tasktree.dev/v1 and kind Tasktree")
	}
	seen := map[string]bool{}
	for i := range s.Spec.Sources {
		x := &s.Spec.Sources[i]
		if x.Path == "" {
			x.Path = x.Name
		}
		if x.Name == "" || x.Path == "" {
			return s, nil, fmt.Errorf("source name and path must not be empty")
		}
		if filepath.IsAbs(x.Path) || x.Path == ".." || strings.HasPrefix(filepath.Clean(x.Path), ".."+string(filepath.Separator)) {
			return s, nil, fmt.Errorf("source %s has unsafe path %q", x.Name, x.Path)
		}
		if seen[x.Name] {
			return s, nil, fmt.Errorf("duplicate source name %q", x.Name)
		}
		seen[x.Name] = true
		switch x.Type {
		case "git", "local", "static", "http", "archive":
		default:
			return s, nil, fmt.Errorf("source %s has unsupported type %q", x.Name, x.Type)
		}
	}
	return s, b, nil
}

func (a App) PlanImport(ctx context.Context, root string, o ImportOptions) (ImportPlan, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return ImportPlan{}, err
	}
	p := ImportPlan{Path: abs}
	if _, err := os.Stat(filepath.Join(abs, ".tasktree", "config.yml")); err == nil {
		p.AlreadyImported = true
		p.Handle = filepath.Base(abs)
		p.Target = abs
		return p, nil
	}
	if _, err := os.Stat(filepath.Join(abs, "Tasktree.yml")); os.IsNotExist(err) {
		if _, legacy := os.Stat(filepath.Join(abs, ".tasktree.toml")); legacy == nil {
			p.Refusals = append(p.Refusals, "legacy .tasktree.toml found; run V1 `tasktree migrate` first")
			return p, nil
		}
		return p, fmt.Errorf("read Tasktree.yml: %w", err)
	}
	s, b, err := readV1(abs)
	if err != nil {
		return p, err
	}
	p.spec, p.specBytes = s, b
	p.Handle = o.Handle
	if p.Handle == "" {
		p.Handle = filepath.Base(abs)
	}
	if p.Handle == "" {
		p.Refusals = append(p.Refusals, "could not derive handle; pass --handle")
	}
	if a.Hub.IsSetup() {
		docs, e := a.Hub.ReadAll()
		if e != nil {
			return p, e
		}
		for _, d := range docs {
			if d.Task.Handle == p.Handle {
				p.Refusals = append(p.Refusals, fmt.Sprintf("handle %q already exists; pass --handle", p.Handle))
				break
			}
		}
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return p, err
	}
	p.Target = abs
	if o.Move {
		p.Target = filepath.Join(cfg.TasksRoot, p.Handle)
		if e := preflightMove(abs, p.Target); e != nil {
			p.Refusals = append(p.Refusals, e.Error())
		}
	}
	declared := map[string]bool{}
	g := a.Git.WithDefaults()
	for _, src := range s.Spec.Sources {
		sp := ImportSourcePlan{Name: src.Name, Path: filepath.Clean(src.Path), Type: src.Type}
		if src.Type == "git" {
			declared[sp.Path] = true
		}
		full := filepath.Join(abs, sp.Path)
		_, statErr := os.Lstat(full)
		sp.Present = statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return p, statErr
		}
		if src.Type == "git" && !sp.Present {
			p.Warnings = append(p.Warnings, fmt.Sprintf("git source %s is missing and will not be added", src.Name))
		}
		if src.Type == "git" && sp.Present {
			dot, e := os.Stat(filepath.Join(full, ".git"))
			if e != nil || !dot.IsDir() {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s is not a standalone repository with a .git directory", src.Name))
				p.Sources = append(p.Sources, sp)
				continue
			}
			for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "index.lock"} {
				if _, e := os.Stat(filepath.Join(full, ".git", marker)); e == nil {
					p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has operation in progress (%s)", src.Name, marker))
				}
			}
			url, _, e := git(ctx, g, full, "remote", "get-url", "origin")
			if e != nil {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has no origin remote", src.Name))
			} else {
				sp.URL = strings.TrimSpace(url)
			}
			branch, _ := g.CurrentBranch(ctx, full)
			sp.Branch = branch
			if branch == "" {
				sp.Mode = "pin"
			} else {
				def, _ := g.DefaultBranch(ctx, full)
				if branch == def {
					sp.Mode = "follow"
				} else {
					sp.Mode = "work"
				}
			}
			status, _, _ := g.Run(ctx, full, map[string]string{"GIT_OPTIONAL_LOCKS": "0"}, "status", "--porcelain=v2", "--untracked-files=all")
			sp.Dirty = countLines(status)
			st, _, _ := git(ctx, g, full, "stash", "list")
			sp.Stashes = countLines(st)
			un, _, _ := git(ctx, g, full, "rev-list", "--branches", "--tags", "HEAD", "--not", "--remotes=origin")
			sp.Unpushed = countLines(un)
		}
		p.Sources = append(p.Sources, sp)
	}
	if err := filepath.WalkDir(abs, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == abs {
			return nil
		}
		rel, _ := filepath.Rel(abs, path)
		rel = filepath.Clean(rel)
		for x := range declared {
			if rel == x {
				return filepath.SkipDir
			}
		}
		if d.IsDir() && d.Name() == ".git" {
			p.Refusals = append(p.Refusals, fmt.Sprintf("undeclared nested git repository at %s", filepath.Dir(rel)))
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			p.Symlinks = append(p.Symlinks, rel)
			return nil
		}
		if !d.IsDir() && rel != "Tasktree.yml" {
			info, er := d.Info()
			if er != nil {
				return er
			}
			p.RootFiles++
			p.RootBytes += info.Size()
			if info.Size() > importLargeLimit && !o.AllowLarge {
				p.Refusals = append(p.Refusals, fmt.Sprintf("root file %s is larger than 50 MB; pass --allow-large", rel))
			}
		}
		return nil
	}); err != nil {
		return p, err
	}
	sort.Strings(p.Symlinks)
	return p, nil
}

func preflightMove(root, target string) error {
	if root == target {
		return nil
	}
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return fmt.Errorf("move target %s exists and is not empty", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(target)
	for {
		if _, err := os.Stat(parent); err == nil {
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("cannot find existing target parent")
		}
		parent = next
	}
	a, err := os.Stat(root)
	if err != nil {
		return err
	}
	b, err := os.Stat(parent)
	if err != nil {
		return err
	}
	as, aok := a.Sys().(*syscall.Stat_t)
	bs, bok := b.Sys().(*syscall.Stat_t)
	if !aok || !bok || as.Dev != bs.Dev {
		return fmt.Errorf("move target is on a different filesystem; atomic rename required")
	}
	return nil
}

func countLines(s string) int {
	n := 0
	for _, x := range strings.Split(strings.TrimSpace(s), "\n") {
		if x != "" {
			n++
		}
	}
	return n
}

func PrintImportPlan(w io.Writer, p ImportPlan) {
	fmt.Fprintf(w, "handle: %s\npath: %s\ntarget: %s\n", p.Handle, p.Path, p.Target)
	for _, s := range p.Sources {
		fmt.Fprintf(w, "source %s: path=%s type=%s", s.Name, s.Path, s.Type)
		if s.Type == "git" {
			if !s.Present {
				fmt.Fprint(w, " missing")
			} else {
				br := s.Branch
				if br == "" {
					br = "(detached)"
				}
				fmt.Fprintf(w, " branch=%s mode=%s dirty=%d stashes=%d unpushed=%d", br, s.Mode, s.Dirty, s.Stashes, s.Unpushed)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "root files: %d (%d bytes)\n", p.RootFiles, p.RootBytes)
	for _, x := range p.Symlinks {
		fmt.Fprintf(w, "symlink: %s\n", x)
	}
	for _, x := range p.Warnings {
		fmt.Fprintf(w, "warning: %s\n", x)
	}
	for _, x := range p.Refusals {
		fmt.Fprintf(w, "refusal: %s\n", x)
	}
}

func snapshotRepo(ctx context.Context, a App, path string) (repoSnapshot, error) {
	var s repoSnapshot
	var err error
	run := func(dst *string, args ...string) bool {
		var e error
		*dst, _, e = a.Git.Run(ctx, path, map[string]string{"GIT_OPTIONAL_LOCKS": "0"}, args...)
		if e != nil {
			err = e
			return false
		}
		return true
	}
	if !run(&s.Head, "rev-parse", "HEAD") {
		return s, err
	}
	s.Symbolic, _, _ = a.Git.Run(ctx, path, map[string]string{"GIT_OPTIONAL_LOCKS": "0"}, "symbolic-ref", "-q", "HEAD")
	if !run(&s.Status, "status", "--porcelain=v2", "--untracked-files=all", "--ignored") || !run(&s.Stashes, "stash", "list", "--format=%H %gd %gs") || !run(&s.Refs, "for-each-ref", "--format=%(refname) %(objectname)") || !run(&s.Config, "config", "--list", "--local") {
		return s, err
	}
	gitDir, _, dirErr := git(ctx, a.Git, path, "rev-parse", "--absolute-git-dir")
	if dirErr != nil {
		return s, dirErr
	}
	s.configBytes, err = os.ReadFile(filepath.Join(strings.TrimSpace(gitDir), "config"))
	return s, err
}

func rootSnapshot(root string, sources []ImportSourcePlan) (map[string]rootEntry, error) {
	skip := map[string]bool{}
	for _, s := range sources {
		if s.Type == "git" && s.Present {
			skip[filepath.Clean(s.Path)] = true
		}
	}
	out := map[string]rootEntry{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.Clean(rel)
		for x := range skip {
			if rel == x {
				return filepath.SkipDir
			}
		}
		info, e := os.Lstat(path)
		if e != nil {
			return e
		}
		ent := rootEntry{Size: info.Size(), Mtime: info.ModTime().UnixNano(), Mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			ent.Link, e = os.Readlink(path)
			if e != nil {
				return e
			}
		} else if info.Mode().IsRegular() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			ent.Hash = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		out[rel] = ent
		return nil
	})
	return out, err
}

func snapshotsEqual(a, b repoSnapshot, allowWorktree bool) error {
	if a.Head != b.Head {
		return fmt.Errorf("HEAD changed")
	}
	if a.Symbolic != b.Symbolic {
		return fmt.Errorf("symbolic HEAD changed")
	}
	if a.Status != b.Status {
		return fmt.Errorf("status changed")
	}
	if a.Stashes != b.Stashes {
		return fmt.Errorf("stashes changed")
	}
	if a.Refs != b.Refs {
		return fmt.Errorf("refs changed")
	}
	if a.Config != b.Config && !allowWorktree {
		return fmt.Errorf("local config changed")
	}
	if allowWorktree {
		clean := func(s string) string {
			lines := []string{}
			for _, x := range strings.Split(s, "\n") {
				if !strings.HasPrefix(strings.ToLower(x), "core.worktree=") {
					lines = append(lines, x)
				}
			}
			return strings.Join(lines, "\n")
		}
		if clean(a.Config) != clean(b.Config) {
			return fmt.Errorf("local config changed beyond core.worktree")
		}
	}
	return nil
}

func testImportFail(step int) error {
	v := os.Getenv("TASKTREE_TEST_IMPORT_FAIL")
	if v == strconv.Itoa(step) || v == fmt.Sprintf("step-%d", step) {
		return fmt.Errorf("test-only injected import failure at step %d", step)
	}
	return nil
}

func (a App) Import(ctx context.Context, root string, o ImportOptions) (res ImportResult, err error) {
	p, err := a.PlanImport(ctx, root, o)
	res.Plan = p
	if err != nil {
		return res, err
	}
	if p.AlreadyImported {
		res.Skipped = true
		res.Path = p.Path
		return res, nil
	}
	if len(p.Refusals) > 0 {
		return res, GateError{strings.Join(p.Refusals, "; ")}
	}
	if o.DryRun {
		res.Path = p.Path
		return res, nil
	}
	if !a.Hub.IsSetup() {
		return res, fmt.Errorf("tasktree is not set up; run tt2 setup")
	}
	if o.Status == "" {
		o.Status = "active"
	}
	if err = record.ValidateStatus(o.Status); err != nil {
		return res, err
	}
	id, err := record.NewID()
	if err != nil {
		return res, err
	}
	st := &importState{plan: p, id: id, before: map[string]repoSnapshot{}, generatedOriginals: map[string][]byte{}}
	for _, rel := range []string{".gitmodules", filepath.Join(".tasktree", ".gitignore")} {
		if b, e := os.ReadFile(filepath.Join(p.Path, rel)); e == nil {
			st.generatedOriginals[rel] = b
		}
	}
	for _, s := range p.Sources {
		if s.Type == "git" && s.Present {
			st.before[s.Name], err = snapshotRepo(ctx, a, filepath.Join(p.Path, s.Path))
			if err != nil {
				return res, err
			}
		}
	}
	st.rootBefore, err = rootSnapshot(p.Path, p.Sources)
	if err != nil {
		return res, err
	}
	if err = testImportFail(1); err != nil {
		return res, err
	}
	started := true
	defer func() {
		if err != nil && started {
			rb := a.rollbackImport(ctx, st)
			if rb != nil {
				err = fmt.Errorf("%w; ROLLBACK INCOMPLETE: %v", err, rb)
			} else {
				err = fmt.Errorf("%w; rollback restored the workspace exactly", err)
			}
		}
	}()
	if err = os.MkdirAll(a.Home.ClonesPath(), 0755); err != nil {
		return
	}
	_, _, err = a.Git.Run(ctx, p.Path, rootEnv(a.Home, id, p.Path), "init")
	if err != nil {
		return
	}
	_, _, err = rootGitInit(ctx, a.Git, a.Home, id, p.Path, "checkout", "--orphan", "task/"+id)
	if err != nil {
		return
	}
	_, _, err = rootGitInit(ctx, a.Git, a.Home, id, p.Path, "remote", "add", "origin", a.Home.GitPath())
	if err != nil {
		return
	}
	if err = os.MkdirAll(filepath.Join(a.Home.ClonePath(id), "info"), 0755); err == nil {
		err = os.WriteFile(filepath.Join(a.Home.ClonePath(id), "info", "exclude"), []byte(".DS_Store\n"), 0644)
	}
	if err != nil {
		return
	}
	if err = testImportFail(2); err != nil {
		return
	}
	for _, s := range p.Sources {
		if s.Type != "git" || !s.Present {
			continue
		}
		_, _, err = rootGitInit(ctx, a.Git, a.Home, id, p.Path, "-c", "protocol.file.allow=always", "submodule", "add", "--force", s.URL, s.Path)
		if err != nil {
			return
		}
		// Track it before absorption: absorbgitdirs can fail after moving the git dir.
		st.absorbed = append(st.absorbed, s)
		_, _, err = rootGitInit(ctx, a.Git, a.Home, id, p.Path, "submodule", "absorbgitdirs", "--", s.Path)
		if err != nil {
			return
		}
		if err = rewriteSubmodule(a.Home, id, p.Path, module{Name: s.Name, Path: s.Path}); err != nil {
			return
		}
		gm := filepath.Join(p.Path, ".gitmodules")
		if s.Branch != "" {
			_, _, err = git(ctx, a.Git, "", "config", "-f", gm, "submodule."+s.Name+".branch", s.Branch)
			if err != nil {
				return
			}
		}
		_, _, err = git(ctx, a.Git, "", "config", "-f", gm, "submodule."+s.Name+".tasktree-mode", s.Mode)
		if err != nil {
			return
		}
	}
	if err = testImportFail(3); err != nil {
		return
	}
	if _, e := os.Stat(filepath.Join(p.Path, "AGENTS.md")); os.IsNotExist(e) {
		t := record.Task{ID: id, Handle: p.Handle, Title: p.spec.Metadata.Name}
		if t.Title == "" {
			t.Title = filepath.Base(p.Path)
		}
		err = os.WriteFile(filepath.Join(p.Path, "AGENTS.md"), []byte(starterAgents(t)), 0644)
		st.starter = err == nil
		if err != nil {
			return
		}
	}
	if err = testImportFail(4); err != nil {
		return
	}
	if err = os.Remove(filepath.Join(p.Path, "Tasktree.yml")); err != nil {
		return
	}
	if err = os.MkdirAll(filepath.Join(p.Path, ".tasktree"), 0755); err != nil {
		return
	}
	b, _ := yaml.Marshal(TasktreeConfig{APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: id})
	if err = os.WriteFile(filepath.Join(p.Path, ".tasktree", "config.yml"), b, 0644); err != nil {
		return
	}
	if err = os.WriteFile(filepath.Join(p.Path, ".tasktree", ".gitignore"), []byte("task.md\n"), 0644); err != nil {
		return
	}
	_, _, err = rootGit(ctx, a.Git, a.Home, id, p.Path, "add", "-A")
	if err != nil {
		return
	}
	cfg, _ := a.Home.LoadConfig()
	name := cfg.Author
	if name == "" {
		name = "tasktree"
	}
	machine := cfg.Machine
	if machine == "" {
		machine = "localhost"
	}
	env := rootEnv(a.Home, id, p.Path)
	env["GIT_AUTHOR_NAME"], env["GIT_COMMITTER_NAME"] = name, name
	env["GIT_AUTHOR_EMAIL"], env["GIT_COMMITTER_EMAIL"] = name+"@"+machine, name+"@"+machine
	_, _, err = a.Git.Run(ctx, p.Path, env, "commit", "-m", fmt.Sprintf("tasktree(%s): import from V1", id))
	if err != nil {
		return
	}
	if err = testImportFail(5); err != nil {
		return
	}
	if err = a.verifyImport(ctx, st); err != nil {
		return
	}
	if err = testImportFail(6); err != nil {
		return
	}
	if err = testImportFail(7); err != nil {
		return
	}
	if err = testImportFail(8); err != nil {
		return
	}
	_, _, err = rootGit(ctx, a.Git, a.Home, id, p.Path, "push", "-u", "origin", "task/"+id)
	if err != nil {
		return
	}
	title := p.spec.Metadata.Name
	if title == "" {
		title = filepath.Base(p.Path)
	}
	created := p.spec.Metadata.CreatedAt
	if created.IsZero() {
		created = a.Now().UTC()
	}
	labels := make([]string, 0, len(p.spec.Metadata.Labels))
	for k := range p.spec.Metadata.Labels {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	refs := importRefs(p.spec.Metadata.Annotations)
	task := record.Task{ID: id, Handle: p.Handle, Title: title, Status: o.Status, Labels: labels, Refs: refs, CreatedAt: created, UpdatedAt: a.Now().UTC(), Description: p.spec.Metadata.Description}
	doc, e := record.New(task)
	if e != nil {
		err = e
		return
	}
	entryID, e := record.NewID()
	if e != nil {
		err = e
		return
	}
	body := "Imported from V1\n\n```yaml\n" + strings.TrimRight(string(p.specBytes), "\n") + "\n```"
	if len(p.Warnings) > 0 {
		body += "\n\nWarnings:\n- " + strings.Join(p.Warnings, "\n- ")
	}
	if e = doc.Append(record.Entry{ID: entryID, Timestamp: a.Now().UTC(), Kind: "note", Author: name, Machine: machine, Body: body}); e != nil {
		err = e
		return
	}
	if err = a.Hub.Create(ctx, doc, "import from V1", name, machine); err != nil {
		return
	}
	finalPath := p.Path
	if err = a.Home.UpdateLocal(func(s *home.LocalState) error { s.Checkouts[id] = home.Checkout{Path: finalPath}; return nil }); err != nil {
		return
	}
	started = false
	if o.Move && p.Target != p.Path {
		if err = testImportFail(9); err != nil {
			return
		}
		if entries, e := os.ReadDir(p.Target); e == nil && len(entries) == 0 {
			if e = os.Remove(p.Target); e != nil {
				err = e
				return
			}
		}
		if e = os.MkdirAll(filepath.Dir(p.Target), 0755); e != nil {
			err = e
			return
		}
		if e = os.Rename(p.Path, p.Target); e != nil {
			err = e
			return
		}
		moved := true
		for _, s := range st.absorbed {
			if e = rewriteSubmodule(a.Home, id, p.Target, module{Name: s.Name, Path: s.Path}); e != nil {
				_ = os.Rename(p.Target, p.Path)
				for _, x := range st.absorbed {
					_ = rewriteSubmodule(a.Home, id, p.Path, module{Name: x.Name, Path: x.Path})
				}
				err = e
				return
			}
		}
		if e = a.Home.UpdateLocal(func(ls *home.LocalState) error { ls.Checkouts[id] = home.Checkout{Path: p.Target}; return nil }); e != nil {
			if moved {
				_ = os.Rename(p.Target, p.Path)
			}
			for _, x := range st.absorbed {
				_ = rewriteSubmodule(a.Home, id, p.Path, module{Name: x.Name, Path: x.Path})
			}
			err = e
			return
		}
		finalPath = p.Target
	}
	res.Task = task
	res.Path = finalPath
	return res, nil
}

func importRefs(a map[string]string) []record.Ref {
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	jira := regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)
	out := []record.Ref{}
	for _, k := range keys {
		v := a[k]
		if (k == "jira.ticket" || k == "ticket" || k == "jira") && jira.MatchString(v) {
			out = append(out, record.Ref{Type: "jira", Value: v})
		} else if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			out = append(out, record.Ref{Type: "url", Value: v})
		}
	}
	return out
}

func (a App) verifyImport(ctx context.Context, st *importState) error {
	for _, s := range st.absorbed {
		after, e := snapshotRepo(ctx, a, filepath.Join(st.plan.Path, s.Path))
		if e != nil {
			return e
		}
		if e = snapshotsEqual(st.before[s.Name], after, true); e != nil {
			return fmt.Errorf("repository %s verification failed: %w", s.Name, e)
		}
	}
	after, e := rootSnapshot(st.plan.Path, st.plan.Sources)
	if e != nil {
		return e
	}
	allowed := map[string]bool{"Tasktree.yml": true, ".gitmodules": true, ".tasktree": true, "AGENTS.md": st.starter}
	for path, b := range st.rootBefore {
		if allowed[path] || strings.HasPrefix(path, ".tasktree"+string(filepath.Separator)) {
			continue
		}
		x, ok := after[path]
		if !ok || x != b {
			return fmt.Errorf("root path %s changed during import", path)
		}
	}
	return nil
}

func (a App) rollbackImport(ctx context.Context, st *importState) error {
	var problems []string
	for i := len(st.absorbed) - 1; i >= 0; i-- {
		s := st.absorbed[i]
		src := filepath.Join(a.Home.ClonePath(st.id), "modules", s.Name)
		dst := filepath.Join(st.plan.Path, s.Path, ".git")
		if info, e := os.Stat(src); e == nil && info.IsDir() {
			_ = os.Remove(dst)
			if e := os.Rename(src, dst); e != nil {
				problems = append(problems, fmt.Sprintf("move %s back to %s: %v", src, dst, e))
				continue
			}
		} else if info, de := os.Stat(dst); de != nil || !info.IsDir() {
			problems = append(problems, fmt.Sprintf("cannot locate git directory for %s at %s or %s", s.Name, src, dst))
			continue
		}
		if e := os.WriteFile(filepath.Join(dst, "config"), st.before[s.Name].configBytes, 0644); e != nil {
			problems = append(problems, e.Error())
		}
	}
	for _, rel := range []string{".gitmodules", filepath.Join(".tasktree", ".gitignore")} {
		path := filepath.Join(st.plan.Path, rel)
		if b, ok := st.generatedOriginals[rel]; ok {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, b, 0o644)
			if original, exists := st.rootBefore[rel]; exists {
				_ = os.Chmod(path, original.Mode.Perm())
				when := time.Unix(0, original.Mtime)
				_ = os.Chtimes(path, when, when)
			}
		} else {
			_ = os.Remove(path)
		}
	}
	_ = os.Remove(filepath.Join(st.plan.Path, ".tasktree", "config.yml"))
	_ = os.Remove(filepath.Join(st.plan.Path, ".tasktree"))
	if st.starter {
		_ = os.Remove(filepath.Join(st.plan.Path, "AGENTS.md"))
	}
	tasktreePath := filepath.Join(st.plan.Path, "Tasktree.yml")
	if e := os.WriteFile(tasktreePath, st.plan.specBytes, 0644); e != nil {
		problems = append(problems, e.Error())
	} else if original, ok := st.rootBefore["Tasktree.yml"]; ok {
		_ = os.Chmod(tasktreePath, original.Mode.Perm())
		when := time.Unix(0, original.Mtime)
		_ = os.Chtimes(tasktreePath, when, when)
	}
	if e := os.RemoveAll(a.Home.ClonePath(st.id)); e != nil {
		problems = append(problems, e.Error())
	}
	for _, s := range st.plan.Sources {
		if s.Type != "git" || !s.Present {
			continue
		}
		after, e := snapshotRepo(ctx, a, filepath.Join(st.plan.Path, s.Path))
		if e != nil {
			problems = append(problems, fmt.Sprintf("snapshot %s: %v", s.Name, e))
			continue
		}
		if e = snapshotsEqual(st.before[s.Name], after, false); e != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", s.Name, e))
		}
	}
	after, e := rootSnapshot(st.plan.Path, st.plan.Sources)
	if e != nil {
		problems = append(problems, e.Error())
	} else {
		for p, b := range st.rootBefore {
			if x, ok := after[p]; !ok || x != b {
				problems = append(problems, "root path not restored: "+p)
			}
		}
		for p := range after {
			if _, ok := st.rootBefore[p]; !ok {
				problems = append(problems, "unexpected root path after rollback: "+p)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s; preserve the workspace and manually move any remaining %s/modules/<name> directories back to each source's .git directory", strings.Join(problems, "; "), a.Home.ClonePath(st.id))
	}
	return nil
}

// ReadV1Registry reads only the ordered path entries needed by import --all.
func ReadV1Registry(path string) ([]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var out []string
	scan := bufio.NewScanner(strings.NewReader(string(b)))
	in := false
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "[[tasktrees]]" {
			in = true
			continue
		}
		if in && strings.HasPrefix(line, "path") {
			_, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			x, e := strconv.Unquote(v)
			if e != nil {
				return nil, fmt.Errorf("parse registry path %q: %w", v, e)
			}
			out = append(out, x)
			in = false
		}
	}
	return out, scan.Err()
}
