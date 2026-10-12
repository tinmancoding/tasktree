package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/home"
)

func importGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, b)
	}
	return strings.TrimSpace(string(b))
}

func importFixture(t *testing.T) (App, string) {
	t.Helper()
	base := t.TempDir()
	h := home.Home{Root: filepath.Join(base, "home")}
	a := New(h, nil)
	if _, err := a.Setup(context.Background(), SetupOptions{TasksRoot: filepath.Join(base, "tasks"), TasksRootSet: true, Author: "test", AuthorSet: true, Machine: "test", MachineSet: true}); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(base, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	importGit(t, seed, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "tracked"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, ".gitignore"), []byte("*.ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	importGit(t, seed, "add", ".")
	importGit(t, seed, "commit", "-m", "base")
	up := filepath.Join(base, "upstream.git")
	importGit(t, base, "clone", "--bare", seed, up)
	root := filepath.Join(base, "legacy")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	importGit(t, base, "clone", up, repo)
	importGit(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "x.ignored"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "stashme"), []byte("stash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	importGit(t, repo, "add", "stashme")
	importGit(t, repo, "stash", "push", "--keep-index")
	importGit(t, repo, "reset")
	if err := os.MkdirAll(filepath.Join(root, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "research", "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("research/notes.md", filepath.Join(root, "local-link")); err != nil {
		t.Fatal(err)
	}
	spec := `apiVersion: tasktree.dev/v1
kind: Tasktree
metadata:
  name: Imported task
  description: legacy description
  annotations:
    jira.ticket: ABC-12
    docs: https://example.com/docs
spec:
  sources:
    - name: repo
      type: git
    - name: local-link
      type: local
`
	if err := os.WriteFile(filepath.Join(root, "Tasktree.yml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return a, root
}

func treeDigest(t *testing.T, root string) string {
	t.Helper()
	var rows []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			rows = append(rows, rel+"/")
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			x, e := os.Readlink(path)
			if e != nil {
				return e
			}
			rows = append(rows, rel+"->"+x)
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		rows = append(rows, fmt.Sprintf("%s:%x", rel, sha256.Sum256(b)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rows)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(rows, "\n"))))
}

func TestImportDryRunRollbackAndSuccess(t *testing.T) {
	a, root := importFixture(t)
	ctx := context.Background()
	before := treeDigest(t, root)
	p, err := a.PlanImport(ctx, root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Sources[0].Mode != "work" || p.Sources[0].Dirty < 2 || p.Sources[0].Stashes != 1 || len(p.Symlinks) != 1 {
		t.Fatalf("incomplete plan: %+v", p)
	}
	if got := treeDigest(t, root); got != before {
		t.Fatal("dry-run planning changed fixture")
	}
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "5")
	if _, err = a.Import(ctx, root, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "rollback restored") {
		t.Fatalf("rollback error = %v", err)
	}
	if got := treeDigest(t, root); got != before {
		t.Fatalf("rollback changed fixture: %s != %s", got, before)
	}
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "")
	r, err := a.Import(ctx, root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Task.Title != "Imported task" || len(r.Task.Refs) != 2 {
		t.Fatalf("record = %+v", r.Task)
	}
	if info, err := os.Stat(filepath.Join(root, "repo", ".git")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("absorbed .git: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(root, "Tasktree.yml")); !os.IsNotExist(err) {
		t.Fatalf("Tasktree.yml remains: %v", err)
	}
	if got := importGit(t, filepath.Join(root, "repo"), "branch", "--show-current"); got != "feature" {
		t.Fatalf("branch = %q", got)
	}
	if got := importGit(t, filepath.Join(root, "repo"), "stash", "list"); got == "" {
		t.Fatal("stash lost")
	}
	if c := importGit(t, a.Home.GitPath(), "ls-tree", "task/"+r.Task.ID, "repo"); !strings.Contains(c, "commit") {
		t.Fatalf("repo is not gitlink: %s", c)
	}
}

func TestReadV1RegistryOrder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "registry.toml")
	if err := os.WriteFile(p, []byte("version = 1\n[[tasktrees]]\npath = '/one'\nname = 'one'\nadded_at = '2025-01-02T03:04:05Z'\n[[tasktrees]]\npath = '/two'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadV1Registry(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "/one,/two" {
		t.Fatalf("paths = %v", got)
	}
}
