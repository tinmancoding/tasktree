package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

	"github.com/pelletier/go-toml/v2"
	"github.com/tinmancoding/tasktree/v2/internal/fsx"
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
	journalDir         string
}

type importJournalRepo struct {
	Name, Path, State string
	Before            repoSnapshotJSON
}
type repoSnapshotJSON struct {
	Head, Symbolic, Status, Stashes, Refs, Config string
	ConfigBytes                                   []byte
}
type importJournal struct {
	ID, Root, Handle   string
	Step               int
	Repos              []importJournalRepo
	RootBefore         map[string]rootEntry
	GeneratedOriginals map[string][]byte
	Starter            bool
}

func snapshotJSON(s repoSnapshot) repoSnapshotJSON {
	return repoSnapshotJSON{s.Head, s.Symbolic, s.Status, s.Stashes, s.Refs, s.Config, s.configBytes}
}
func (s repoSnapshotJSON) snapshot() repoSnapshot {
	return repoSnapshot{s.Head, s.Symbolic, s.Status, s.Stashes, s.Refs, s.Config, s.ConfigBytes}
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
	if id, ok := pendingImportForRoot(a.Home, abs); ok {
		return p, GateError{fmt.Sprintf("incomplete import %s; run tt2 import --recover %s", id, abs)}
	}
	if _, err := os.Lstat(filepath.Join(abs, ".tasktree", "config.yml")); err == nil {
		var marker TasktreeConfig
		b, readErr := os.ReadFile(filepath.Join(abs, ".tasktree", "config.yml"))
		if readErr != nil || yaml.Unmarshal(b, &marker) != nil || marker.TaskID == "" {
			return p, GateError{"incomplete import: invalid .tasktree/config.yml; run tt2 import --recover " + abs}
		}
		_, hubErr := a.Hub.Read(marker.TaskID)
		local, localErr := a.Home.LoadLocal()
		_, localOK := local.Checkouts[marker.TaskID]
		if hubErr != nil || localErr != nil || !localOK {
			return p, GateError{"incomplete import: marker exists without both hub record and local checkout; run tt2 import --recover " + abs}
		}
		p.AlreadyImported = true
		p.Handle = filepath.Base(abs)
		p.Target = abs
		return p, nil
	}
	if _, err := os.Lstat(filepath.Join(abs, "Tasktree.yml")); os.IsNotExist(err) {
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
	if !handlePattern.MatchString(p.Handle) {
		p.Refusals = append(p.Refusals, fmt.Sprintf("invalid handle %q (expected [A-Za-z0-9._-], max 64 characters)", p.Handle))
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
	declaredSources := map[string]string{}
	g := a.Git.WithDefaults()
	for _, src := range s.Spec.Sources {
		sp := ImportSourcePlan{Name: src.Name, Path: filepath.Clean(src.Path), Type: src.Type}
		if src.Type == "git" {
			declared[sp.Path] = true
			if filepath.IsAbs(src.Name) || src.Name == ".." || strings.Contains(src.Name, "..") || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(src.Name) {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has invalid submodule name", src.Name))
			}
		}
		full := filepath.Join(abs, sp.Path)
		fullClean, _ := filepath.Abs(full)
		if previous, duplicate := declaredSources[fullClean]; src.Type == "git" && duplicate {
			p.Refusals = append(p.Refusals, fmt.Sprintf("git sources %s and %s use the same path", previous, src.Name))
		} else if src.Type == "git" {
			declaredSources[fullClean] = src.Name
		}
		info, statErr := os.Lstat(full)
		sp.Present = statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return p, statErr
		}
		if src.Type == "git" && !sp.Present {
			p.Warnings = append(p.Warnings, fmt.Sprintf("git source %s is missing and will not be added", src.Name))
		}
		if src.Type == "git" && sp.Present {
			if info.Mode()&os.ModeSymlink != 0 {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s is a symlink", src.Name))
				p.Sources = append(p.Sources, sp)
				continue
			}
			dot, e := os.Lstat(filepath.Join(full, ".git"))
			if e != nil || !dot.IsDir() || dot.Mode()&os.ModeSymlink != 0 {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s is not a standalone repository with a non-symlink .git directory", src.Name))
				p.Sources = append(p.Sources, sp)
				continue
			}
			if _, e := os.Stat(filepath.Join(full, ".gitmodules")); e == nil {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has submodules", src.Name))
			}
			if entries, e := os.ReadDir(filepath.Join(full, ".git", "modules")); e == nil && len(entries) > 0 {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has absorbed submodules", src.Name))
			}
			if entries, e := os.ReadDir(filepath.Join(full, ".git", "worktrees")); e == nil && len(entries) > 0 {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has linked worktrees", src.Name))
			}
			if _, e := os.Stat(filepath.Join(full, ".git", "sequencer")); e == nil {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has operation in progress (sequencer)", src.Name))
			}
			if _, _, e := git(ctx, g, full, "rev-parse", "--verify", "HEAD^{commit}"); e != nil {
				p.Refusals = append(p.Refusals, fmt.Sprintf("git source %s has no commits", src.Name))
			}
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
				if !filepath.IsAbs(sp.URL) && !strings.Contains(sp.URL, "://") && !regexp.MustCompile(`^[^/]+@[^:]+:`).MatchString(sp.URL) {
					sp.URL = filepath.Clean(filepath.Join(full, sp.URL))
				}
			}
			branch, _ := g.CurrentBranch(ctx, full)
			sp.Branch = branch
			if branch == "" {
				sp.Mode = "pin"
			} else {
				def, defErr := g.DefaultBranch(ctx, full)
				if defErr != nil {
					sp.Mode = "work"
					p.Warnings = append(p.Warnings, fmt.Sprintf("git source %s has no origin default branch; using work mode", src.Name))
				} else if branch == def {
					sp.Mode = "follow"
				} else {
					sp.Mode = "work"
				}
			}
			status, _, _ := g.Run(ctx, full, map[string]string{"GIT_OPTIONAL_LOCKS": "0"}, "status", "--porcelain=v2", "--untracked-files=all")
			sp.Dirty = countLines(status)
			st, _, _ := git(ctx, g, full, "stash", "list")
			sp.Stashes = countLines(st)
			un, _, _ := git(ctx, g, full, "rev-list", "--branches", "HEAD", "--not", "--remotes=origin")
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
			if rel == x && d.IsDir() {
				return filepath.SkipDir
			}
		}
		if d.Name() == ".git" {
			p.Refusals = append(p.Refusals, fmt.Sprintf("undeclared nested git repository at %s", filepath.Dir(rel)))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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
	if p.RootBytes > 200*1024*1024 && !o.AllowLarge {
		p.Refusals = append(p.Refusals, "root files total more than 200 MB; pass --allow-large")
	}
	// Absorption and the journal's Tasktree.yml rename must be atomic.
	if err := requireSameFilesystem(abs, a.Home.Root); err != nil {
		p.Refusals = append(p.Refusals, err.Error())
	}
	sort.Strings(p.Symlinks)
	return p, nil
}

func requireSameFilesystem(a, b string) error {
	findExisting := func(path string) (os.FileInfo, error) {
		for {
			info, err := os.Stat(path)
			if err == nil {
				return info, nil
			}
			if !os.IsNotExist(err) || filepath.Dir(path) == path {
				return nil, err
			}
			path = filepath.Dir(path)
		}
	}
	ai, err := findExisting(a)
	if err != nil {
		return err
	}
	bi, err := findExisting(b)
	if err != nil {
		return err
	}
	as, aok := ai.Sys().(*syscall.Stat_t)
	bs, bok := bi.Sys().(*syscall.Stat_t)
	if !aok || !bok || as.Dev != bs.Dev {
		return fmt.Errorf("workspace and tasktree home are on different filesystems; atomic rename required")
	}
	return nil
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
			if rel == x && d.IsDir() {
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

func pendingImportForRoot(h home.Home, root string) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(h.Root, "imports"))
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(h.Root, "imports", entry.Name(), "journal.json"))
		if e != nil {
			continue
		}
		var j importJournal
		if json.Unmarshal(b, &j) == nil && j.Root == root {
			return j.ID, true
		}
	}
	return "", false
}

