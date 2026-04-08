package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
)

func makeTasktreeSpec(name string) domain.TasktreeSpec {
	return domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:      name,
			CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{},
		},
	}
}

func TestPruneServiceReturnsNothingWhenAllValid(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	store := metadata.NewStore()

	root := t.TempDir()
	if err := store.Save(root, makeTasktreeSpec(filepath.Base(root))); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := reg.Register(root, "valid-ws"); err != nil {
		t.Fatalf("register: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no stale entries, got %d", len(results))
	}

	// registry must still contain the valid entry
	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("registry entry count = %d, want 1", len(f.Tasktrees))
	}
}

func TestPruneServiceRemovesMissingEntry(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))

	missingPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(missingPath, "ghost"); err != nil {
		t.Fatalf("register: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(results))
	}
	if results[0].Status != app.TasktreeStatusMissing {
		t.Fatalf("status = %q, want missing", results[0].Status)
	}

	// entry must have been removed from registry
	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 0 {
		t.Fatalf("registry entry count = %d, want 0", len(f.Tasktrees))
	}
}

func TestPruneServiceRemovesInvalidEntry(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))

	// path exists but has no Tasktree.yml
	invalidRoot := t.TempDir()
	if err := reg.Register(invalidRoot, "no-yaml"); err != nil {
		t.Fatalf("register: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(results))
	}
	if results[0].Status != app.TasktreeStatusInvalid {
		t.Fatalf("status = %q, want invalid", results[0].Status)
	}

	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 0 {
		t.Fatalf("registry entry count = %d, want 0", len(f.Tasktrees))
	}
}

func TestPruneServiceDryRunDoesNotModifyRegistry(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))

	missingPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(missingPath, "ghost"); err != nil {
		t.Fatalf("register: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(true)
	if err != nil {
		t.Fatalf("prune dry-run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// registry must be untouched
	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("registry entry count = %d, want 1 (dry-run should not remove)", len(f.Tasktrees))
	}
}

func TestPruneServiceOnlyRemovesStaleEntries(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	store := metadata.NewStore()

	validRoot := t.TempDir()
	if err := store.Save(validRoot, makeTasktreeSpec("valid")); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := reg.Register(validRoot, "valid"); err != nil {
		t.Fatalf("register valid: %v", err)
	}

	missingPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(missingPath, "ghost"); err != nil {
		t.Fatalf("register ghost: %v", err)
	}

	invalidRoot := t.TempDir() // exists but no Tasktree.yml
	if err := reg.Register(invalidRoot, "no-yaml"); err != nil {
		t.Fatalf("register invalid: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 stale entries, got %d", len(results))
	}

	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("registry entry count = %d, want 1", len(f.Tasktrees))
	}
	if f.Tasktrees[0].Path != validRoot {
		t.Fatalf("remaining path = %q, want %q", f.Tasktrees[0].Path, validRoot)
	}
}

func TestPruneServiceNothingToPruneMessage(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune empty registry: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestPruneServiceRespectsDeletedYaml(t *testing.T) {
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	store := metadata.NewStore()

	root := t.TempDir()
	if err := store.Save(root, makeTasktreeSpec("was-valid")); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := reg.Register(root, "was-valid"); err != nil {
		t.Fatalf("register: %v", err)
	}

	// now delete the Tasktree.yml
	if err := os.Remove(filepath.Join(root, domain.SpecFileName)); err != nil {
		t.Fatalf("remove yaml: %v", err)
	}

	service := app.NewPruneService(reg)
	results, err := service.Run(false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(results) != 1 || results[0].Status != app.TasktreeStatusInvalid {
		t.Fatalf("expected 1 invalid result, got %+v", results)
	}
}
