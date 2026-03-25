package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
)

func TestResolveTasktreeRootFindsParent(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "api", "src", "components")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	metadataPath := filepath.Join(root, domain.MetadataFileName)
	if err := os.WriteFile(metadataPath, []byte("version = 1\nname = \"demo\"\ncreated_at = 2026-03-25T12:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	resolved, err := fsx.ResolveTasktreeRoot(nested)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if resolved != root {
		t.Fatalf("resolved root = %q, want %q", resolved, root)
	}
}

func TestResolveTasktreeRootReturnsTypedError(t *testing.T) {
	_, err := fsx.ResolveTasktreeRoot(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(domain.NotInTasktreeError); !ok {
		t.Fatalf("expected NotInTasktreeError, got %T", err)
	}
}

func TestIsWithin(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "api")
	outside := filepath.Join(filepath.Dir(root), "outside")

	ok, err := fsx.IsWithin(root, inside)
	if err != nil {
		t.Fatalf("is within inside: %v", err)
	}
	if !ok {
		t.Fatal("expected inside path to be within root")
	}

	ok, err = fsx.IsWithin(root, outside)
	if err != nil {
		t.Fatalf("is within outside: %v", err)
	}
	if ok {
		t.Fatal("expected outside path to be rejected")
	}
}
