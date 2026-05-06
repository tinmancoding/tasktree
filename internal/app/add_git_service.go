package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/materialize"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// BranchResolutionPath is re-exported from the materialize package so that
// callers that only import app do not need to import materialize directly.
type BranchResolutionPath = materialize.BranchResolutionPath

const (
	BranchPathLocalExisting  = materialize.BranchPathLocalExisting
	BranchPathRemoteTracking = materialize.BranchPathRemoteTracking
	BranchPathCreated        = materialize.BranchPathCreated
	BranchPathHeadless       = materialize.BranchPathHeadless
)

// AddGitOptions are the inputs for AddGitService.
type AddGitOptions struct {
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

// AddGitResult is the outcome of a successful AddGitService.Run call.
type AddGitResult struct {
	Root            string
	Source          domain.SourceSpec
	BranchPath      BranchResolutionPath
	IgnoredFrom     string // non-empty when --from was supplied but ignored
	EffectiveBranch string // the branch name used (empty for headless)
	EffectiveFrom   string // the base ref used for creation (only for BranchPathCreated)
}

// AddGitService clones a git repository, checks out the requested branch or
// ref, and registers the source in Tasktree.yml.
type AddGitService struct {
	store metadata.Store
	cache cache.Manager
	git   gitx.Client
}

func NewAddGitService(store metadata.Store, cache cache.Manager, git gitx.Client) AddGitService {
	return AddGitService{store: store, cache: cache, git: git}
}

func (s AddGitService) Run(ctx context.Context, start string, opts AddGitOptions) (AddGitResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddGitResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return AddGitResult{}, err
	}

	repoName := opts.Name
	if repoName == "" {
		repoName, err = domain.DeriveRepoName(opts.RepoURL)
		if err != nil {
			return AddGitResult{}, err
		}
	}
	if err := domain.ValidateRepoName(repoName); err != nil {
		return AddGitResult{}, err
	}
	destRelPath := domain.RepoPathForName(repoName)
	for _, source := range spec.Spec.Sources {
		if source.Name == repoName || source.Path == destRelPath {
			return AddGitResult{}, domain.DuplicateRepoNameError{Name: repoName}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddGitResult{}, err
	}
	if exists {
		return AddGitResult{}, domain.DestinationExistsError{Path: destPath}
	}

	gitResult, err := materialize.Git(ctx, destPath, materialize.GitParams{
		URL:    opts.RepoURL,
		Branch: opts.Branch,
		From:   opts.From,
	}, s.cache, s.git)
	if err != nil {
		return AddGitResult{}, err
	}

	// Build the source spec — pure intent, no resolved state.
	// Ref field: use From if provided (explicit intent), else Branch, else empty (default branch).
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
			Branch: gitResult.EffectiveBranch,
		},
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddGitResult{}, fmt.Errorf("save metadata: %w", err)
	}

	return AddGitResult{
		Root:            root,
		Source:          source,
		BranchPath:      gitResult.BranchPath,
		IgnoredFrom:     gitResult.IgnoredFrom,
		EffectiveBranch: gitResult.EffectiveBranch,
		EffectiveFrom:   gitResult.EffectiveFrom,
	}, nil
}

// --- Backward-compatibility aliases so existing code using AddService /
//     AddOptions / AddResult / NewAddService compiles without changes. ---

// AddService is an alias for AddGitService kept for backward compatibility.
type AddService = AddGitService

// AddOptions is an alias for AddGitOptions kept for backward compatibility.
type AddOptions = AddGitOptions

// AddResult is an alias for AddGitResult kept for backward compatibility.
type AddResult = AddGitResult

// NewAddService is an alias for NewAddGitService kept for backward compatibility.
func NewAddService(store metadata.Store, cache cache.Manager, git gitx.Client) AddGitService {
	return NewAddGitService(store, cache, git)
}
