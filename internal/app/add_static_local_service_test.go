package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// TestAddStaticServiceWritesFile verifies that AddStaticService creates the
// file with the declared content and registers it in Tasktree.yml.
func TestAddStaticServiceWritesFile(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	store := metadata.NewStore()
	svc := app.NewAddStaticService(store)

	result, err := svc.Run(ctx, root, app.AddStaticOptions{
		Name:    "override.yml",
		Content: "services:\n  api:\n    environment:\n      DEBUG: \"true\"\n",
	})
	if err != nil {
		t.Fatalf("AddStaticService.Run: %v", err)
	}
	if result.Source.Type != domain.SourceTypeStatic {
		t.Fatalf("source type = %q, want static", result.Source.Type)
	}

	content, err := os.ReadFile(filepath.Join(root, "override.yml"))
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if string(content) != result.Source.Static.Content {
		t.Fatalf("content mismatch")
	}

	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(spec.Spec.Sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(spec.Spec.Sources))
	}
}

// TestAddStaticServiceRejectsDuplicateName verifies that a duplicate source
// name is rejected with DuplicateSourceNameError.
func TestAddStaticServiceRejectsDuplicateName(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	store := metadata.NewStore()
	svc := app.NewAddStaticService(store)

	_, err := svc.Run(ctx, root, app.AddStaticOptions{Name: "cfg.yml", Content: "a: b"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err = svc.Run(ctx, root, app.AddStaticOptions{Name: "cfg.yml", Content: "c: d"})
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
	var dup domain.DuplicateSourceNameError
	if !isErrorType[domain.DuplicateSourceNameError](err, &dup) {
		t.Fatalf("expected DuplicateSourceNameError, got %T: %v", err, err)
	}
}

// TestAddLocalServiceCreatesSymlink verifies that AddLocalService creates a
// symlink and registers the source.
func TestAddLocalServiceCreatesSymlink(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "tool.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	store := metadata.NewStore()
	svc := app.NewAddLocalService(store)

	result, err := svc.Run(ctx, root, app.AddLocalOptions{
		SourcePath: srcDir,
		Name:       "scripts",
	})
	if err != nil {
		t.Fatalf("AddLocalService.Run: %v", err)
	}
	if !result.Symlinked {
		t.Fatal("expected Symlinked = true")
	}

	target, err := os.Readlink(filepath.Join(root, "scripts"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != srcDir {
		t.Fatalf("symlink target = %q, want %q", target, srcDir)
	}

	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(spec.Spec.Sources) != 1 || spec.Spec.Sources[0].Type != domain.SourceTypeLocal {
		t.Fatalf("unexpected sources: %v", spec.Spec.Sources)
	}
}

// TestAddLocalServiceCopiesDirectory verifies that --copy produces a real
// directory, not a symlink.
func TestAddLocalServiceCopiesDirectory(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	store := metadata.NewStore()
	svc := app.NewAddLocalService(store)

	result, err := svc.Run(ctx, root, app.AddLocalOptions{
		SourcePath: srcDir,
		Name:       "copy",
		Copy:       true,
	})
	if err != nil {
		t.Fatalf("AddLocalService.Run: %v", err)
	}
	if result.Symlinked {
		t.Fatal("expected Symlinked = false for copy mode")
	}

	destFile := filepath.Join(root, "copy", "a.txt")
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(content) != "a" {
		t.Fatalf("copied content = %q, want %q", string(content), "a")
	}
}

// TestAddLocalServiceRejectsMissingSource verifies that a non-existent
// sourcePath is rejected with LocalSourceNotFoundError.
func TestAddLocalServiceRejectsMissingSource(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	store := metadata.NewStore()
	svc := app.NewAddLocalService(store)

	_, err := svc.Run(ctx, root, app.AddLocalOptions{
		SourcePath: "/nonexistent/xyz",
		Name:       "missing",
	})
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
	var notFound domain.LocalSourceNotFoundError
	if !isErrorType[domain.LocalSourceNotFoundError](err, &notFound) {
		t.Fatalf("expected LocalSourceNotFoundError, got %T: %v", err, err)
	}
}

// isErrorType is a helper analogous to errors.As but usable without importing
// the errors package in this test file.
func isErrorType[T error](err error, target *T) bool {
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
