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

func (s ListService) Run(start string) (string, domain.TasktreeSpec, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return "", domain.TasktreeSpec{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return "", domain.TasktreeSpec{}, err
	}
	return root, spec, nil
}
