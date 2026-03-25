package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestRemoveServiceRemovesCheckoutAndKeepsCache(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	_ = mutateRemote
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store := metadata.NewStore()
	addService := app.NewAddService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient())
	removeService := app.NewRemoveService(store)

	result, err := addService.Run(ctx, root, app.AddOptions{RepoURL: remoteURL})
	if err != nil {
		t.Fatalf("add repo: %v", err)
	}
	cachePath := cache.NewManager(cacheRoot, gitx.NewClient()).PathForURL(remoteURL)

	removedPath, err := removeService.Run(root, result.Repo.Name)
	if err != nil {
		t.Fatalf("remove repo: %v", err)
	}
	if removedPath != filepath.Join(root, result.Repo.Path) {
		t.Fatalf("removed path = %q", removedPath)
	}
	if _, err := os.Stat(filepath.Join(root, result.Repo.Path)); !os.IsNotExist(err) {
		t.Fatalf("expected repo directory to be removed, got %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache to remain, got %v", err)
	}
	file, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(file.Repos) != 0 {
		t.Fatalf("expected empty repo metadata, got %d entries", len(file.Repos))
	}
}

func TestRemoveServiceReturnsNotFoundError(t *testing.T) {
	root := createTasktree(t)
	service := app.NewRemoveService(metadata.NewStore())

	_, err := service.Run(root, "missing")
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
