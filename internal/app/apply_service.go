package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/bootstrap"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/materialize"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// ApplyOptions holds runtime options for ApplyService.Run.
type ApplyOptions struct {
	DryRun        bool
	SkipBootstrap bool
	// Stderr receives bootstrap step headers and live child output. If nil,
	// the executor falls back to os.Stderr.
	Stderr io.Writer
}

// SourceApplyStatus describes what happened when applying a single source.
type SourceApplyStatus string

const (
	// SourceApplyStatusCloned indicates a git source was cloned successfully.
	SourceApplyStatusCloned SourceApplyStatus = "cloned"
	// SourceApplyStatusCreated indicates a static source was written successfully.
	SourceApplyStatusCreated SourceApplyStatus = "created"
	// SourceApplyStatusLinked indicates a local source was symlinked successfully.
	SourceApplyStatusLinked SourceApplyStatus = "linked"
	// SourceApplyStatusCopied indicates a local source was copied successfully.
	SourceApplyStatusCopied SourceApplyStatus = "copied"
	// SourceApplyStatusDownloaded indicates an http source was downloaded successfully.
	SourceApplyStatusDownloaded SourceApplyStatus = "downloaded"
	// SourceApplyStatusExtracted indicates an archive source was extracted successfully.
	SourceApplyStatusExtracted SourceApplyStatus = "extracted"
	// SourceApplyStatusSkipped indicates the destination already exists on disk.
	SourceApplyStatusSkipped SourceApplyStatus = "skipped"
	// SourceApplyStatusWouldApply indicates the source would be materialized (dry-run only).
	SourceApplyStatusWouldApply SourceApplyStatus = "would-apply"
	// SourceApplyStatusUnsupported indicates the source type is unrecognised.
	SourceApplyStatusUnsupported SourceApplyStatus = "unsupported"
)

// SourceApplyStatusWouldClone is a deprecated alias for SourceApplyStatusWouldApply.
// It is kept for backward compatibility with existing tests.
const SourceApplyStatusWouldClone = SourceApplyStatusWouldApply

// SourceApplyResult holds the outcome for a single source entry.
type SourceApplyResult struct {
	Source          domain.SourceSpec
	Status          SourceApplyStatus
	BranchPath      BranchResolutionPath // meaningful only for git sources
	EffectiveBranch string               // live branch name after checkout; empty for headless
	EffectiveFrom   string               // base ref used when a branch was created
}

// ApplyResult holds the overall outcome of an apply run.
type ApplyResult struct {
	Root          string
	Results       []SourceApplyResult
	BootstrapPlan []bootstrap.PlanStep // populated on dry-run when bootstrap steps exist
	BootstrapRan  bool                 // true when bootstrap steps were executed
}

// BootstrapRunner executes bootstrap steps. It is an interface so tests can
// substitute a fake; the default wraps bootstrap.Run.
type BootstrapRunner interface {
	Run(ctx context.Context, root string, steps []domain.BootstrapStep, opts bootstrap.Options) error
}

type defaultBootstrapRunner struct{}

func (defaultBootstrapRunner) Run(ctx context.Context, root string, steps []domain.BootstrapStep, opts bootstrap.Options) error {
	return bootstrap.Run(ctx, root, steps, opts)
}

// ApplyService materializes sources declared in Tasktree.yml that are not yet
// present on disk.
type ApplyService struct {
	store     metadata.Store
	cache     cache.Manager
	git       gitx.Client
	bootstrap BootstrapRunner
}

func NewApplyService(store metadata.Store, cache cache.Manager, git gitx.Client) ApplyService {
	return ApplyService{store: store, cache: cache, git: git, bootstrap: defaultBootstrapRunner{}}
}

