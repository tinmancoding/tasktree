package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	"github.com/tinmancoding/tasktree/internal/snapshot"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// gitWorkspace applies a single-git-source workspace and returns its root.
func gitWorkspace(t *testing.T, store metadata.Store, mgr cache.Manager, git gitx.Client, remotePath string) string {
	t.Helper()
	wsRoot := t.TempDir()
	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata:   domain.SpecMetadata{Name: "demo"},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{
				{Name: "api", Type: domain.SourceTypeGit, Git: &domain.GitSourceSpec{URL: remotePath, Branch: "main", Ref: "main"}},
			},
		},
	}
	if err := store.Save(wsRoot, spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	if _, err := NewApplyService(store, mgr, git).Run(context.Background(), wsRoot, ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return wsRoot
}

func TestSnapshotCleanWorkspace(t *testing.T) {
	requireGit(t)
	remotePath, _ := testutil.CreateRemoteRepo(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	mgr := cache.NewManager(t.TempDir(), git)
	wsRoot := gitWorkspace(t, store, mgr, git, remotePath)

	var buf bytes.Buffer
	res, err := NewSnapshotService(store, git).Run(context.Background(), wsRoot, SnapshotOptions{Output: &buf})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if res.Sources[0].HasBundle || res.Sources[0].HasDirty {
		t.Fatalf("clean workspace should have no bundle/dirty: %+v", res.Sources[0])
	}
	// Manifest still pins base==head.
	members, err := snapshot.Open(&buf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok := members[domain.SnapshotManifestName]; !ok {
		t.Fatal("missing manifest")
	}
	if _, ok := members[domain.SpecFileName]; !ok {
		t.Fatal("missing embedded spec")
	}
}

func TestSnapshotMissingSourceFails(t *testing.T) {
	requireGit(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	wsRoot := t.TempDir()
	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion, Kind: domain.KindTasktree,
		Metadata: domain.SpecMetadata{Name: "demo"},
		Spec: domain.WorkspaceSpec{Sources: []domain.SourceSpec{
			{Name: "api", Type: domain.SourceTypeGit, Git: &domain.GitSourceSpec{URL: "x", Branch: "main"}},
		}},
	}
	if err := store.Save(wsRoot, spec); err != nil {
		t.Fatalf("save: %v", err)
	}
	var buf bytes.Buffer
	_, err := NewSnapshotService(store, git).Run(context.Background(), wsRoot, SnapshotOptions{Output: &buf})
	var target domain.SourceNotMaterializedError
	if !errors.As(err, &target) {
		t.Fatalf("want SourceNotMaterializedError, got %v", err)
	}
}

func TestSnapshotNoOriginFails(t *testing.T) {
	requireGit(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	wsRoot := t.TempDir()
	// A git source dir with no origin remote.
	apiPath := filepath.Join(wsRoot, "api")
	if err := os.MkdirAll(apiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, apiPath, "init")
	testutil.RunGit(t, apiPath, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(apiPath, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, apiPath, "add", ".")
	testutil.RunGit(t, apiPath, "commit", "-m", "init")

	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion, Kind: domain.KindTasktree,
		Metadata: domain.SpecMetadata{Name: "demo"},
		Spec: domain.WorkspaceSpec{Sources: []domain.SourceSpec{
			{Name: "api", Type: domain.SourceTypeGit, Git: &domain.GitSourceSpec{URL: "x", Branch: "main"}},
		}},
	}
	if err := store.Save(wsRoot, spec); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err := NewSnapshotService(store, git).Run(context.Background(), wsRoot, SnapshotOptions{Output: &buf})
	var target domain.MissingOriginRemoteError
	if !errors.As(err, &target) {
		t.Fatalf("want MissingOriginRemoteError, got %v", err)
	}
}

func TestRestoreRejectsNonEmptyTarget(t *testing.T) {
	requireGit(t)
	remotePath, _ := testutil.CreateRemoteRepo(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	mgr := cache.NewManager(t.TempDir(), git)
	wsRoot := gitWorkspace(t, store, mgr, git, remotePath)

	var buf bytes.Buffer
	if _, err := NewSnapshotService(store, git).Run(context.Background(), wsRoot, SnapshotOptions{Output: &buf}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	target := t.TempDir() // exists and non-empty
	if err := os.WriteFile(filepath.Join(target, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	regStore := registry.NewStoreAt(filepath.Join(t.TempDir(), "reg.toml"))
	_, err := NewRestoreService(store, mgr, git, regStore).Run(context.Background(), t.TempDir(), RestoreOptions{Input: &buf, Into: target, SkipBootstrap: true})
	var nonEmpty domain.NonEmptyRestoreTargetError
	if !errors.As(err, &nonEmpty) {
		t.Fatalf("want NonEmptyRestoreTargetError, got %v", err)
	}
}

func TestRestoreRejectsUnknownVersion(t *testing.T) {
	requireGit(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	mgr := cache.NewManager(t.TempDir(), git)
	regStore := registry.NewStoreAt(filepath.Join(t.TempDir(), "reg.toml"))

	// Hand-craft a snapshot tar.gz with a bad manifest version.
	var buf bytes.Buffer
	members := []snapshot.Member{
		{Name: domain.SnapshotManifestName, Data: []byte("version: 99\ntasktree: demo\n")},
		{Name: domain.SpecFileName, Data: []byte("apiVersion: tasktree.dev/v1\nkind: Tasktree\nmetadata:\n  name: demo\nspec:\n  sources: []\n")},
	}
	if err := snapshot.Pack(&buf, members); err != nil {
		t.Fatal(err)
	}
	_, err := NewRestoreService(store, mgr, git, regStore).Run(context.Background(), t.TempDir(), RestoreOptions{Input: &buf, Into: filepath.Join(t.TempDir(), "out"), SkipBootstrap: true})
	var target domain.UnsupportedSnapshotVersionError
	if !errors.As(err, &target) {
		t.Fatalf("want UnsupportedSnapshotVersionError, got %v", err)
	}
}

func TestSnapshotRestoreDetachedAndDeletion(t *testing.T) {
	requireGit(t)
	remotePath, _ := testutil.CreateRemoteRepo(t)
	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	mgr := cache.NewManager(t.TempDir(), git)
	wsRoot := gitWorkspace(t, store, mgr, git, remotePath)
	apiPath := filepath.Join(wsRoot, "api")

	// Detach HEAD.
	headSHA := strings.TrimSpace(testutil.RunGit(t, apiPath, "rev-parse", "HEAD"))
	testutil.RunGit(t, apiPath, "checkout", "--detach", headSHA)
	// Delete a tracked file (dirty deletion).
	if err := os.Remove(filepath.Join(apiPath, "README.md")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := NewSnapshotService(store, git).Run(context.Background(), apiPath, SnapshotOptions{Output: &buf}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	regStore := registry.NewStoreAt(filepath.Join(t.TempDir(), "reg.toml"))
	parent := t.TempDir()
	rres, err := NewRestoreService(store, mgr, git, regStore).Run(context.Background(), parent, RestoreOptions{Input: &buf, Into: filepath.Join(parent, "out"), SkipBootstrap: true})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	restoredAPI := filepath.Join(rres.Target, "api")

	// Detached HEAD restored at headSHA.
	gotBranch := strings.TrimSpace(testutil.RunGit(t, restoredAPI, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch != "HEAD" {
		t.Fatalf("expected detached HEAD, got branch %q", gotBranch)
	}
	gotHead := strings.TrimSpace(testutil.RunGit(t, restoredAPI, "rev-parse", "HEAD"))
	if gotHead != headSHA {
		t.Fatalf("restored HEAD = %s, want %s", gotHead, headSHA)
	}
	// Deletion applied.
	if _, err := os.Stat(filepath.Join(restoredAPI, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("README.md should have been deleted, stat err=%v", err)
	}
}
