package snapshot

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/gitx"
)

func TestPackOpenRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	members := []Member{
		{Name: "snapshot.yaml", Data: []byte("version: 1\n")},
		{Name: "bundles/api.bundle", Data: []byte("binary-ish\x00data")},
	}
	if err := Pack(&buf, members); err != nil {
		t.Fatalf("pack: %v", err)
	}
	got, err := Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(got["snapshot.yaml"]) != "version: 1\n" {
		t.Errorf("manifest mismatch: %q", got["snapshot.yaml"])
	}
	if !bytes.Equal(got["bundles/api.bundle"], []byte("binary-ish\x00data")) {
		t.Errorf("bundle mismatch: %q", got["bundles/api.bundle"])
	}
}

func TestBuildAndUnpackDirtyTar(t *testing.T) {
	src := t.TempDir()
	// modified.txt exists (captured), deleted.txt does not (deletion).
	if err := os.WriteFile(filepath.Join(src, "modified.txt"), []byte("new content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "untracked.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []gitx.StatusEntry{
		{X: 'M', Y: ' ', Path: "modified.txt"}, // staged modify
		{X: '?', Y: '?', Path: "untracked.txt", Untracked: true},
		{X: ' ', Y: 'D', Path: "deleted.txt"}, // worktree deletion
	}

	tarBytes, dirty, err := BuildDirtyTar(src, entries)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !dirty {
		t.Fatal("expected dirty=true")
	}

	// Unpack into a fresh dir.
	dst := t.TempDir()
	dm, err := UnpackDirtyTar(dst, tarBytes)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if b, err := os.ReadFile(filepath.Join(dst, "modified.txt")); err != nil || string(b) != "new content\n" {
		t.Fatalf("modified.txt: %v / %q", err, string(b))
	}
	if _, err := os.Stat(filepath.Join(dst, "untracked.txt")); err != nil {
		t.Fatalf("untracked.txt missing: %v", err)
	}
	if len(dm.Deleted) != 1 || dm.Deleted[0] != "deleted.txt" {
		t.Fatalf("deletions = %v, want [deleted.txt]", dm.Deleted)
	}
	if len(dm.Staged) != 1 || dm.Staged[0] != "modified.txt" {
		t.Fatalf("staged = %v, want [modified.txt]", dm.Staged)
	}

	// Mode preserved for the captured file.
	info, err := os.Stat(filepath.Join(dst, "modified.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestBuildDirtyTarCleanIsNil(t *testing.T) {
	tarBytes, dirty, err := BuildDirtyTar(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if dirty || tarBytes != nil {
		t.Fatalf("clean tree should yield (nil,false), got dirty=%v len=%d", dirty, len(tarBytes))
	}
}
