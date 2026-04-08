package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

// TestApplyServiceClonesGitSource verifies that apply clones a git source that
// is declared in the spec but not yet present on disk.
func TestApplyServiceClonesGitSource(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	spec.Spec.Sources = []domain.SourceSpec{
		{Name: "myapp", Type: domain.SourceTypeGit, Path: "myapp", Git: &domain.GitSourceSpec{URL: remoteURL}},
	}
	if err := store.Save(root, spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	service := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	result, err := service.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	if result.Results[0].Status != app.SourceApplyStatusCloned {
		t.Fatalf("status = %v, want SourceApplyStatusCloned", result.Results[0].Status)
	}
	if _, err := os.Stat(filepath.Join(root, "myapp")); os.IsNotExist(err) {
		t.Fatalf("expected checkout at myapp, not found")
	}
}

// TestApplyServiceSkipsExistingSource verifies that apply skips a source whose
// destination directory already exists on disk.
func TestApplyServiceSkipsExistingSource(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	git := gitx.NewClient()
	addService := app.NewAddService(store, cache.NewManager(cacheRoot, git), git)
	applyService := app.NewApplyService(store, cache.NewManager(cacheRoot, git), git)

	if _, err := addService.Run(ctx, root, app.AddOptions{RepoURL: remoteURL}); err != nil {
		t.Fatalf("add: %v", err)
	}

	result, err := applyService.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	if result.Results[0].Status != app.SourceApplyStatusSkipped {
		t.Fatalf("status = %v, want SourceApplyStatusSkipped", result.Results[0].Status)
	}
}

// TestApplyServiceIsIdempotent verifies that running apply twice does not fail
// and correctly skips already-present sources on the second run.
func TestApplyServiceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	git := gitx.NewClient()
	service := app.NewApplyService(store, cache.NewManager(cacheRoot, git), git)

	spec, _ := store.Load(root)
	spec.Spec.Sources = []domain.SourceSpec{
		{Name: "myapp", Type: domain.SourceTypeGit, Path: "myapp", Git: &domain.GitSourceSpec{URL: remoteURL}},
	}
	_ = store.Save(root, spec)

	result1, err := service.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if result1.Results[0].Status != app.SourceApplyStatusCloned {
		t.Fatalf("first apply status = %v, want Cloned", result1.Results[0].Status)
	}

	result2, err := service.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if result2.Results[0].Status != app.SourceApplyStatusSkipped {
		t.Fatalf("second apply status = %v, want Skipped", result2.Results[0].Status)
	}
}

// TestApplyServiceDryRunDoesNotMaterialize verifies that --dry-run returns
// SourceApplyStatusWouldClone without creating any directories.
func TestApplyServiceDryRunDoesNotMaterialize(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	spec, _ := store.Load(root)
	spec.Spec.Sources = []domain.SourceSpec{
		{Name: "myapp", Type: domain.SourceTypeGit, Path: "myapp", Git: &domain.GitSourceSpec{URL: remoteURL}},
	}
	_ = store.Save(root, spec)

	service := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	result, err := service.Run(ctx, root, app.ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run apply: %v", err)
	}
	if result.Results[0].Status != app.SourceApplyStatusWouldClone {
		t.Fatalf("status = %v, want SourceApplyStatusWouldClone", result.Results[0].Status)
	}
	if _, err := os.Stat(filepath.Join(root, "myapp")); !os.IsNotExist(err) {
		t.Fatalf("expected no checkout after dry-run, but found one")
	}
}

// TestApplyServiceSkipsUnsupportedSourceType verifies that sources with types
// other than "git" are reported as unsupported and do not cause an error.
func TestApplyServiceSkipsUnsupportedSourceType(t *testing.T) {
	ctx := context.Background()
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	spec, _ := store.Load(root)
	spec.Spec.Sources = []domain.SourceSpec{
		{Name: "cfg", Type: domain.SourceTypeHTTP, Path: "cfg"},
	}
	_ = store.Save(root, spec)

	service := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	result, err := service.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Results[0].Status != app.SourceApplyStatusUnsupported {
		t.Fatalf("status = %v, want SourceApplyStatusUnsupported", result.Results[0].Status)
	}
}

// TestApplyServiceAppliesBranchFromSpec verifies that a source with a branch
// set in the spec results in that branch being checked out.
func TestApplyServiceAppliesBranchFromSpec(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	// Simulate what `tasktree add --branch feature/new` would write to the spec:
	// Ref == Branch (no explicit --from was given).
	spec, _ := store.Load(root)
	spec.Spec.Sources = []domain.SourceSpec{
		{
			Name: "myapp",
			Type: domain.SourceTypeGit,
			Path: "myapp",
			Git: &domain.GitSourceSpec{
				URL:    remoteURL,
				Ref:    "feature/new",
				Branch: "feature/new",
			},
		},
	}
	_ = store.Save(root, spec)

	service := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	result, err := service.Run(ctx, root, app.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Results[0].Status != app.SourceApplyStatusCloned {
		t.Fatalf("status = %v, want Cloned", result.Results[0].Status)
	}
	if result.Results[0].BranchPath != app.BranchPathCreated {
		t.Fatalf("branch path = %v, want BranchPathCreated", result.Results[0].BranchPath)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "myapp"), "branch", "--show-current"))
	if branch != "feature/new" {
		t.Fatalf("branch = %q, want feature/new", branch)
	}
}

// TestApplyServiceCleansUpAfterFailure verifies that the checkout directory is
// removed when an error occurs during materialization.
func TestApplyServiceCleansUpAfterFailure(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	spec, _ := store.Load(root)
	spec.Spec.Sources = []domain.SourceSpec{
		{
			Name: "myapp",
			Type: domain.SourceTypeGit,
			Path: "myapp",
			Git:  &domain.GitSourceSpec{URL: remoteURL, Ref: "nonexistent-ref-xyz"},
		},
	}
	_ = store.Save(root, spec)

	service := app.NewApplyService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	_, err := service.Run(ctx, root, app.ApplyOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent ref, got nil")
	}
	if !strings.Contains(err.Error(), "could not resolve ref") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "myapp")); !os.IsNotExist(statErr) {
		t.Fatalf("expected cleanup after failure, checkout still exists")
	}
}
