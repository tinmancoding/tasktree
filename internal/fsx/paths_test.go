package fsx_test

import (
	"errors"
	"github.com/tinmancoding/tasktree/v2/internal/fsx"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTasktreeRootFindsParent(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "api", "src")
	if err := os.MkdirAll(filepath.Join(root, ".tasktree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tasktree", "config.yml"), []byte("taskId: abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := fsx.ResolveTasktreeRoot(nested)
	if err != nil || got != root {
		t.Fatalf("got %q, %v", got, err)
	}
}
func TestResolveTasktreeRootReturnsTypedError(t *testing.T) {
	_, err := fsx.ResolveTasktreeRoot(t.TempDir())
	var target fsx.NotInTasktreeError
	if !errors.As(err, &target) {
		t.Fatalf("got %T", err)
	}
}
func TestIsWithin(t *testing.T) {
	root := t.TempDir()
	ok, err := fsx.IsWithin(root, filepath.Join(root, "api"))
	if err != nil || !ok {
		t.Fatal(err)
	}
	ok, err = fsx.IsWithin(root, filepath.Join(filepath.Dir(root), "outside"))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