func writeImportJournal(dir string, j importJournal) error {
	j.Step++
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return fsx.AtomicWriteFile(filepath.Join(dir, "journal.json"), append(b, '\n'), 0o600)
}

func journalFromState(st *importState, step int) importJournal {
	j := importJournal{ID: st.id, Root: st.plan.Path, Handle: st.plan.Handle, Step: step, RootBefore: st.rootBefore, GeneratedOriginals: st.generatedOriginals, Starter: st.starter}
	absorbed := map[string]bool{}
	for _, s := range st.absorbed {
		absorbed[s.Name] = true
	}
	for _, s := range st.plan.Sources {
		if s.Type == "git" && s.Present {
			state := "pending"
			if absorbed[s.Name] {
				state = "absorbed"
			}
			j.Repos = append(j.Repos, importJournalRepo{Name: s.Name, Path: s.Path, State: state, Before: snapshotJSON(st.before[s.Name])})
		}
	}
	return j
}

func (st *importState) saveJournal(step int) error {
	return writeImportJournal(st.journalDir, journalFromState(st, step))
}

func testImportFail(step int) error {
	v := os.Getenv("TASKTREE_TEST_IMPORT_FAIL")
	if v == strconv.Itoa(step) || v == fmt.Sprintf("step-%d", step) {
		return fmt.Errorf("test-only injected import failure at step %d", step)
	}
	return nil
}

