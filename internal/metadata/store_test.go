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
	input := domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:      "feature-payments",
			CreatedAt: createdAt,
		},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{{
				Name: "api",
				Type: domain.SourceTypeGit,
				Path: "api",
				Git: &domain.GitSourceSpec{
					URL:    "git@github.com:myorg/api.git",
					Ref:    "main",
					Branch: "feature/payments",
				},
			}},
		},
	}

	if err := store.Save(root, input); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	loaded, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}

	if loaded.APIVersion != input.APIVersion {
		t.Fatalf("loaded apiVersion = %q, want %q", loaded.APIVersion, input.APIVersion)
	}
	if loaded.Kind != input.Kind {
		t.Fatalf("loaded kind = %q, want %q", loaded.Kind, input.Kind)
	}
	if loaded.Metadata.Name != input.Metadata.Name {
		t.Fatalf("loaded name = %q, want %q", loaded.Metadata.Name, input.Metadata.Name)
	}
	if !loaded.Metadata.CreatedAt.Equal(createdAt) {
		t.Fatalf("loaded createdAt = %v, want %v", loaded.Metadata.CreatedAt, createdAt)
	}
	if len(loaded.Spec.Sources) != 1 {
		t.Fatalf("loaded sources = %d, want 1", len(loaded.Spec.Sources))
	}
	src := loaded.Spec.Sources[0]
	want := input.Spec.Sources[0]
	if src.Name != want.Name || src.Type != want.Type || src.Path != want.Path {
		t.Fatalf("loaded source = %#v, want %#v", src, want)
	}
	if src.Git == nil {
		t.Fatal("loaded source.Git is nil")
	}
	if src.Git.URL != want.Git.URL || src.Git.Ref != want.Git.Ref || src.Git.Branch != want.Git.Branch {
		t.Fatalf("loaded source.Git = %#v, want %#v", src.Git, want.Git)
	}

	metadataPath := filepath.Join(root, domain.SpecFileName)
	if metadataPath != store.Path(root) {
		t.Fatalf("metadata path = %q, want %q", store.Path(root), metadataPath)
	}
}
