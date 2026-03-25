package app

import "github.com/tinmancoding/tasktree/internal/fsx"

type RootService struct{}

func NewRootService() RootService {
	return RootService{}
}

func (s RootService) Run(start string) (string, error) {
	return fsx.ResolveTasktreeRoot(start)
}
