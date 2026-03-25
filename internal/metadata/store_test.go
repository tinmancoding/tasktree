package metadata_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

func TestStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := metadata.NewStore()
	createdAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	input := domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      "feature-payments",
		CreatedAt: createdAt,
		Repos: []domain.RepoSpec{{
			Name:        "api",
			Path:        "api",
			URL:         "git@github.com:myorg/api.git",
			Checkout:    "main",
			ResolvedRef: "refs/heads/main",
			Commit:      "abc123",
			Branch:      "feature/payments",
		}},
	}

	if err := store.Save(root, input); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	loaded, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}

	if loaded.Version != input.Version || loaded.Name != input.Name || !loaded.CreatedAt.Equal(createdAt) {
		t.Fatalf("loaded top-level fields mismatch: %#v", loaded)
	}
	if len(loaded.Repos) != 1 {
		t.Fatalf("loaded repos = %d, want 1", len(loaded.Repos))
	}
	if loaded.Repos[0] != input.Repos[0] {
		t.Fatalf("loaded repo = %#v, want %#v", loaded.Repos[0], input.Repos[0])
	}

	metadataPath := filepath.Join(root, domain.MetadataFileName)
	if metadataPath != store.Path(root) {
		t.Fatalf("metadata path = %q, want %q", store.Path(root), metadataPath)
	}
}
