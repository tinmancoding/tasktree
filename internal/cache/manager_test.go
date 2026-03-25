package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestManagerEnsureCreatesAndRefreshesBareCache(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	manager := cache.NewManager(cacheRoot, gitx.NewClient())

	cachePath, err := manager.Ensure(ctx, remoteURL)
	if err != nil {
		t.Fatalf("ensure cache create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cachePath, "HEAD")); err != nil {
		t.Fatalf("cache HEAD stat: %v", err)
	}

	commit := mutateRemote(t)
	cachePath, err = manager.Ensure(ctx, remoteURL)
	if err != nil {
		t.Fatalf("ensure cache refresh: %v", err)
	}

	sha, err := gitx.NewClient().ResolveCommit(ctx, cachePath, "main")
	if err != nil {
		t.Fatalf("read cache head: %v", err)
	}
	if sha != commit {
		t.Fatalf("cache head = %q, want %q", sha, commit)
	}
}
