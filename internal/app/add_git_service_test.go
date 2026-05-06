package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestAddServiceAddsDefaultBranchAndResetsOrigin(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	latestCommit := mutateRemote(t)
	_ = latestCommit
	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL})
	if err != nil {
		t.Fatalf("add repo: %v", err)
	}
	if result.Source.Name != "app" {
		t.Fatalf("source name = %q, want app", result.Source.Name)
	}
	if result.Source.Git.Ref != "" {
		// default branch checkout has empty ref in the spec (intent: default branch)
		t.Fatalf("source git ref = %q, want empty (default branch)", result.Source.Git.Ref)
	}

	repoPath := filepath.Join(root, "app")
	originURL := testutil.RunGit(t, repoPath, "remote", "get-url", "origin")
	if strings.TrimSpace(originURL) != remoteURL {
		t.Fatalf("origin url = %q, want %q", originURL, remoteURL)
	}

	spec, err := metadata.NewStore().Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(spec.Spec.Sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(spec.Spec.Sources))
	}
	if result.BranchPath != app.BranchPathHeadless {
		t.Fatalf("branch path = %v, want BranchPathHeadless", result.BranchPath)
	}
}

// TestAddServiceSupportsTagCheckout verifies that --from v1.2.0 results in a
// detached HEAD at that tag.
func TestAddServiceSupportsTagCheckout(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	commit := mutateRemote(t)
	work := t.TempDir()
	testutil.RunGit(t, work, "clone", remoteURL, filepath.Join(work, "repo"))
	repoPath := filepath.Join(work, "repo")
	testutil.RunGit(t, repoPath, "tag", "v1.2.0", commit)
	testutil.RunGit(t, repoPath, "push", "origin", "v1.2.0")

	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, From: "v1.2.0", Name: "tagged"})
	if err != nil {
		t.Fatalf("add tagged repo: %v", err)
	}
	// With the new declarative approach, Ref holds the user's intent ("v1.2.0").
	if result.Source.Git.Ref != "v1.2.0" {
		t.Fatalf("source git ref = %q, want v1.2.0", result.Source.Git.Ref)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "tagged"), "branch", "--show-current"))
	if branch != "" {
		t.Fatalf("branch = %q, want detached HEAD", branch)
	}
	if result.BranchPath != app.BranchPathHeadless {
		t.Fatalf("branch path = %v, want BranchPathHeadless", result.BranchPath)
	}
}

// TestAddServiceCreatesBranchFromExplicitFrom verifies that --branch + --from
// creates a new local branch from the specified base ref.
func TestAddServiceCreatesBranchFromExplicitFrom(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	_ = mutateRemote(t)
	result, err := service.Run(ctx, root, app.AddOptions{
		RepoURL: remoteURL,
		Branch:  "feature/payments",
		From:    "main",
		Name:    "feature-app",
	})
	if err != nil {
		t.Fatalf("add branch repo: %v", err)
	}
	if result.Source.Git.Branch != "feature/payments" {
		t.Fatalf("branch = %q, want feature/payments", result.Source.Git.Branch)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "feature-app"), "branch", "--show-current"))
	if branch != "feature/payments" {
		t.Fatalf("branch = %q, want feature/payments", branch)
	}
	if result.BranchPath != app.BranchPathCreated {
		t.Fatalf("branch path = %v, want BranchPathCreated", result.BranchPath)
	}
	if result.EffectiveFrom != "main" {
		t.Fatalf("effective from = %q, want main", result.EffectiveFrom)
	}
}

// TestAddServiceCreatesBranchFromDefaultBranch verifies that --branch without
// --from creates the new branch from the repo's default branch.
func TestAddServiceCreatesBranchFromDefaultBranch(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	_ = mutateRemote(t)
	result, err := service.Run(ctx, root, app.AddOptions{
		RepoURL: remoteURL,
		Branch:  "feature/new",
		Name:    "new-app",
	})
	if err != nil {
		t.Fatalf("add repo: %v", err)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "new-app"), "branch", "--show-current"))
	if branch != "feature/new" {
		t.Fatalf("branch = %q, want feature/new", branch)
	}
	if result.BranchPath != app.BranchPathCreated {
		t.Fatalf("branch path = %v, want BranchPathCreated", result.BranchPath)
	}
	// effective from should be the default branch (main)
	if result.EffectiveFrom != "main" {
		t.Fatalf("effective from = %q, want main", result.EffectiveFrom)
	}
}

// TestAddServiceTracksRemoteBranchViaBranchFlag verifies that --branch
// feature-branch creates a local tracking branch when only origin/feature-branch
// exists, and that --from is ignored and reported.
func TestAddServiceTracksRemoteBranchViaBranchFlag(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	// Create a remote branch.
	work := t.TempDir()
	testutil.RunGit(t, work, "clone", remoteURL, filepath.Join(work, "repo"))
	repoPath := filepath.Join(work, "repo")
	testutil.RunGit(t, repoPath, "checkout", "-b", "feature-branch")
	testutil.RunGit(t, repoPath, "commit", "--allow-empty", "-m", "feature commit")
	testutil.RunGit(t, repoPath, "push", "origin", "feature-branch")

	result, err := service.Run(ctx, root, app.AddOptions{
		RepoURL: remoteURL,
		Branch:  "feature-branch",
		From:    "main", // should be ignored
		Name:    "feature-app",
	})
	if err != nil {
		t.Fatalf("add repo with remote branch: %v", err)
	}

	destPath := filepath.Join(root, "feature-app")

	// Verify we're on a local branch (not detached HEAD).
	currentBranch := strings.TrimSpace(testutil.RunGit(t, destPath, "branch", "--show-current"))
	if currentBranch != "feature-branch" {
		t.Fatalf("current branch = %q, want feature-branch", currentBranch)
	}

	// Verify the branch tracks the remote branch.
	upstream := strings.TrimSpace(testutil.RunGit(t, destPath, "rev-parse", "--symbolic-full-name", "@{u}"))
	if upstream != "refs/remotes/origin/feature-branch" {
		t.Fatalf("upstream = %q, want refs/remotes/origin/feature-branch", upstream)
	}

	if result.BranchPath != app.BranchPathRemoteTracking {
		t.Fatalf("branch path = %v, want BranchPathRemoteTracking", result.BranchPath)
	}
	if result.IgnoredFrom != "main" {
		t.Fatalf("ignored from = %q, want main", result.IgnoredFrom)
	}
}

