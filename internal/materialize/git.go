package materialize

import (
	"context"
	"fmt"
	"os"

	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
)

// BranchResolutionPath describes which git branch resolution path was taken
// during materialization.
type BranchResolutionPath int

const (
	BranchPathLocalExisting  BranchResolutionPath = iota // reused an existing local branch
	BranchPathRemoteTracking                             // created a local tracking branch from origin/<branch>
	BranchPathCreated                                    // created a new branch from From / default branch
	BranchPathHeadless                                   // checked out a ref directly without creating a branch
)

// GitParams holds the inputs for materializing a git source.
type GitParams struct {
	URL    string
	Branch string // desired branch name; empty = default branch
	From   string // base ref for branch creation or direct checkout; empty = use default branch
}

// GitResult holds the outcome of a successful git materialization.
type GitResult struct {
	BranchPath      BranchResolutionPath
	IgnoredFrom     string // non-empty when From was supplied but ignored (branch already existed)
	EffectiveBranch string // the branch name checked out; empty for headless
	EffectiveFrom   string // the base ref used when a new branch was created
}

// Git clones and checks out a git source into destPath using the provided
// cache manager to accelerate cloning.
func Git(ctx context.Context, destPath string, params GitParams, mgr cache.Manager, git gitx.Client) (GitResult, error) {
	cachePath, err := mgr.Ensure(ctx, params.URL)
	if err != nil {
		return GitResult{}, err
	}
	if err := git.Clone(ctx, cachePath, destPath); err != nil {
		return GitResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(destPath)
		}
	}()

	if err := git.RemoteSetURL(ctx, destPath, "origin", params.URL); err != nil {
		return GitResult{}, err
	}

	defaultBranch, err := git.DefaultBranch(ctx, destPath)
	if err != nil {
		return GitResult{}, err
	}

	var (
		resolvedBranch string
		branchPath     BranchResolutionPath
		ignoredFrom    string
		effectiveFrom  string
	)

	if params.Branch != "" {
		if err := git.ValidateBranchName(ctx, params.Branch); err != nil {
			return GitResult{}, err
		}

		localExists, err := git.BranchExists(ctx, destPath, params.Branch)
		if err != nil {
			return GitResult{}, err
		}
		if localExists {
			if params.From != "" {
				ignoredFrom = params.From
			}
			if err := git.Checkout(ctx, destPath, params.Branch); err != nil {
				return GitResult{}, err
			}
			resolvedBranch = params.Branch
			branchPath = BranchPathLocalExisting
		} else {
			remoteRef := fmt.Sprintf("origin/%s", params.Branch)
			_, remoteErr := git.ResolveCommit(ctx, destPath, remoteRef)
			if remoteErr == nil {
				if params.From != "" {
					ignoredFrom = params.From
				}
				if err := git.CreateTrackingBranch(ctx, destPath, params.Branch, remoteRef); err != nil {
					return GitResult{}, err
				}
				resolvedBranch = params.Branch
				branchPath = BranchPathRemoteTracking
			} else {
				base := params.From
				if base == "" {
					base = defaultBranch
				}
				effectiveFrom = base
				if _, err := git.ResolveCommit(ctx, destPath, base); err != nil {
					return GitResult{}, domain.UnresolvedRefError{RepoURL: params.URL, Ref: base}
				}
				if err := git.Checkout(ctx, destPath, base); err != nil {
					return GitResult{}, err
				}
				if err := git.CreateBranch(ctx, destPath, params.Branch); err != nil {
					return GitResult{}, err
				}
				resolvedBranch = params.Branch
				branchPath = BranchPathCreated
			}
		}
	} else if params.From != "" {
		// Direct ref checkout: try locally first, then as a remote branch.
		if _, err := git.ResolveCommit(ctx, destPath, params.From); err != nil {
			remoteRef := fmt.Sprintf("origin/%s", params.From)
			if _, err2 := git.ResolveCommit(ctx, destPath, remoteRef); err2 == nil {
				if err := git.CreateTrackingBranch(ctx, destPath, params.From, remoteRef); err != nil {
					return GitResult{}, err
				}
				resolvedBranch = params.From
				branchPath = BranchPathRemoteTracking
			} else {
				return GitResult{}, domain.UnresolvedRefError{RepoURL: params.URL, Ref: params.From}
			}
		} else {
			if err := git.Checkout(ctx, destPath, params.From); err != nil {
				return GitResult{}, err
			}
			branchPath = BranchPathHeadless
		}
	} else {
		// No branch, no from: check out the default branch.
		if err := git.Checkout(ctx, destPath, defaultBranch); err != nil {
			return GitResult{}, err
		}
		branchPath = BranchPathHeadless
	}

	cleanup = false
	return GitResult{
		BranchPath:      branchPath,
		IgnoredFrom:     ignoredFrom,
		EffectiveBranch: resolvedBranch,
		EffectiveFrom:   effectiveFrom,
	}, nil
}
