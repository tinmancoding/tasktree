package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type RepoStatus struct {
	Name  string
	Path  string
	Head  string
	State string
}

type StatusResult struct {
	TasktreeName string
	Root         string
	Annotations  map[string]string
	Repos        []RepoStatus
}

type StatusService struct {
	store metadata.Store
	git   gitx.Client
}

func NewStatusService(store metadata.Store, git gitx.Client) StatusService {
	return StatusService{store: store, git: git}
}

func (s StatusService) Run(ctx context.Context, start string) (StatusResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return StatusResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return StatusResult{}, err
	}

	result := StatusResult{TasktreeName: spec.Metadata.Name, Root: root, Annotations: spec.Metadata.Annotations, Repos: make([]RepoStatus, 0, len(spec.Spec.Sources))}
	for _, source := range spec.Spec.Sources {
		sourcePath := source.Path
		if sourcePath == "" {
			sourcePath = source.Name
		}
		repoPath := filepath.Join(root, sourcePath)
		branch, err := s.git.CurrentBranch(ctx, repoPath)
		if err != nil {
			return StatusResult{}, fmt.Errorf("inspect branch for %s: %w", source.Name, err)
		}
		dirty, err := s.git.IsDirty(ctx, repoPath)
		if err != nil {
			return StatusResult{}, fmt.Errorf("inspect status for %s: %w", source.Name, err)
		}

		head := branch
		state := "clean"
		if dirty {
			state = "modified"
		}
		if branch == "" {
			head, err = s.git.HeadDescription(ctx, repoPath)
			if err != nil {
				return StatusResult{}, fmt.Errorf("inspect detached HEAD for %s: %w", source.Name, err)
			}
			if dirty {
				state = "detached, modified"
			} else {
				state = "detached, clean"
			}
		}

		result.Repos = append(result.Repos, RepoStatus{
			Name:  source.Name,
			Path:  sourcePath,
			Head:  head,
			State: state,
		})
	}

	return result, nil
}
