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
	file, err := s.store.Load(root)
	if err != nil {
		return StatusResult{}, err
	}

	result := StatusResult{TasktreeName: file.Name, Root: root, Repos: make([]RepoStatus, 0, len(file.Repos))}
	for _, repo := range file.Repos {
		repoPath := filepath.Join(root, repo.Path)
		branch, err := s.git.CurrentBranch(ctx, repoPath)
		if err != nil {
			return StatusResult{}, fmt.Errorf("inspect branch for %s: %w", repo.Name, err)
		}
		dirty, err := s.git.IsDirty(ctx, repoPath)
		if err != nil {
			return StatusResult{}, fmt.Errorf("inspect status for %s: %w", repo.Name, err)
		}

		head := branch
		state := "clean"
		if dirty {
			state = "modified"
		}
		if branch == "" {
			head, err = s.git.HeadDescription(ctx, repoPath)
			if err != nil {
				return StatusResult{}, fmt.Errorf("inspect detached HEAD for %s: %w", repo.Name, err)
			}
			if dirty {
				state = "detached, modified"
			} else {
				state = "detached, clean"
			}
		}

		result.Repos = append(result.Repos, RepoStatus{
			Name:  repo.Name,
			Path:  repo.Path,
			Head:  head,
			State: state,
		})
	}

	return result, nil
}
