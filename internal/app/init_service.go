package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
)

type InitService struct {
	store    metadata.Store
	registry *registry.Store
	now      func() time.Time
}

func NewInitService(store metadata.Store, reg *registry.Store) InitService {
	return InitService{
		store:    store,
		registry: reg,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s InitService) Run(targetPath string) (string, error) {
	root, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	metadataPath := s.store.Path(root)
	exists, err := fsx.Exists(metadataPath)
	if err != nil {
		return "", fmt.Errorf("check metadata: %w", err)
	}
	if exists {
		return "", domain.MetadataExistsError{Path: metadataPath}
	}

	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:      filepath.Base(root),
			CreatedAt: s.now(),
		},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{},
		},
	}
	if err := s.store.Save(root, spec); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}

	if regErr := s.registry.Register(root, spec.Metadata.Name); regErr != nil {
		// Non-fatal: the tasktree is valid on disk. Warn but do not fail.
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not update registry: %v\n", regErr)
	}

	return root, nil
}
