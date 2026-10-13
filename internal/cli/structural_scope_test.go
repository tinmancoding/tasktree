package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Structural commands commit only what the tool changed; unrelated
// uncommitted root files stay uncommitted (D13′, D14).
func TestStructuralCommitsDoNotSweepUserFiles(t *testing.T) {
	f := newCloseFixture(t)
	_, id, root := createAndOpen(t, "Scope", filepath.Join(f.tasks, "scope"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := [][]string{
		{"add", f.upstream, "--name", "api"},
		{"mode", "api", "pin"},
		{"bootstrap", "add", "hello", "true"},
	}
	for _, s := range steps {
		if _, _, err := run(t, s...); err != nil {
			t.Fatalf("%v: %v", s, err)
		}
		tree := gitRun(t, filepath.Join(f.home, "git"), "ls-tree", "-r", "--name-only", "task/"+id)
		if strings.Contains(tree, "scratch.md") {
			t.Fatalf("%v swept scratch.md into the task branch:\n%s", s, tree)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "scratch.md")); err != nil {
		t.Fatalf("scratch.md must remain in the working tree: %v", err)
	}
}
