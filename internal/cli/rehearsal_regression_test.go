package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFallbackIdentityAndRemoveRollback(t *testing.T) {
	gitHome := filepath.Join(t.TempDir(), "git-home")
	if err := os.MkdirAll(gitHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitHome, ".gitconfig"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", gitHome)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	f := newCloseFixture(t)
	rootPath := filepath.Join(f.tasks, "fallback")
	out, errOut, err := run(t, "new", "Fallback", "--open", "--path", rootPath)
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(out)
	fields := strings.Fields(errOut)
	if len(fields) != 2 {
		t.Fatalf("new --open identity = %q", errOut)
	}
	id := fields[1]
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "add", f.upstream, "--name", "api"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "checkpoint"), []byte("recorded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "commit"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "mode", "api", "pin"); err != nil {
		t.Fatal(err)
	}

	gitmodulesBefore := mustRead(t, filepath.Join(root, ".gitmodules"))
	indexPath := filepath.Join(f.home, "clones", id+".git", "index")
	indexBefore := mustRead(t, indexPath)
	modulePath := filepath.Join(f.home, "clones", id+".git", "modules", "api")
	t.Setenv("TASKTREE_TEST_REMOVE_COMMIT_FAIL", "1")
	if _, _, err = run(t, "remove", "api"); err == nil || !strings.Contains(err.Error(), "test-only injected remove commit failure") {
		t.Fatalf("forced remove error = %v", err)
	}
	if _, err = os.Stat(filepath.Join(root, "api")); err != nil {
		t.Fatalf("working repository was removed: %v", err)
	}
	if _, err = os.Stat(modulePath); err != nil {
		t.Fatalf("module git dir was removed: %v", err)
	}
	if got := mustRead(t, filepath.Join(root, ".gitmodules")); string(got) != string(gitmodulesBefore) {
		t.Fatalf(".gitmodules changed after failed remove:\n%s", got)
	}
	if got := mustRead(t, indexPath); string(got) != string(indexBefore) {
		t.Fatal("index changed after failed remove")
	}
	t.Setenv("TASKTREE_TEST_REMOVE_COMMIT_FAIL", "")
	if _, _, err = run(t, "remove", "api"); err != nil {
		t.Fatal(err)
	}
	if committed := gitRun(t, filepath.Join(f.home, "git"), "show", "task/"+id+":.gitmodules"); strings.Contains(committed, "submodule \"api\"") {
		t.Fatalf("removed submodule remains in committed .gitmodules:\n%s", committed)
	}
	if _, _, err = run(t, "mark", "done"); err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(f.base); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(f.base, "legacy")
	if err = os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, legacy, "clone", f.upstream, "repo")
	manifest := "apiVersion: tasktree.dev/v1\nkind: Tasktree\nmetadata:\n  name: Imported fallback\nspec:\n  sources:\n    - name: repo\n      type: git\n"
	if err = os.WriteFile(filepath.Join(legacy, "Tasktree.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "import", legacy); err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "remove", "repo"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(legacy, "after-remove"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "commit"); err != nil {
		t.Fatal(err)
	}

	for _, repo := range []string{filepath.Join(f.home, "git"), filepath.Join(f.home, "clones", id+".git")} {
		cmd := exec.Command("git", "config", "--local", "--get", "user.name")
		cmd.Dir = repo
		if output, configErr := cmd.CombinedOutput(); configErr == nil {
			t.Fatalf("tool wrote local git identity in %s: %s", repo, output)
		}
	}
	if got := gitRun(t, filepath.Join(f.home, "git"), "log", "--format=%an <%ae>"); !strings.Contains(got, "test <test@box>") {
		t.Fatalf("hub commits did not use fallback identity: %q", got)
	}
}

func TestImportAllSkipsStaleV1RegistryEntries(t *testing.T) {
	f := newCloseFixture(t)
	makeWorkspace := func(name string) string {
		path := filepath.Join(f.base, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := "apiVersion: tasktree.dev/v1\nkind: Tasktree\nmetadata:\n  name: " + name + "\nspec:\n  sources: []\n"
		if err := os.WriteFile(filepath.Join(path, "Tasktree.yml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	one := makeWorkspace("one")
	two := makeWorkspace("two")
	missing := filepath.Join(f.base, "missing")
	registry := filepath.Join(f.base, "registry.toml")
	contents := "version = 1\n[[tasktrees]]\npath = '" + one + "'\n[[tasktrees]]\npath = '" + missing + "'\n[[tasktrees]]\npath = '" + two + "'\n"
	if err := os.WriteFile(registry, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKTREE_V1_REGISTRY", registry)
	out, errOut, err := run(t, "import", "--all")
	if err != nil {
		t.Fatalf("import --all: %v\nstderr: %s\nstdout: %s", err, errOut, out)
	}
	if !strings.Contains(out, missing+": skipped: path missing") || !strings.Contains(out, "summary: imported=2 refused=0 failed=0 skipped=1") {
		t.Fatalf("unexpected import --all output:\n%s", out)
	}
}
