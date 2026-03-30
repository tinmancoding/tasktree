package registry_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/registry"
)

func newTestStore(t *testing.T) *registry.Store {
	t.Helper()
	return registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
}

func TestLoad_MissingFile(t *testing.T) {
	s := newTestStore(t)
	f, err := s.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(f.Tasktrees) != 0 {
		t.Fatalf("expected empty tasktrees, got %d", len(f.Tasktrees))
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.toml")
	if err := os.WriteFile(path, []byte("this is not [[[ valid toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := registry.NewStoreAt(path)
	_, err := s.Load()
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

func TestRegister_NewEntry(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(f.Tasktrees))
	}
	if f.Tasktrees[0].Path != "/tmp/ws/alpha" {
		t.Errorf("unexpected path: %s", f.Tasktrees[0].Path)
	}
	if f.Tasktrees[0].Name != "alpha" {
		t.Errorf("unexpected name: %s", f.Tasktrees[0].Name)
	}
}

func TestRegister_DuplicatePathUpdatesName(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "old-name"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("/tmp/ws/alpha", "new-name"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("expected 1 entry after duplicate, got %d", len(f.Tasktrees))
	}
	if f.Tasktrees[0].Name != "new-name" {
		t.Errorf("expected updated name, got %s", f.Tasktrees[0].Name)
	}
}

func TestRegister_PreservesAddedAt(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	original := f.Tasktrees[0].AddedAt

	time.Sleep(10 * time.Millisecond)
	if err := s.Register("/tmp/ws/alpha", "alpha-renamed"); err != nil {
		t.Fatal(err)
	}
	f, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !f.Tasktrees[0].AddedAt.Equal(original) {
		t.Error("AddedAt should be preserved on re-register")
	}
}

func TestRegister_MultipleEntries(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("/tmp/ws/beta", "beta"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(f.Tasktrees))
	}
}

func TestDeregister_Existing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("/tmp/ws/beta", "beta"); err != nil {
		t.Fatal(err)
	}
	if err := s.Deregister("/tmp/ws/alpha"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("expected 1 entry after deregister, got %d", len(f.Tasktrees))
	}
	if f.Tasktrees[0].Path != "/tmp/ws/beta" {
		t.Errorf("unexpected remaining path: %s", f.Tasktrees[0].Path)
	}
}

func TestDeregister_NonExistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Deregister("/tmp/ws/nonexistent"); err != nil {
		t.Fatalf("Deregister of unknown path: %v", err)
	}
}

func TestSave_CreatesParentDirectories(t *testing.T) {
	deep := filepath.Join(t.TempDir(), "a", "b", "c", "registry.toml")
	s := registry.NewStoreAt(deep)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatalf("Register with deep path: %v", err)
	}
}

func TestRoundtrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Register("/tmp/ws/alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("/tmp/ws/beta", "beta"); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 2 {
		t.Fatalf("roundtrip: expected 2 entries, got %d", len(f.Tasktrees))
	}
	if f.Tasktrees[0].Path != "/tmp/ws/alpha" || f.Tasktrees[1].Path != "/tmp/ws/beta" {
		t.Errorf("roundtrip: unexpected paths %v", f.Tasktrees)
	}
}
