package app

import (
	"context"
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
	// Branch is the primary branch selection flag. If the branch exists locally
	// it is checked out directly; if it exists only on origin it is tracked; if
	// it does not exist anywhere it is created from From (or the default branch).
	Branch string
	// From is the base ref used when Branch does not yet exist. When Branch is
	// empty, From is checked out directly (headless / tag / SHA workflow).
	From string
	Name string
}

// BranchResolutionPath describes which resolution path was taken so the CLI
// can print an informative message to the user.
type BranchResolutionPath int

const (
	BranchPathLocalExisting  BranchResolutionPath = iota // reused existing local branch
	BranchPathRemoteTracking                             // created local tracking branch from origin/<branch>
	BranchPathCreated                                    // created new branch from From / default
	BranchPathHeadless                                   // checked out From directly, no branch created
)

type AddResult struct {
	Root            string
	Source          domain.SourceSpec
	BranchPath      BranchResolutionPath
	IgnoredFrom     string // non-empty when --from was supplied but ignored
	EffectiveBranch string // the branch name used (empty for headless)
	EffectiveFrom   string // the base ref used for creation (only for BranchPathCreated)
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
	spec, err := s.store.Load(root)
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
	for _, source := range spec.Spec.Sources {
		if source.Name == repoName || source.Path == destRelPath {
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

	// --- Branch resolution ---
	//
	// The result variables we need to populate:
	var (
		resolvedBranch string // the branch name we end up on (empty = headless)
		branchPath     BranchResolutionPath
		ignoredFrom    string
		effectiveFrom  string
	)

	if opts.Branch != "" {
		// --branch provided: validate the name first.
		if err := s.git.ValidateBranchName(ctx, opts.Branch); err != nil {
			return AddResult{}, err
		}

		// 1. Does the local branch already exist?
		localExists, err := s.git.BranchExists(ctx, destPath, opts.Branch)
		if err != nil {
			return AddResult{}, err
		}
		if localExists {
			// Reuse the existing local branch.
			if opts.From != "" {
				ignoredFrom = opts.From
			}
			if err := s.git.Checkout(ctx, destPath, opts.Branch); err != nil {
				return AddResult{}, err
			}
			resolvedBranch = opts.Branch
			branchPath = BranchPathLocalExisting
		} else {
			// 2. Does origin/<branch> exist?
			remoteRef := fmt.Sprintf("origin/%s", opts.Branch)
			_, remoteErr := s.git.ResolveCommit(ctx, destPath, remoteRef)
			if remoteErr == nil {
				// Remote tracking branch exists — create a local tracking branch.
				if opts.From != "" {
					ignoredFrom = opts.From
				}
				if err := s.git.CreateTrackingBranch(ctx, destPath, opts.Branch, remoteRef); err != nil {
					return AddResult{}, err
				}
				resolvedBranch = opts.Branch
				branchPath = BranchPathRemoteTracking
			} else {
				// 3. Neither local nor remote branch exists — create from From (or default branch).
				base := opts.From
				if base == "" {
					base = defaultBranch
				}
				effectiveFrom = base
				// Resolve base ref to make sure it exists.
				if _, err := s.git.ResolveCommit(ctx, destPath, base); err != nil {
					return AddResult{}, domain.UnresolvedRefError{RepoURL: opts.RepoURL, Ref: base}
				}
				// Checkout the base first, then create the new branch.
				if err := s.git.Checkout(ctx, destPath, base); err != nil {
					return AddResult{}, err
				}
				if err := s.git.CreateBranch(ctx, destPath, opts.Branch); err != nil {
					return AddResult{}, err
				}
				resolvedBranch = opts.Branch
				branchPath = BranchPathCreated
			}
		}
	} else {
		// No --branch: check out From directly (headless), or the default branch.
		ref := opts.From
		if ref == "" {
			ref = defaultBranch
		}

		// Determine whether we need a local tracking branch for a remote-only ref.
		if opts.From != "" {
			// Try the ref directly first.
			if _, err := s.git.ResolveCommit(ctx, destPath, ref); err != nil {
				// Try as a remote branch.
				remoteRef := fmt.Sprintf("origin/%s", ref)
				if _, err2 := s.git.ResolveCommit(ctx, destPath, remoteRef); err2 == nil {
					if err := s.git.CreateTrackingBranch(ctx, destPath, ref, remoteRef); err != nil {
						return AddResult{}, err
					}
					resolvedBranch = ref
					branchPath = BranchPathRemoteTracking
				} else {
					return AddResult{}, domain.UnresolvedRefError{RepoURL: opts.RepoURL, Ref: ref}
				}
			} else {
				if err := s.git.Checkout(ctx, destPath, ref); err != nil {
					return AddResult{}, err
				}
				branchPath = BranchPathHeadless
			}
		} else {
			// Default branch checkout.
			if err := s.git.Checkout(ctx, destPath, ref); err != nil {
				return AddResult{}, err
			}
			branchPath = BranchPathHeadless
		}
	}

	// Build the source spec — pure intent, no resolved state.
	// ref field: use opts.From if provided (explicit intent), else opts.Branch, else leave empty (default branch).
	var sourceRef string
	if opts.From != "" {
		sourceRef = opts.From
	} else if opts.Branch != "" {
		sourceRef = opts.Branch
	}

	source := domain.SourceSpec{
		Name: repoName,
		Path: destRelPath,
		Type: domain.SourceTypeGit,
		Git: &domain.GitSourceSpec{
			URL:    opts.RepoURL,
			Ref:    sourceRef,
			Branch: resolvedBranch,
		},
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddResult{}, fmt.Errorf("save metadata: %w", err)
	}

	cleanup = false
	return AddResult{
		Root:            root,
		Source:          source,
		BranchPath:      branchPath,
		IgnoredFrom:     ignoredFrom,
		EffectiveBranch: resolvedBranch,
		EffectiveFrom:   effectiveFrom,
	}, nil
}
