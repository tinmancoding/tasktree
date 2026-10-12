package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The machine config at ~/.tasktree/config.yml shares the marker's relative
// path; root discovery must skip it instead of failing.
func TestDiscoverRootSkipsMachineConfig(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, ".tasktree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, ".tasktree", "config.yml"), []byte("tasksRoot: /x\nauthor: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(base, "Developer", "repo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DiscoverRoot(nested); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want not-in-tasktree, got %v", err)
	}

	root := filepath.Join(base, "Developer", "tasks", "t1")
	if err := os.MkdirAll(filepath.Join(root, ".tasktree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tasktree", "config.yml"), []byte("apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, c, err := DiscoverRoot(filepath.Join(root))
	if err != nil || got != root || c.TaskID != "abc123" {
		t.Fatalf("got %q %+v %v", got, c, err)
	}
}
