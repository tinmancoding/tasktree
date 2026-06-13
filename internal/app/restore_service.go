package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/bootstrap"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/materialize"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	"github.com/tinmancoding/tasktree/internal/snapshot"
)

// RestoreOptions configures a restore run.
type RestoreOptions struct {
	// Input is the snapshot tar.gz stream.
	Input io.Reader
	// Into is the target directory; empty means ./<tasktree-name>.
	Into string
	// SkipBootstrap materializes/restores without running bootstrap steps.
	SkipBootstrap bool
	// Stderr receives bootstrap step headers and live child output.
	Stderr io.Writer
}

// RestoreResult holds the outcome of a restore run.
type RestoreResult struct {
	Target       string
	Tasktree     string
	BootstrapRan bool
}

// RestoreService reproduces a workspace from a snapshot tar.gz.
type RestoreService struct {
	store     metadata.Store
	cache     cache.Manager
	git       gitx.Client
	registry  *registry.Store
	bootstrap BootstrapRunner
}

func NewRestoreService(store metadata.Store, c cache.Manager, git gitx.Client, reg *registry.Store) RestoreService {
	return RestoreService{store: store, cache: c, git: git, registry: reg, bootstrap: defaultBootstrapRunner{}}
}

// WithBootstrapRunner returns a copy using the given bootstrap runner (tests).
func (s RestoreService) WithBootstrapRunner(r BootstrapRunner) RestoreService {
	s.bootstrap = r
	return s
}

func (s RestoreService) Run(ctx context.Context, start string, opts RestoreOptions) (RestoreResult, error) {
	members, err := snapshot.Open(opts.Input)
	if err != nil {
		return RestoreResult{}, err
	}

	manifestBytes, ok := members[domain.SnapshotManifestName]
	if !ok {
		return RestoreResult{}, fmt.Errorf("snapshot is missing %s", domain.SnapshotManifestName)
	}
	var manifest domain.SnapshotManifest
	if err := yaml.Unmarshal(manifestBytes, &manifest); err != nil {
		return RestoreResult{}, fmt.Errorf("parse snapshot manifest: %w", err)
	}
	if err := domain.ValidateManifest(manifest); err != nil {
		return RestoreResult{}, err
	}

	specBytes, ok := members[domain.SpecFileName]
	if !ok {
		return RestoreResult{}, fmt.Errorf("snapshot is missing embedded %s", domain.SpecFileName)
	}
	var spec domain.TasktreeSpec
	if err := yaml.Unmarshal(specBytes, &spec); err != nil {
		return RestoreResult{}, fmt.Errorf("parse embedded spec: %w", err)
	}

	target := opts.Into
	if target == "" {
		base := manifest.Tasktree
		if base == "" {
			base = "restored-tasktree"
		}
		target = filepath.Join(start, base)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := ensureEmptyTarget(target); err != nil {
		return RestoreResult{}, err
	}

	// Stage on the same filesystem as the target's parent for an atomic rename.
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return RestoreResult{}, err
	}
	staging, err := os.MkdirTemp(parent, ".tasktree-restore-*")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("create staging dir: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	// Write the spec into staging.
	if err := os.WriteFile(filepath.Join(staging, domain.SpecFileName), specBytes, 0o644); err != nil {
		return RestoreResult{}, fmt.Errorf("write spec: %w", err)
	}

	gitEntries := make(map[string]*domain.GitSubSnapshot, len(manifest.Sources))
	for _, e := range manifest.Sources {
		if e.Git != nil {
			gitEntries[e.Name] = e.Git
		}
	}

	for _, source := range spec.Spec.Sources {
		relPath := source.Path
		if relPath == "" {
			relPath = source.Name
		}
		destPath := filepath.Join(staging, relPath)

		if source.Type == domain.SourceTypeGit {
			gs := gitEntries[source.Name]
			if gs == nil {
				return RestoreResult{}, fmt.Errorf("source %q: git source missing from snapshot manifest", source.Name)
			}
			if err := s.restoreGit(ctx, destPath, source.Name, gs, members); err != nil {
				return RestoreResult{}, fmt.Errorf("source %q: %w", source.Name, err)
			}
			continue
		}
		if err := s.materializeNonGit(ctx, staging, destPath, source); err != nil {
			return RestoreResult{}, fmt.Errorf("source %q: %w", source.Name, err)
		}
	}

	// Atomic commit.
	if err := os.Rename(staging, target); err != nil {
		return RestoreResult{}, fmt.Errorf("finalize restore: %w", err)
	}
	committed = true

	if regErr := s.registry.Register(target, manifest.Tasktree); regErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not update registry: %v\n", regErr)
	}

	result := RestoreResult{Target: target, Tasktree: manifest.Tasktree}

	steps := spec.Spec.Bootstrap
	if opts.SkipBootstrap || len(steps) == 0 {
		return result, nil
	}
	if err := domain.ValidateBootstrap(steps); err != nil {
		return result, err
	}
	if err := s.bootstrap.Run(ctx, target, steps, bootstrap.Options{Stderr: opts.Stderr}); err != nil {
		return result, err
	}
	result.BootstrapRan = true
	return result, nil
}

