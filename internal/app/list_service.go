package app

import (
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type ListService struct {
	store metadata.Store
}

func NewListService(store metadata.Store) ListService {
	return ListService{store: store}
}

func (s ListService) Run(start string) (string, domain.TasktreeFile, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return "", domain.TasktreeFile{}, err
	}
	file, err := s.store.Load(root)
	if err != nil {
		return "", domain.TasktreeFile{}, err
	}
	return root, file, nil
}
