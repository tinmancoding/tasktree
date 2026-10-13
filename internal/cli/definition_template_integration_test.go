package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func setupDefinitionTest(t *testing.T) (home, tasks, origin string) {
	t.Helper()
	base := t.TempDir()
	home = filepath.Join(base, "home")
	tasks = filepath.Join(base, "tasks")
	src := filepath.Join(base, "source")
	origin = filepath.Join(base, "origin.git")
	t.Setenv("TASKTREE_HOME", home)
	t.Setenv("TASKTREE_CACHE_DIR", filepath.Join(base, "cache"))
	t.Setenv("TASKTREE_V1_ALIASES_FILE", filepath.Join(base, "missing"))
	if out, _, err := run(t, "setup", "--tasks-root", tasks, "--author", "test", "--machine", "box"); err != nil {
		t.Fatalf("setup: %v (%s)", err, out)
	}
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, src, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, src, "add", "README.md")
	gitRun(t, src, "commit", "-m", "initial")
	gitRun(t, "", "clone", "--bare", src, origin)
	return
}
func newID(t *testing.T, title string) string {
	t.Helper()
	out, _, err := run(t, "new", title)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		t.Fatalf("new output %q", out)
	}
	return parts[1]
}

func TestDefinitionCommandsWithoutCheckout(t *testing.T) {
	home, tasks, origin := setupDefinitionTest(t)
	id := newID(t, "Defined task")
	if _, _, err := run(t, "add", origin, "-t", id, "--branch", "topic"); err != nil {
		t.Fatal(err)
	}
	hub := filepath.Join(home, "git")
	tree := gitRun(t, hub, "ls-tree", "-r", "task/"+id)
	for _, want := range []string{".tasktree/config.yml", ".tasktree/.gitignore", "AGENTS.md", "\torigin"} {
		if !strings.Contains(tree, want) {
			t.Fatalf("tree missing %s:\n%s", want, tree)
		}
	}
	gm := gitRun(t, hub, "show", "task/"+id+":.gitmodules")
	if !strings.Contains(gm, "branch = topic") || !strings.Contains(gm, "tasktree-mode = work") {
		t.Fatal(gm)
	}
	out, _, err := run(t, "info", "-t", id)
	if err != nil || !strings.Contains(out, "defined (not checked out here)") {
		t.Fatalf("info %v %s", err, out)
	}
	listed, _, err := run(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if json.Unmarshal([]byte(listed), &rows) != nil || rows[0]["tasktree"] != "defined" {
		t.Fatal(listed)
	}
	opened, _, err := run(t, "open", id)
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(opened)
	if root != filepath.Join(tasks, "defined-task") {
		t.Fatal(root)
	}
	if branch := gitRun(t, filepath.Join(root, "origin"), "branch", "--show-current"); branch != "topic" {
		t.Fatalf("branch=%s", branch)
	}
}
func TestDefinitionModeAndRemoveWithoutCheckout(t *testing.T) {
	home, _, origin := setupDefinitionTest(t)
	id := newID(t, "Edit definition")
	if _, _, err := run(t, "add", origin, "-t", id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "mode", "origin", "pin", "-t", id); err != nil {
		t.Fatal(err)
	}
	gm := gitRun(t, filepath.Join(home, "git"), "show", "task/"+id+":.gitmodules")
	if strings.Contains(gm, "branch =") || !strings.Contains(gm, "tasktree-mode = pin") {
		t.Fatal(gm)
	}
	if _, _, err := run(t, "mode", "origin", "work", "-t", id); err == nil || cli.Code(err) != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if _, _, err := run(t, "remove", "origin", "-t", id); err != nil {
		t.Fatal(err)
	}
	tree := gitRun(t, filepath.Join(home, "git"), "ls-tree", "-r", "task/"+id)
	if strings.Contains(tree, "\torigin") || strings.Contains(tree, ".gitmodules") {
		t.Fatal(tree)
	}
}
func TestTemplateRecordFailureRemovesDefinition(t *testing.T) {
	home, _, origin := setupDefinitionTest(t)
	t.Setenv("TASKTREE_TEST_RECORD_CREATE_FAIL", "1")
	if _, _, err := run(t, "new", "--template", "feature", "feature_name=rollback", "repo="+origin); err == nil {
		t.Fatal("expected injected failure")
	}
	refs := gitRun(t, filepath.Join(home, "git"), "for-each-ref", "--format=%(refname)", "refs/heads/task")
	if refs != "" {
		t.Fatalf("orphaned definition: %s", refs)
	}
	entries, err := os.ReadDir(filepath.Join(home, "git", "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			t.Fatalf("record was written: %s", entry.Name())
		}
	}
}

func TestBuiltinTemplateCreatesRecordAndDefinition(t *testing.T) {
	home, _, origin := setupDefinitionTest(t)
	out, _, err := run(t, "new", "--template", "bugfix", "ticket_number=BUG-123", "repo="+origin)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(out)[1]
	record := string(mustRead(t, filepath.Join(home, "git", "tasks", id+".md")))
	if !strings.Contains(record, "Created from template bugfix") || !strings.Contains(record, "ticket_number=BUG-123") {
		t.Fatal(record)
	}
	gm := gitRun(t, filepath.Join(home, "git"), "show", "task/"+id+":.gitmodules")
	if !strings.Contains(gm, "branch = fix/BUG-123") || !strings.Contains(gm, "tasktree-mode = work") {
		t.Fatal(gm)
	}
	list, _, err := run(t, "template", "list")
	if err != nil || !regexp.MustCompile(`(?m)^bugfix\s+builtin\s`).MatchString(list) {
		t.Fatalf("%v %s", err, list)
	}
}