func (s RestoreService) restoreGit(ctx context.Context, destPath, name string, gs *domain.GitSubSnapshot, members map[string][]byte) error {
	// Clone via cache, then point origin at the real remote.
	cachePath, err := s.cache.Ensure(ctx, gs.RemoteURL)
	if err != nil {
		return err
	}
	if err := s.git.Clone(ctx, cachePath, destPath); err != nil {
		return err
	}
	if err := s.git.RemoteSetURL(ctx, destPath, "origin", gs.RemoteURL); err != nil {
		return err
	}

	// Ensure base is present (fetch from remote if the cache lacks it).
	hasBase, err := s.git.HasCommit(ctx, destPath, gs.BaseSHA)
	if err != nil {
		return err
	}
	if !hasBase {
		if err := s.git.FetchSHA(ctx, destPath, "origin", gs.BaseSHA); err != nil {
			return err
		}
	}

	// Replay local commits from the bundle.
	if gs.Bundle != "" {
		bundleBytes, ok := members[gs.Bundle]
		if !ok {
			return fmt.Errorf("snapshot missing bundle %q", gs.Bundle)
		}
		bundlePath, cleanup, err := writeTempBytes(bundleBytes, "tasktree-bundle-*.bundle")
		if err != nil {
			return err
		}
		defer cleanup()
		if err := s.git.FetchBundle(ctx, destPath, bundlePath); err != nil {
			return err
		}
	}

	// Recreate branch identity at headSHA.
	if gs.Detached {
		if err := s.git.CheckoutDetached(ctx, destPath, gs.HeadSHA); err != nil {
			return err
		}
	} else {
		if err := s.git.CreateBranchAt(ctx, destPath, gs.Branch, gs.HeadSHA); err != nil {
			return err
		}
	}

	// Verify HEAD matches.
	head, err := s.git.CommitSHA(ctx, destPath)
	if err != nil {
		return err
	}
	if head != gs.HeadSHA {
		return domain.HeadMismatchError{Name: name, Want: gs.HeadSHA, Got: head}
	}

	// Restore dirty state.
	if gs.Dirty != "" {
		dirtyBytes, ok := members[gs.Dirty]
		if !ok {
			return fmt.Errorf("snapshot missing dirty archive %q", gs.Dirty)
		}
		dm, err := snapshot.UnpackDirtyTar(destPath, dirtyBytes)
		if err != nil {
			return err
		}
		for _, rel := range dm.Deleted {
			if err := os.RemoveAll(filepath.Join(destPath, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("apply deletion %q: %w", rel, err)
			}
		}
		if err := s.git.AddPaths(ctx, destPath, dm.Staged); err != nil {
			return err
		}
	}
	return nil
}

func (s RestoreService) materializeNonGit(ctx context.Context, root, destPath string, source domain.SourceSpec) error {
	switch source.Type {
	case domain.SourceTypeStatic:
		if source.Static == nil {
			return domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
		}
		return materialize.Static(destPath, source.Static)
	case domain.SourceTypeLocal:
		if source.Local == nil {
			return domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
		}
		return materialize.Local(root, destPath, source.Local)
	case domain.SourceTypeHTTP:
		if source.HTTP == nil {
			return domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
		}
		return materialize.HTTP(ctx, destPath, source.HTTP)
	case domain.SourceTypeArchive:
		if source.Archive == nil {
			return domain.MissingSourceSpecError{Name: source.Name, Type: source.Type}
		}
		return materialize.Archive(ctx, destPath, source.Archive)
	default:
		return domain.UnknownSourceTypeError{Type: source.Type}
	}
}

func ensureEmptyTarget(target string) error {
	entries, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return domain.NonEmptyRestoreTargetError{Path: target}
	}
	return nil
}

func writeTempBytes(data []byte, pattern string) (string, func(), error) {
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	path := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
