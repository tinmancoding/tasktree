package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/fsx"
)

func TestAtomicWriteFileReplacesContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.toml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := fsx.AtomicWriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(contents) != "new" {
		t.Fatalf("contents = %q, want %q", contents, "new")
	}
}