// WithBootstrapRunner returns a copy of the service using the given bootstrap
// runner. Intended for tests.
func (s ApplyService) WithBootstrapRunner(r BootstrapRunner) ApplyService {
	s.bootstrap = r
	return s
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

	out := ApplyResult{Root: root, Results: results}

	steps := spec.Spec.Bootstrap
	if opts.SkipBootstrap || len(steps) == 0 {
		return out, nil
	}

	// Runtime validation gate (Load does not validate); fail before executing.
	if err := domain.ValidateBootstrap(steps); err != nil {
		return ApplyResult{}, err
	}

	if opts.DryRun {
		out.BootstrapPlan = bootstrap.Plan(root, steps)
		return out, nil
	}

	if err := s.bootstrap.Run(ctx, root, steps, bootstrap.Options{Stderr: opts.Stderr}); err != nil {
		return ApplyResult{}, err
	}
	out.BootstrapRan = true
	return out, nil
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
		return SourceApplyResult{Source: source, Status: SourceApplyStatusWouldApply}, nil
	}

	switch source.Type {
	case domain.SourceTypeGit:
		return s.applyGitSource(ctx, destPath, source)
	case domain.SourceTypeStatic:
		return s.applyStaticSource(destPath, source)
	case domain.SourceTypeLocal:
		return s.applyLocalSource(root, destPath, source)
	case domain.SourceTypeHTTP:
		return s.applyHTTPSource(ctx, destPath, source)
	case domain.SourceTypeArchive:
		return s.applyArchiveSource(ctx, destPath, source)
	default:
		return SourceApplyResult{Source: source, Status: SourceApplyStatusUnsupported}, nil
	}
}

func (s ApplyService) applyGitSource(ctx context.Context, destPath string, source domain.SourceSpec) (SourceApplyResult, error) {
	if source.Git == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}

	// Reconstruct the original --from intent from the stored spec fields.
	//
	// AddGitService writes the spec as follows:
	//   --from F  --branch B  → Ref=F,         Branch=B  (F is the explicit base)
	//   --branch B            → Ref=B,         Branch=B  (Ref == Branch: no explicit from)
	//   --from F              → Ref=F,         Branch="" (direct ref checkout)
	//   (neither)             → Ref="",        Branch="" (default branch)
	//
	// When Ref == Branch, no explicit --from was provided; use "" so the branch
	// resolution falls back to the repo default branch.
	fromRef := source.Git.Ref
	if fromRef == source.Git.Branch {
		fromRef = ""
	}

	gitResult, err := materialize.Git(ctx, destPath, materialize.GitParams{
		URL:    source.Git.URL,
		Branch: source.Git.Branch,
		From:   fromRef,
	}, s.cache, s.git)
	if err != nil {
		return SourceApplyResult{}, err
	}

	return SourceApplyResult{
		Source:          source,
		Status:          SourceApplyStatusCloned,
		BranchPath:      gitResult.BranchPath,
		EffectiveBranch: gitResult.EffectiveBranch,
		EffectiveFrom:   gitResult.EffectiveFrom,
	}, nil
}

func (s ApplyService) applyStaticSource(destPath string, source domain.SourceSpec) (SourceApplyResult, error) {
	if source.Static == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}
	if err := materialize.Static(destPath, source.Static); err != nil {
		return SourceApplyResult{}, err
	}
	return SourceApplyResult{Source: source, Status: SourceApplyStatusCreated}, nil
}

func (s ApplyService) applyLocalSource(root, destPath string, source domain.SourceSpec) (SourceApplyResult, error) {
	if source.Local == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}
	if err := materialize.Local(root, destPath, source.Local); err != nil {
		return SourceApplyResult{}, err
	}
	status := SourceApplyStatusLinked
	if source.Local.Copy {
		status = SourceApplyStatusCopied
	}
	return SourceApplyResult{Source: source, Status: status}, nil
}

func (s ApplyService) applyHTTPSource(ctx context.Context, destPath string, source domain.SourceSpec) (SourceApplyResult, error) {
	if source.HTTP == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}
	if err := materialize.HTTP(ctx, destPath, source.HTTP); err != nil {
		return SourceApplyResult{}, err
	}
	return SourceApplyResult{Source: source, Status: SourceApplyStatusDownloaded}, nil
}

func (s ApplyService) applyArchiveSource(ctx context.Context, destPath string, source domain.SourceSpec) (SourceApplyResult, error) {
	if source.Archive == nil {
		return SourceApplyResult{}, domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
	}
	if err := materialize.Archive(ctx, destPath, source.Archive); err != nil {
		return SourceApplyResult{}, err
	}
	return SourceApplyResult{Source: source, Status: SourceApplyStatusExtracted}, nil
}
