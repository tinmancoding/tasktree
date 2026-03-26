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
	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL})
	if err != nil {
		t.Fatalf("add repo: %v", err)
	}
	if result.Repo.Name != "app" {
		t.Fatalf("repo name = %q, want app", result.Repo.Name)
	}
	if result.Repo.Checkout != "main" {
		t.Fatalf("checkout = %q, want main", result.Repo.Checkout)
	}
	if result.Repo.Commit != latestCommit {
		t.Fatalf("commit = %q, want %q", result.Repo.Commit, latestCommit)
	}

	repoPath := filepath.Join(root, "app")
	originURL := testutil.RunGit(t, repoPath, "remote", "get-url", "origin")
	if strings.TrimSpace(originURL) != remoteURL {
		t.Fatalf("origin url = %q, want %q", originURL, remoteURL)
	}

	file, err := metadata.NewStore().Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(file.Repos) != 1 {
		t.Fatalf("repo count = %d, want 1", len(file.Repos))
	}
}

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

	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Ref: "v1.2.0", Name: "tagged"})
	if err != nil {
		t.Fatalf("add tagged repo: %v", err)
	}
	if result.Repo.ResolvedRef != "refs/tags/v1.2.0" {
		t.Fatalf("resolved ref = %q, want refs/tags/v1.2.0", result.Repo.ResolvedRef)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "tagged"), "branch", "--show-current"))
	if branch != "" {
		t.Fatalf("branch = %q, want detached HEAD", branch)
	}
}

func TestAddServiceCreatesBranchFromRequestedRef(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	commit := mutateRemote(t)
	_ = commit
	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Ref: "main", Branch: "feature/payments", Name: "feature-app"})
	if err != nil {
		t.Fatalf("add branch repo: %v", err)
	}
	if result.Repo.Branch != "feature/payments" {
		t.Fatalf("branch = %q, want feature/payments", result.Repo.Branch)
	}
	if result.Repo.ResolvedRef != "refs/heads/feature/payments" {
		t.Fatalf("resolved ref = %q, want refs/heads/feature/payments", result.Repo.ResolvedRef)
	}
	branch := strings.TrimSpace(testutil.RunGit(t, filepath.Join(root, "feature-app"), "branch", "--show-current"))
	if branch != "feature/payments" {
		t.Fatalf("branch = %q, want feature/payments", branch)
	}
}

func TestAddServiceRejectsDuplicateRepoName(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	file, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	file.Repos = append(file.Repos, domain.RepoSpec{Name: "app", Path: "app"})
	if err := store.Save(root, file); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	service := app.NewAddService(store, cache.NewManager(filepath.Join(t.TempDir(), "cache"), gitx.NewClient()), gitx.NewClient())
	_, err = service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate repo error, got %v", err)
	}
}

func TestAddServiceCleansUpAfterUnresolvedRefFailure(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	store := metadata.NewStore()
	service := app.NewAddService(store, cache.NewManager(filepath.Join(t.TempDir(), "cache"), gitx.NewClient()), gitx.NewClient())

	_, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Ref: "missing-ref", Name: "broken-app"})
	if err == nil || !strings.Contains(err.Error(), "could not resolve ref") {
		t.Fatalf("expected unresolved ref error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "broken-app")); !os.IsNotExist(statErr) {
		t.Fatalf("expected checkout cleanup, got %v", statErr)
	}
	file, loadErr := store.Load(root)
	if loadErr != nil {
		t.Fatalf("load metadata: %v", loadErr)
	}
	if len(file.Repos) != 0 {
		t.Fatalf("expected metadata to remain unchanged, got %d repos", len(file.Repos))
	}
}

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

func TestAddServiceChecksOutRemoteBranchAsLocalTracking(t *testing.T) {
	ctx := context.Background()
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	service := app.NewAddService(metadata.NewStore(), cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())

	// Create a remote branch
	work := t.TempDir()
	testutil.RunGit(t, work, "clone", remoteURL, filepath.Join(work, "repo"))
	repoPath := filepath.Join(work, "repo")
	testutil.RunGit(t, repoPath, "checkout", "-b", "feature-branch")
	testutil.RunGit(t, repoPath, "commit", "--allow-empty", "-m", "feature commit")
	testutil.RunGit(t, repoPath, "push", "origin", "feature-branch")

	// Add repo with --ref pointing to the remote branch name (without origin/ prefix)
	result, err := service.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Ref: "feature-branch", Name: "feature-app"})
	if err != nil {
		t.Fatalf("add repo with remote branch ref: %v", err)
	}

	destPath := filepath.Join(root, "feature-app")

	// Verify we're on a local branch (not detached HEAD)
	currentBranch := strings.TrimSpace(testutil.RunGit(t, destPath, "branch", "--show-current"))
	if currentBranch != "feature-branch" {
		t.Fatalf("current branch = %q, want feature-branch", currentBranch)
	}

	// Verify the branch tracks the remote branch
	upstream := strings.TrimSpace(testutil.RunGit(t, destPath, "rev-parse", "--symbolic-full-name", "@{u}"))
	if upstream != "refs/remotes/origin/feature-branch" {
		t.Fatalf("upstream = %q, want refs/remotes/origin/feature-branch", upstream)
	}

	// Verify resolved ref is the local branch
	if result.Repo.ResolvedRef != "refs/heads/feature-branch" {
		t.Fatalf("resolved ref = %q, want refs/heads/feature-branch", result.Repo.ResolvedRef)
	}

	// Verify checkout field shows the branch name
	if result.Repo.Checkout != "feature-branch" {
		t.Fatalf("checkout = %q, want feature-branch", result.Repo.Checkout)
	}
}

func createTasktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	store := metadata.NewStore()
	if err := store.Save(root, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save tasktree metadata: %v", err)
	}
	return root
}
