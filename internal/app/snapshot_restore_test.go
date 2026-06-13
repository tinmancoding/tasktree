package app

import (
	"bytes"
	"context"
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
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestSnapshotRestoreGitRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	remotePath, _ := testutil.CreateRemoteRepo(t)

	store := metadata.NewStore()
	git := gitx.NewClient().WithDefaults()
	cacheRoot := t.TempDir()
	mgr := cache.NewManager(cacheRoot, git)

	// Workspace with a single git source.
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

	ctx := context.Background()
	apply := NewApplyService(store, mgr, git)
	if _, err := apply.Run(ctx, wsRoot, ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	apiPath := filepath.Join(wsRoot, "api")

	// Local commit not present on the remote.
	if err := os.WriteFile(filepath.Join(apiPath, "local.txt"), []byte("local commit file\n"), 0o644); err != nil {
		t.Fatalf("write local.txt: %v", err)
	}
	testutil.RunGit(t, apiPath, "add", "local.txt")
	testutil.RunGit(t, apiPath, "commit", "-m", "local-only commit")
	headSHA := strings.TrimSpace(testutil.RunGit(t, apiPath, "rev-parse", "HEAD"))

	// Dirty edits: modify a tracked file + add an untracked file.
	if err := os.WriteFile(filepath.Join(apiPath, "README.md"), []byte("seed\ndirty edit\n"), 0o644); err != nil {
		t.Fatalf("modify README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(apiPath, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	// Snapshot to a buffer.
	var buf bytes.Buffer
	snap := NewSnapshotService(store, git)
	res, err := snap.Run(ctx, apiPath, SnapshotOptions{Output: &buf})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(res.Sources) != 1 || !res.Sources[0].HasBundle || !res.Sources[0].HasDirty {
		t.Fatalf("unexpected snapshot result: %+v", res.Sources)
	}

	// Restore into a fresh dir.
	regStore := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	restoreParent := t.TempDir()
	restore := NewRestoreService(store, mgr, git, regStore)
	rres, err := restore.Run(ctx, restoreParent, RestoreOptions{Input: &buf, Into: filepath.Join(restoreParent, "restored"), SkipBootstrap: true})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	restoredAPI := filepath.Join(rres.Target, "api")

	// HEAD matches.
	gotHead := strings.TrimSpace(testutil.RunGit(t, restoredAPI, "rev-parse", "HEAD"))
	if gotHead != headSHA {
		t.Fatalf("restored HEAD = %s, want %s", gotHead, headSHA)
	}
	// Branch identity.
	gotBranch := strings.TrimSpace(testutil.RunGit(t, restoredAPI, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch != "main" {
		t.Fatalf("restored branch = %q, want main", gotBranch)
	}
	// Local commit file present.
	if b, err := os.ReadFile(filepath.Join(restoredAPI, "local.txt")); err != nil || string(b) != "local commit file\n" {
		t.Fatalf("local.txt not restored: %v / %q", err, string(b))
	}
	// Dirty modification restored.
	if b, err := os.ReadFile(filepath.Join(restoredAPI, "README.md")); err != nil || string(b) != "seed\ndirty edit\n" {
		t.Fatalf("README dirty edit not restored: %v / %q", err, string(b))
	}
	// Untracked file restored.
	if b, err := os.ReadFile(filepath.Join(restoredAPI, "untracked.txt")); err != nil || string(b) != "untracked\n" {
		t.Fatalf("untracked.txt not restored: %v / %q", err, string(b))
	}
}
