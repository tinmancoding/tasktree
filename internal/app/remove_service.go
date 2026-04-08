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
	spec, err := s.store.Load(root)
	if err != nil {
		return "", err
	}

	index := -1
	var source domain.SourceSpec
	for i, candidate := range spec.Spec.Sources {
		if candidate.Name == name {
			index = i
			source = candidate
			break
		}
	}
	if index == -1 {
		return "", domain.RepoNotFoundError{Name: name}
	}

	sourcePath := source.Path
	if sourcePath == "" {
		sourcePath = source.Name
	}
	removePath := filepath.Join(root, sourcePath)
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

	spec.Spec.Sources = append(spec.Spec.Sources[:index], spec.Spec.Sources[index+1:]...)
	if err := s.store.Save(root, spec); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}

	return removePath, nil
}
