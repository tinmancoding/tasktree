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

// ApplyOptions holds runtime options for ApplyService.Run.
type ApplyOptions struct {
	DryRun bool
}

// SourceApplyStatus describes what happened when applying a single source.
type SourceApplyStatus string

const (
	// SourceApplyStatusCloned indicates the source was materialized successfully.
	SourceApplyStatusCloned SourceApplyStatus = "cloned"
	// SourceApplyStatusSkipped indicates the destination already exists on disk.
	SourceApplyStatusSkipped SourceApplyStatus = "skipped"
	// SourceApplyStatusWouldClone indicates the source would be cloned (dry-run only).
	SourceApplyStatusWouldClone SourceApplyStatus = "would-clone"
	// SourceApplyStatusUnsupported indicates the source type is not yet implemented.
	SourceApplyStatusUnsupported SourceApplyStatus = "unsupported"
)

// SourceApplyResult holds the outcome for a single source entry.
type SourceApplyResult struct {
	Source          domain.SourceSpec
	Status          SourceApplyStatus
	BranchPath      BranchResolutionPath // meaningful only when Status == SourceApplyStatusCloned
	EffectiveBranch string               // live branch name after checkout; empty for headless
	EffectiveFrom   string               // base ref used when a branch was created
}

// ApplyResult holds the overall outcome of an apply run.
type ApplyResult struct {
	Root    string
	Results []SourceApplyResult
}

// ApplyService materializes sources declared in Tasktree.yml that are not yet
// present on disk.
type ApplyService struct {
	store metadata.Store
	cache cache.Manager
	git   gitx.Client
}

func NewApplyService(store metadata.Store, cache cache.Manager, git gitx.Client) ApplyService {
	return ApplyService{store: store, cache: cache, git: git}
}

func (s ApplyService) Run(ctx context.Context, start string, opts ApplyOptions) (ApplyResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return ApplyResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return ApplyResult{}, err
	}

	results := make([]SourceApplyResult, 0, len(spec.Spec.Sources))
	for _, source := range spec.Spec.Sources {
		result, err := s.applySource(ctx, root, source, opts)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("source %q: %w", source.Name, err)
		}
		results = append(results, result)
	}

	return ApplyResult{Root: root, Results: results}, nil
}

func (s ApplyService) applySource(ctx context.Context, root string, source domain.SourceSpec, opts ApplyOptions) (SourceApplyResult, error) {
	destRelPath := source.Path
	if destRelPath == "" {
		destRelPath = source.Name
	}
	destPath := filepath.Join(root, destRelPath)

	// If already present on disk, skip without error.
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return SourceApplyResult{}, err
	}
	if exists {
		return SourceApplyResult{Source: source, Status: SourceApplyStatusSkipped}, nil
	}

	// Dry-run: report intent only, do not touch the filesystem.
	if opts.DryRun {
		return SourceApplyResult{Source: source, Status: SourceApplyStatusWouldClone}, nil
	}

	// Only the git source type is implemented in v1.
	if source.Type != domain.SourceTypeGit {
		return SourceApplyResult{Source: source, Status: SourceApplyStatusUnsupported}, nil
	}
	if source.Git == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}

	branchPath, effectiveBranch, effectiveFrom, err := s.materializeGitSource(ctx, destPath, source.Git)
	if err != nil {
		return SourceApplyResult{}, err
	}
	return SourceApplyResult{
		Source:          source,
		Status:          SourceApplyStatusCloned,
		BranchPath:      branchPath,
		EffectiveBranch: effectiveBranch,
		EffectiveFrom:   effectiveFrom,
	}, nil
}

