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

	sourcePath := result.Source.Path
	if sourcePath == "" {
		sourcePath = result.Source.Name
	}
	removedPath, err := removeService.Run(root, result.Source.Name)
	if err != nil {
		t.Fatalf("remove repo: %v", err)
	}
	if removedPath != filepath.Join(root, sourcePath) {
		t.Fatalf("removed path = %q, want %q", removedPath, filepath.Join(root, sourcePath))
	}
	if _, err := os.Stat(filepath.Join(root, sourcePath)); !os.IsNotExist(err) {
		t.Fatalf("expected repo directory to be removed, got %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache to remain, got %v", err)
	}
	spec, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(spec.Spec.Sources) != 0 {
		t.Fatalf("expected empty sources metadata, got %d entries", len(spec.Spec.Sources))
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
