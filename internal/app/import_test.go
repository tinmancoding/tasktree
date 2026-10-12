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

func TestRollbackQuarantinesCloneWhenRepositoryCannotBeRestored(t *testing.T) {
	base := t.TempDir()
	a := New(home.Home{Root: filepath.Join(base, "home")}, nil)
	root := filepath.Join(base, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "abc123"
	clone := a.Home.ClonePath(id)
	history := filepath.Join(clone, "modules", "repo", "objects", "only-history")
	if err := os.MkdirAll(filepath.Dir(history), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, []byte("irreplaceable"), 0o600); err != nil {
		t.Fatal(err)
	}
	journalDir := filepath.Join(a.Home.Root, "imports", id)
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Force the move-back failure by making the destination parent absent while
	// keeping another module-like directory as the clone's only history.
	st := &importState{id: id, journalDir: journalDir, plan: ImportPlan{Path: root, Sources: []ImportSourcePlan{{Name: "missing", Path: "missing", Type: "git", Present: true}}}, absorbed: []ImportSourcePlan{{Name: "missing", Path: "missing", Type: "git", Present: true}}, before: map[string]repoSnapshot{"missing": {}}, rootBefore: map[string]rootEntry{}}
	err := a.rollbackImport(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("rollback error = %v", err)
	}
	quarantined := filepath.Join(journalDir, "quarantine-clone", "modules", "repo", "objects", "only-history")
	if b, readErr := os.ReadFile(quarantined); readErr != nil || string(b) != "irreplaceable" {
		t.Fatalf("history was not quarantined: %q %v", b, readErr)
	}
}

func TestInterruptedImportRequiresRecoveryAndRestoresExactly(t *testing.T) {
	a, root := importFixture(t)
	before := treeDigest(t, root)
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "3")
	t.Setenv("TASKTREE_TEST_IMPORT_KILL", "1")
	if _, err := a.Import(context.Background(), root, ImportOptions{}); err == nil {
		t.Fatal("expected simulated interruption")
	}
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "")
	t.Setenv("TASKTREE_TEST_IMPORT_KILL", "")
	if _, err := a.Import(context.Background(), root, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "incomplete import") {
		t.Fatalf("rerun error = %v", err)
	}
	message, err := a.RecoverImport(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "restored V1 workspace exactly") {
		t.Fatalf("message = %q", message)
	}
	if got := treeDigest(t, root); got != before {
		t.Fatalf("recovery changed fixture: %s != %s", got, before)
	}
}

func TestImportMovePreservesRepositorySnapshot(t *testing.T) {
	a, root := importFixture(t)
	before, err := snapshotRepo(context.Background(), a, filepath.Join(root, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.Import(context.Background(), root, ImportOptions{Move: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Path == root {
		t.Fatal("workspace was not moved")
	}
	after, err := snapshotRepo(context.Background(), a, filepath.Join(r.Path, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotsEqual(before, after, true); err != nil {
		t.Fatalf("moved snapshot differs: %v", err)
	}
	if _, _, err := a.Git.Run(context.Background(), "", nil, "config", "--file", filepath.Join(a.Home.ClonePath(r.Task.ID), "config"), "--get", "core.worktree"); err == nil {
		t.Fatal("root clone retained core.worktree")
	}
}

func TestImportUsesDeclaredNameForNestedPath(t *testing.T) {
	a, root := importFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "repo"), filepath.Join(root, "a", "b")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "Tasktree.yml"))
	if err != nil {
		t.Fatal(err)
	}
	spec := strings.Replace(string(b), "- name: repo\n      type: git", "- name: logical-name\n      path: a/b\n      type: git", 1)
	if err := os.WriteFile(filepath.Join(root, "Tasktree.yml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := a.Import(context.Background(), root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gm := filepath.Join(root, ".gitmodules")
	if got := importGit(t, root, "config", "-f", gm, "--get", "submodule.logical-name.path"); got != "a/b" {
		t.Fatalf("path = %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.Home.ClonePath(r.Task.ID), "modules", "logical-name", "HEAD")); err != nil {
		t.Fatalf("named module missing: %v", err)
	}
}

func TestImportPreflightRefusesUnsafeRepositories(t *testing.T) {
	t.Run("symlink source", func(t *testing.T) {
		a, root := importFixture(t)
		if err := os.Rename(filepath.Join(root, "repo"), filepath.Join(root, "actual")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("actual", filepath.Join(root, "repo")); err != nil {
			t.Fatal(err)
		}
		p, err := a.PlanImport(context.Background(), root, ImportOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(p.Refusals, "\n"), "is a symlink") {
			t.Fatalf("refusals = %v", p.Refusals)
		}
	})
	t.Run("submodules", func(t *testing.T) {
		a, root := importFixture(t)
		if err := os.WriteFile(filepath.Join(root, "repo", ".gitmodules"), []byte("[submodule \\\"x\\\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := a.PlanImport(context.Background(), root, ImportOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(p.Refusals, "\n"), "has submodules") {
			t.Fatalf("refusals = %v", p.Refusals)
		}
	})
	t.Run("linked worktrees", func(t *testing.T) {
		a, root := importFixture(t)
		if err := os.MkdirAll(filepath.Join(root, "repo", ".git", "worktrees", "other"), 0o755); err != nil {
			t.Fatal(err)
		}
		p, err := a.PlanImport(context.Background(), root, ImportOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(p.Refusals, "\n"), "linked worktrees") {
			t.Fatalf("refusals = %v", p.Refusals)
		}
	})
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