// materializeGitSource clones and checks out a git source. It is the apply
// equivalent of the clone+checkout block in AddService.Run.
func (s ApplyService) materializeGitSource(ctx context.Context, destPath string, g *domain.GitSourceSpec) (BranchResolutionPath, string, string, error) {
	cachePath, err := s.cache.Ensure(ctx, g.URL)
	if err != nil {
		return 0, "", "", err
	}
	if err := s.git.Clone(ctx, cachePath, destPath); err != nil {
		return 0, "", "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(destPath)
		}
	}()

	if err := s.git.RemoteSetURL(ctx, destPath, "origin", g.URL); err != nil {
		return 0, "", "", err
	}

	defaultBranch, err := s.git.DefaultBranch(ctx, destPath)
	if err != nil {
		return 0, "", "", err
	}

	// Reconstruct the original --from intent from the stored spec fields.
	//
	// AddService writes the spec as follows:
	//   --from F  --branch B  → Ref=F,         Branch=B  (F is the explicit base)
	//   --branch B            → Ref=B,         Branch=B  (Ref == Branch: no explicit from)
	//   --from F              → Ref=F,         Branch="" (direct ref checkout)
	//   (neither)             → Ref="",        Branch="" (default branch)
	//
	// When Ref == Branch, no explicit --from was provided; use "" so the branch
	// resolution falls back to the repo default branch.
	fromRef := g.Ref
	if fromRef == g.Branch {
		fromRef = ""
	}

	var (
		resolvedBranch string
		branchPath     BranchResolutionPath
		effectiveFrom  string
	)

	switch {
	case g.Branch != "":
		// A specific branch was requested.
		localExists, err := s.git.BranchExists(ctx, destPath, g.Branch)
		if err != nil {
			return 0, "", "", err
		}
		if localExists {
			if err := s.git.Checkout(ctx, destPath, g.Branch); err != nil {
				return 0, "", "", err
			}
			resolvedBranch = g.Branch
			branchPath = BranchPathLocalExisting
		} else {
			remoteRef := fmt.Sprintf("origin/%s", g.Branch)
			_, remoteErr := s.git.ResolveCommit(ctx, destPath, remoteRef)
			if remoteErr == nil {
				if err := s.git.CreateTrackingBranch(ctx, destPath, g.Branch, remoteRef); err != nil {
					return 0, "", "", err
				}
				resolvedBranch = g.Branch
				branchPath = BranchPathRemoteTracking
			} else {
				base := fromRef
				if base == "" {
					base = defaultBranch
				}
				effectiveFrom = base
				if _, err := s.git.ResolveCommit(ctx, destPath, base); err != nil {
					return 0, "", "", domain.UnresolvedRefError{RepoURL: g.URL, Ref: base}
				}
				if err := s.git.Checkout(ctx, destPath, base); err != nil {
					return 0, "", "", err
				}
				if err := s.git.CreateBranch(ctx, destPath, g.Branch); err != nil {
					return 0, "", "", err
				}
				resolvedBranch = g.Branch
				branchPath = BranchPathCreated
			}
		}

	case g.Ref != "":
		// Direct ref checkout (tag, commit, or remote-only branch via --from without --branch).
		if _, err := s.git.ResolveCommit(ctx, destPath, g.Ref); err != nil {
			// Not a local/tag ref — try as a remote branch.
			remoteRef := fmt.Sprintf("origin/%s", g.Ref)
			if _, err2 := s.git.ResolveCommit(ctx, destPath, remoteRef); err2 == nil {
				if err := s.git.CreateTrackingBranch(ctx, destPath, g.Ref, remoteRef); err != nil {
					return 0, "", "", err
				}
				resolvedBranch = g.Ref
				branchPath = BranchPathRemoteTracking
			} else {
				return 0, "", "", domain.UnresolvedRefError{RepoURL: g.URL, Ref: g.Ref}
			}
		} else {
			if err := s.git.Checkout(ctx, destPath, g.Ref); err != nil {
				return 0, "", "", err
			}
			branchPath = BranchPathHeadless
		}

	default:
		// No branch, no ref: check out the default branch.
		if err := s.git.Checkout(ctx, destPath, defaultBranch); err != nil {
			return 0, "", "", err
		}
		branchPath = BranchPathHeadless
	}

	cleanup = false
	return branchPath, resolvedBranch, effectiveFrom, nil
}
