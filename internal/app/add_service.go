package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type AddOptions struct {
	RepoURL string
	Ref     string
	Branch  string
	Name    string
}

type AddResult struct {
	Root string
	Repo domain.RepoSpec
}

type AddService struct {
	store metadata.Store
	cache cache.Manager
	git   gitx.Client
}

func NewAddService(store metadata.Store, cache cache.Manager, git gitx.Client) AddService {
	return AddService{store: store, cache: cache, git: git}
}

func (s AddService) Run(ctx context.Context, start string, opts AddOptions) (AddResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddResult{}, err
	}
	file, err := s.store.Load(root)
	if err != nil {
		return AddResult{}, err
	}

	repoName := opts.Name
	if repoName == "" {
		repoName, err = domain.DeriveRepoName(opts.RepoURL)
		if err != nil {
			return AddResult{}, err
		}
	}
	if err := domain.ValidateRepoName(repoName); err != nil {
		return AddResult{}, err
	}
	destRelPath := domain.RepoPathForName(repoName)
	for _, repo := range file.Repos {
		if repo.Name == repoName || repo.Path == destRelPath {
			return AddResult{}, domain.DuplicateRepoNameError{Name: repoName}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddResult{}, err
	}
	if exists {
		return AddResult{}, domain.DestinationExistsError{Path: destPath}
	}

	cachePath, err := s.cache.Ensure(ctx, opts.RepoURL)
	if err != nil {
		return AddResult{}, err
	}
	if err := s.git.Clone(ctx, cachePath, destPath); err != nil {
		return AddResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(destPath)
		}
	}()

	if err := s.git.RemoteSetURL(ctx, destPath, "origin", opts.RepoURL); err != nil {
		return AddResult{}, err
	}

	defaultBranch, err := s.git.DefaultBranch(ctx, destPath)
	if err != nil {
		return AddResult{}, err
	}
	requestedCheckout := domain.RequestedCheckout(defaultBranch, opts.Ref)

	if opts.Ref != "" {
		if _, err := s.git.ResolveCommit(ctx, destPath, opts.Ref); err != nil {
			var cmdErr gitx.CommandError
			if errors.As(err, &cmdErr) && cmdErr.ExitCode != 0 {
				return AddResult{}, domain.UnresolvedRefError{RepoURL: opts.RepoURL, Ref: opts.Ref}
			}
			return AddResult{}, err
		}
	}

	if err := s.git.Checkout(ctx, destPath, requestedCheckout); err != nil {
		return AddResult{}, err
	}
	if opts.Branch != "" {
		if err := s.git.ValidateBranchName(ctx, opts.Branch); err != nil {
			return AddResult{}, err
		}
		exists, err := s.git.BranchExists(ctx, destPath, opts.Branch)
		if err != nil {
			return AddResult{}, err
		}
		if exists {
			return AddResult{}, domain.BranchExistsError{Branch: opts.Branch}
		}
		if err := s.git.CreateBranch(ctx, destPath, opts.Branch); err != nil {
			return AddResult{}, err
		}
	}

	commit, err := s.git.CommitSHA(ctx, destPath)
	if err != nil {
		return AddResult{}, err
	}
	resolvedRef, err := s.git.CurrentFullRef(ctx, destPath)
	if err != nil {
		return AddResult{}, err
	}
	if resolvedRef == "" {
		resolvedRef, err = s.git.ResolveFullRef(ctx, destPath, requestedCheckout)
		if err != nil {
			return AddResult{}, err
		}
	}

	repo := domain.RepoSpec{
		Name:        repoName,
		Path:        destRelPath,
		URL:         opts.RepoURL,
		Checkout:    requestedCheckout,
		ResolvedRef: resolvedRef,
		Commit:      commit,
		Branch:      opts.Branch,
	}
	file.Repos = append(file.Repos, repo)
	if err := s.store.Save(root, file); err != nil {
		return AddResult{}, fmt.Errorf("save metadata: %w", err)
	}

	cleanup = false
	return AddResult{Root: root, Repo: repo}, nil
}