func (a App) Import(ctx context.Context, root string, o ImportOptions) (res ImportResult, err error) {
	unlock, err := lockImport(ctx, a.Home)
	if err != nil {
		return res, err
	}
	defer unlock()
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
	task, doc, name, machine, err := a.buildImportRecord(p, o, id)
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
	st.journalDir = filepath.Join(a.Home.Root, "imports", id)
	if err = os.MkdirAll(st.journalDir, 0o700); err != nil {
		return res, err
	}
	if err = st.saveJournal(1); err != nil {
		return res, err
	}
	// Preserve the exact V1 manifest by atomic rename; rollback/recovery moves it back.
	if err = os.Rename(filepath.Join(p.Path, "Tasktree.yml"), filepath.Join(st.journalDir, "Tasktree.yml")); err != nil {
		return res, err
	}
	started := true
	defer func() {
		if err != nil && started && os.Getenv("TASKTREE_TEST_IMPORT_KILL") == "" {
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
	_, _, err = a.Git.Run(ctx, p.Path, rootEnv(a.Home, id, p.Path), "init", "--template=")
	if err != nil {
		return
	}
	_, _, _ = a.Git.Run(ctx, "", nil, "config", "--file", filepath.Join(a.Home.ClonePath(id), "config"), "--unset-all", "core.worktree")
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
		if err = recheckAbsorbPreflight(p.Path, s); err != nil {
			return
		}
		_, _, err = rootGitInit(ctx, a.Git, a.Home, id, p.Path, "-c", "protocol.file.allow=always", "submodule", "add", "--name", s.Name, "--force", s.URL, s.Path)
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
		if err = st.saveJournal(3); err != nil {
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
	if _, e := os.Lstat(filepath.Join(p.Path, "AGENTS.md")); os.IsNotExist(e) {
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
	if err = st.saveJournal(4); err != nil {
		return
	}
	if err = testImportFail(4); err != nil {
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
	env := rootEnv(a.Home, id, p.Path)
	env["GIT_AUTHOR_NAME"], env["GIT_COMMITTER_NAME"] = name, name
	env["GIT_AUTHOR_EMAIL"], env["GIT_COMMITTER_EMAIL"] = name+"@"+machine, name+"@"+machine
	_, _, err = a.Git.Run(ctx, p.Path, env, "-c", "commit.gpgsign=false", "commit", "--no-verify", "-m", fmt.Sprintf("tasktree(%s): import from V1", id))
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
	if err = a.Hub.Create(ctx, doc, "import from V1", name, machine); err != nil {
		return
	}
	// Hub.Create is authoritative. From here recovery completes local state; never
	// roll the live workspace back underneath the committed record.
	started = false
	finalPath := p.Path
	if err = a.Home.UpdateLocal(func(s *home.LocalState) error { s.Checkouts[id] = home.Checkout{Path: finalPath}; return nil }); err != nil {
		err = fmt.Errorf("record created but local.yml update failed: %w; run tt2 import --recover %s", err, p.Path)
		return
	}
	if err = st.saveJournal(9); err != nil {
		return
	}
	if o.Move && p.Target != p.Path {
		var e error
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
				renameErr := os.Rename(p.Target, p.Path)
				for _, x := range st.absorbed {
					_ = rewriteSubmodule(a.Home, id, p.Path, module{Name: x.Name, Path: x.Path})
				}
				if renameErr != nil {
					err = fmt.Errorf("rewrite after move: %v; rename back %s to %s: %w", e, p.Target, p.Path, renameErr)
				} else {
					err = e
				}
				return
			}
		}
		_, _, _ = a.Git.Run(ctx, "", nil, "config", "--file", filepath.Join(a.Home.ClonePath(id), "config"), "--unset-all", "core.worktree")
		oldPath := st.plan.Path
		st.plan.Path = p.Target
		if e = a.verifyImport(ctx, st); e != nil {
			renameErr := os.Rename(p.Target, p.Path)
			st.plan.Path = oldPath
			for _, x := range st.absorbed {
				_ = rewriteSubmodule(a.Home, id, p.Path, module{Name: x.Name, Path: x.Path})
			}
			if renameErr != nil {
				err = fmt.Errorf("verify after move: %v; rename back %s to %s: %w", e, p.Target, p.Path, renameErr)
			} else {
				err = fmt.Errorf("verify after move: %w", e)
			}
			return
		}
		if e = a.Home.UpdateLocal(func(ls *home.LocalState) error { ls.Checkouts[id] = home.Checkout{Path: p.Target}; return nil }); e != nil {
			var renameErr error
			if moved {
				renameErr = os.Rename(p.Target, p.Path)
			}
			for _, x := range st.absorbed {
				_ = rewriteSubmodule(a.Home, id, p.Path, module{Name: x.Name, Path: x.Path})
			}
			if renameErr != nil {
				err = fmt.Errorf("update local.yml: %v; rename back %s to %s: %w", e, p.Target, p.Path, renameErr)
			} else {
				err = e
			}
			return
		}
		finalPath = p.Target
	}
	if err = os.RemoveAll(st.journalDir); err != nil {
		return
	}
	res.Task = task
	res.Path = finalPath
	return res, nil
}

func (a App) buildImportRecord(p ImportPlan, o ImportOptions, id string) (record.Task, *record.Document, string, string, error) {
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return record.Task{}, nil, "", "", err
	}
	name, machine := cfg.Author, cfg.Machine
	if name == "" {
		name = "tasktree"
	}
	if machine == "" {
		machine = "localhost"
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
	task := record.Task{ID: id, Handle: p.Handle, Title: title, Status: o.Status, Labels: labels, Refs: importRefs(p.spec.Metadata.Annotations), CreatedAt: created, UpdatedAt: a.Now().UTC(), Description: p.spec.Metadata.Description}
	doc, err := record.New(task)
	if err != nil {
		return task, nil, name, machine, err
	}
	entryID, err := record.NewID()
	if err != nil {
		return task, nil, name, machine, err
	}
	body := "Imported from V1\n\n```yaml\n" + strings.TrimRight(string(p.specBytes), "\n") + "\n```"
	if len(p.Warnings) > 0 {
		body += "\n\nWarnings:\n- " + strings.Join(p.Warnings, "\n- ")
	}
	if err = doc.Append(record.Entry{ID: entryID, Timestamp: a.Now().UTC(), Kind: "note", Author: name, Machine: machine, Body: body}); err != nil {
		return task, nil, name, machine, err
	}
	return task, doc, name, machine, nil
}

func lockImport(ctx context.Context, h home.Home) (func(), error) {
	if err := os.MkdirAll(h.Root, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(h.Root, "imports.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func recheckAbsorbPreflight(root string, s ImportSourcePlan) error {
	gitDir := filepath.Join(root, s.Path, ".git")
	for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "index.lock", "sequencer"} {
		if _, err := os.Lstat(filepath.Join(gitDir, marker)); err == nil {
			return fmt.Errorf("git source %s has operation in progress (%s)", s.Name, marker)
		}
	}
	return nil
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
	mods, err := modules(st.plan.Path)
	if err != nil {
		return err
	}
	if len(mods) != len(st.absorbed) {
		return fmt.Errorf(".gitmodules has %d sections, expected %d", len(mods), len(st.absorbed))
	}
	byName := map[string]module{}
	for _, m := range mods {
		byName[m.Name] = m
	}
	for _, s := range st.absorbed {
		m, ok := byName[s.Name]
		if !ok || m.Path != s.Path || m.URL != s.URL || m.Mode != s.Mode {
			return fmt.Errorf(".gitmodules entry %s does not match import plan", s.Name)
		}
		entry, _, e := rootGit(ctx, a.Git, a.Home, st.id, st.plan.Path, "ls-files", "--stage", "--", s.Path)
		if e != nil || !strings.HasPrefix(strings.TrimSpace(entry), "160000 "+strings.TrimSpace(st.before[s.Name].Head)+" ") {
			return fmt.Errorf("gitlink %s does not match before HEAD", s.Name)
		}
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
	for path := range after {
		if _, existed := st.rootBefore[path]; existed {
			continue
		}
		if path == ".gitmodules" || path == ".tasktree" || strings.HasPrefix(path, ".tasktree"+string(filepath.Separator)) || (path == "AGENTS.md" && st.starter) {
			continue
		}
		return fmt.Errorf("unexpected new root path %s during import", path)
	}
	return nil
}

func (a App) rollbackImport(ctx context.Context, st *importState) error {
	ctx = context.WithoutCancel(ctx)
	var problems []string
	// The orphan branch may already have reached the hub even when a later step
	// failed before record creation.
	if a.Hub.IsSetup() {
		_, _, _ = a.Git.Run(ctx, a.Home.GitPath(), nil, "update-ref", "-d", "refs/heads/task/"+st.id)
	}
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
	journalManifest := filepath.Join(st.journalDir, "Tasktree.yml")
	if _, statErr := os.Lstat(journalManifest); statErr == nil {
		_ = os.Remove(tasktreePath)
		if e := os.Rename(journalManifest, tasktreePath); e != nil {
			problems = append(problems, "restore Tasktree.yml: "+e.Error())
		}
	} else if _, existingErr := os.Lstat(tasktreePath); existingErr != nil {
		problems = append(problems, fmt.Sprintf("cannot locate preserved Tasktree.yml at %s", journalManifest))
	}
	clone := a.Home.ClonePath(st.id)
	if len(problems) == 0 {
		if e := os.RemoveAll(clone); e != nil {
			problems = append(problems, e.Error())
		}
	} else if _, e := os.Lstat(clone); e == nil {
		quarantine := filepath.Join(a.Home.Root, "imports", st.id, "quarantine-clone")
		if mkErr := os.MkdirAll(filepath.Dir(quarantine), 0o755); mkErr != nil {
			problems = append(problems, "create quarantine: "+mkErr.Error())
		} else if renameErr := os.Rename(clone, quarantine); renameErr != nil {
			problems = append(problems, "quarantine clone: "+renameErr.Error())
		} else {
			problems = append(problems, fmt.Sprintf("clone quarantined at %s; recover each repository by moving %s/modules/<name> to %s/<repo-path>/.git", quarantine, quarantine, st.plan.Path))
		}
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
	if st.journalDir != "" {
		if e := os.RemoveAll(st.journalDir); e != nil {
			return e
		}
	}
	return nil
}

// RecoverImport completes local state after record creation, or restores the
// exact V1 workspace described by its durable journal.
func (a App) RecoverImport(ctx context.Context, root string) (string, error) {
	unlock, err := lockImport(ctx, a.Home)
	if err != nil {
		return "", err
	}
	defer unlock()
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	base := filepath.Join(a.Home.Root, "imports")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("no import journal for %s", abs)
	}
	var j importJournal
	var dir string
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(base, entry.Name(), "journal.json"))
		if e != nil {
			continue
		}
		var candidate importJournal
		if json.Unmarshal(b, &candidate) == nil && candidate.Root == abs {
			j, dir = candidate, filepath.Join(base, entry.Name())
			break
		}
	}
	if dir == "" {
		return "", fmt.Errorf("no import journal for %s", abs)
	}
	if _, e := a.Hub.Read(j.ID); e == nil {
		if e = a.Home.UpdateLocal(func(s *home.LocalState) error { s.Checkouts[j.ID] = home.Checkout{Path: abs}; return nil }); e != nil {
			return "", e
		}
		if e = os.RemoveAll(dir); e != nil {
			return "", e
		}
		return fmt.Sprintf("repaired local.yml for %s (%s)", j.Handle, j.ID), nil
	}
	st := &importState{id: j.ID, journalDir: dir, rootBefore: j.RootBefore, generatedOriginals: j.GeneratedOriginals, before: map[string]repoSnapshot{}}
	st.plan = ImportPlan{Path: j.Root, Handle: j.Handle}
	for _, r := range j.Repos {
		s := ImportSourcePlan{Name: r.Name, Path: r.Path, Type: "git", Present: true}
		st.plan.Sources = append(st.plan.Sources, s)
		st.before[r.Name] = r.Before.snapshot()
		moduleDir := filepath.Join(a.Home.ClonePath(j.ID), "modules", r.Name)
		if _, e := os.Lstat(moduleDir); e == nil || r.State == "absorbed" {
			st.absorbed = append(st.absorbed, s)
		}
	}
	st.starter = j.Starter
	if err = a.rollbackImport(context.WithoutCancel(ctx), st); err != nil {
		return "", err
	}
	return fmt.Sprintf("restored V1 workspace exactly at %s", abs), nil
}

// ReadV1Registry reads only the ordered path entries needed by import --all.
func ReadV1Registry(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registry struct {
		Version   int `toml:"version"`
		Tasktrees []struct {
			Path string `toml:"path"`
		} `toml:"tasktrees"`
	}
	if err := toml.Unmarshal(b, &registry); err != nil {
		return nil, fmt.Errorf("parse V1 registry: %w", err)
	}
	out := make([]string, 0, len(registry.Tasktrees))
	for _, entry := range registry.Tasktrees {
		if entry.Path != "" {
			out = append(out, entry.Path)
		}
	}
	return out, nil
}