// TestAddServiceRejectsInvalidBranchName verifies that an invalid --branch name
// is rejected early.
func TestAddServiceRejectsInvalidBranchName(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(filepath.Join(t.TempDir(), "cache"), gitx.NewClient()), gitx.NewClient())

	_, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Branch: "bad..branch", Name: "invalid-branch-app"})
	if err == nil || !strings.Contains(err.Error(), "invalid branch name") {
		t.Fatalf("expected invalid branch name error, got %v", err)
	}
}

// TestAddServiceRejectsDuplicateRepoName verifies that adding a repo whose
// directory name already exists in metadata is rejected.
func TestAddServiceRejectsDuplicateRepoName(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	spec.Spec.Sources = append(spec.Spec.Sources, domain.SourceSpec{
		Name: "app",
		Type: domain.SourceTypeGit,
		Path: "app",
		Git:  &domain.GitSourceSpec{URL: remoteURL},
	})
	if err := store.Save(root, spec); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	service := app.NewAddService(store, cache.NewManager(filepath.Join(t.TempDir(), "cache"), gitx.NewClient()), gitx.NewClient())
	_, err = service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate repo error, got %v", err)
	}
}

// TestAddServiceCleansUpAfterUnresolvedRefFailure verifies that when --from
// points to a non-existent ref the checkout directory is cleaned up and
// metadata is unchanged.
func TestAddServiceCleansUpAfterUnresolvedRefFailure(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	service := app.NewAddService(store, cache.NewManager(filepath.Join(t.TempDir(), "cache"), gitx.NewClient()), gitx.NewClient())

	_, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, From: "missing-ref", Name: "broken-app"})
	if err == nil || !strings.Contains(err.Error(), "could not resolve ref") {
		t.Fatalf("expected unresolved ref error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "broken-app")); !os.IsNotExist(statErr) {
		t.Fatalf("expected checkout cleanup, got %v", statErr)
	}
	spec, loadErr := store.Load(root)
	if loadErr != nil {
		t.Fatalf("load metadata: %v", loadErr)
	}
	if len(spec.Spec.Sources) != 0 {
		t.Fatalf("expected metadata to remain unchanged, got %d sources", len(spec.Spec.Sources))
	}
}

// TestAddServiceFromAloneChecksOutRemoteBranchAsLocalTracking verifies that
// --from feature-branch (with no --branch) creates a local tracking branch
// when only origin/feature-branch exists.
func TestAddServiceFromAloneChecksOutRemoteBranchAsLocalTracking(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	// Create a remote branch.
	work := t.TempDir()
	testutil.RunGit(t, work, "clone", remoteURL, filepath.Join(work, "repo"))
	repoPath := filepath.Join(work, "repo")
	testutil.RunGit(t, repoPath, "checkout", "-b", "feature-branch")
	testutil.RunGit(t, repoPath, "commit", "--allow-empty", "-m", "feature commit")
	testutil.RunGit(t, repoPath, "push", "origin", "feature-branch")

	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, From: "feature-branch", Name: "feature-app"})
	if err != nil {
		t.Fatalf("add repo with remote branch ref: %v", err)
	}

	destPath := filepath.Join(root, "feature-app")

	// Verify we're on a local branch (not detached HEAD).
	currentBranch := strings.TrimSpace(testutil.RunGit(t, destPath, "branch", "--show-current"))
	if currentBranch != "feature-branch" {
		t.Fatalf("current branch = %q, want feature-branch", currentBranch)
	}

	// Verify the branch tracks the remote branch.
	upstream := strings.TrimSpace(testutil.RunGit(t, destPath, "rev-parse", "--symbolic-full-name", "@{u}"))
	if upstream != "refs/remotes/origin/feature-branch" {
		t.Fatalf("upstream = %q, want refs/remotes/origin/feature-branch", upstream)
	}

	// With declarative approach, Ref holds the --from value.
	if result.Source.Git.Ref != "feature-branch" {
		t.Fatalf("source git ref = %q, want feature-branch", result.Source.Git.Ref)
	}

	// --from alone with remote branch = BranchPathRemoteTracking, not BranchPathHeadless.
	if result.BranchPath != app.BranchPathRemoteTracking {
		t.Fatalf("branch path = %v, want BranchPathRemoteTracking", result.BranchPath)
	}
}

func createTasktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	store := metadata.NewStore()
	if err := store.Save(root, domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:      filepath.Base(root),
			CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{},
		},
	}); err != nil {
		t.Fatalf("save tasktree metadata: %v", err)
	}
	return root
}
