package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type RemoveService struct {
	store metadata.Store
}

func NewRemoveService(store metadata.Store) RemoveService {
	return RemoveService{store: store}
}

func (s RemoveService) Run(start, name string) (string, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return "", err
	}
	file, err := s.store.Load(root)
	if err != nil {
		return "", err
	}

	index := -1
	var repo domain.RepoSpec
	for i, candidate := range file.Repos {
		if candidate.Name == name {
			index = i
			repo = candidate
			break
		}
	}
	if index == -1 {
		return "", domain.RepoNotFoundError{Name: name}
	}

	removePath := filepath.Join(root, repo.Path)
	within, err := fsx.IsWithin(root, removePath)
	if err != nil {
		return "", err
	}
	if !within {
		return "", domain.UnsafePathError{Path: removePath}
	}
	if err := os.RemoveAll(removePath); err != nil {
		return "", fmt.Errorf("remove repo directory: %w", err)
	}

	file.Repos = append(file.Repos[:index], file.Repos[index+1:]...)
	if err := s.store.Save(root, file); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}

	return removePath, nil
}
