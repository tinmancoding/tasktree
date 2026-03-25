package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type InitService struct {
	store metadata.Store
	now   func() time.Time
}

func NewInitService(store metadata.Store) InitService {
	return InitService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
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

	file := domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: s.now(),
		Repos:     []domain.RepoSpec{},
	}
	if err := s.store.Save(root, file); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}

	return root, nil
}
