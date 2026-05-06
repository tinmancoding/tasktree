package materialize_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/materialize"
)

func TestStaticWritesContentWithDefaultMode(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "config.json")

	spec := &domain.StaticSourceSpec{
		Content: `{"debug": true}`,
	}
	if err := materialize.Static(destPath, spec); err != nil {
		t.Fatalf("Static: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != spec.Content {
		t.Fatalf("content = %q, want %q", string(got), spec.Content)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 0644", info.Mode().Perm())
	}
}

func TestStaticWritesContentWithCustomMode(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "run.sh")

	spec := &domain.StaticSourceSpec{
		Content: "#!/bin/sh\necho hello\n",
		Mode:    "0755",
	}
	if err := materialize.Static(destPath, spec); err != nil {
		t.Fatalf("Static: %v", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestStaticCreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "a", "b", "c.txt")

	if err := materialize.Static(destPath, &domain.StaticSourceSpec{Content: "hello"}); err != nil {
		t.Fatalf("Static: %v", err)
	}
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("stat nested file: %v", err)
	}
}

func TestStaticRejectsInvalidMode(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.txt")

	err := materialize.Static(destPath, &domain.StaticSourceSpec{Content: "x", Mode: "not-a-mode"})
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
}

func TestLocalSymlinkCreatesLink(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()

	// Create a file in the source directory.
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	destPath := filepath.Join(root, "linked")
	spec := &domain.LocalSourceSpec{SourcePath: srcDir, Copy: false}
	if err := materialize.Local(root, destPath, spec); err != nil {
		t.Fatalf("Local: %v", err)
	}

	// Verify it's a symlink pointing to srcDir.
	target, err := os.Readlink(destPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != srcDir {
		t.Fatalf("symlink target = %q, want %q", target, srcDir)
	}
	// Verify the linked file is accessible.
	if _, err := os.Stat(filepath.Join(destPath, "file.txt")); err != nil {
		t.Fatalf("stat through symlink: %v", err)
	}
}

func TestLocalCopyRecursive(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	destPath := filepath.Join(root, "copied")
	spec := &domain.LocalSourceSpec{SourcePath: srcDir, Copy: true}
	if err := materialize.Local(root, destPath, spec); err != nil {
		t.Fatalf("Local: %v", err)
	}

	for _, rel := range []string{"a.txt", filepath.Join("sub", "b.txt")} {
		p := filepath.Join(destPath, rel)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestLocalRelativeSourcePath(t *testing.T) {
	root := t.TempDir()
	// Create a file relative to the tasktree root.
	relSrc := "shared-scripts"
	if err := os.MkdirAll(filepath.Join(root, relSrc), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	destPath := filepath.Join(root, "scripts")
	spec := &domain.LocalSourceSpec{SourcePath: relSrc, Copy: false}
	if err := materialize.Local(root, destPath, spec); err != nil {
		t.Fatalf("Local: %v", err)
	}
	if _, err := os.Readlink(destPath); err != nil {
		t.Fatalf("readlink: %v", err)
	}
}

func TestLocalReturnsErrorForMissingSource(t *testing.T) {
	root := t.TempDir()
	destPath := filepath.Join(root, "dest")
	spec := &domain.LocalSourceSpec{SourcePath: "/nonexistent/path/xyz", Copy: false}
	err := materialize.Local(root, destPath, spec)
	if err == nil {
		t.Fatal("expected error for missing source path, got nil")
	}
	var lse domain.LocalSourceNotFoundError
	if !isError[domain.LocalSourceNotFoundError](err, &lse) {
		t.Fatalf("expected LocalSourceNotFoundError, got %T: %v", err, err)
	}
}

// isError is a tiny helper for errors.As without importing errors in test.
func isError[T error](err error, target *T) bool {
	for err != nil {
		if e, ok := err.(T); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
