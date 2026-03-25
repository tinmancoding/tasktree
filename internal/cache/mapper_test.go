package cache_test

import (
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/cache"
)

func TestPathForURLIsDeterministic(t *testing.T) {
	root := "/tmp/tasktree-cache"
	url := "git@github.com:myorg/api.git"
	first := cache.PathForURL(root, url)
	second := cache.PathForURL(root, url)
	if first != second {
		t.Fatalf("cache path mismatch: %q != %q", first, second)
	}
	if filepath.Dir(first) != root {
		t.Fatalf("cache path root = %q, want %q", filepath.Dir(first), root)
	}
}
