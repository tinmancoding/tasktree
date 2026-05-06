package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/materialize"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// AddArchiveOptions are the inputs for AddArchiveService.
type AddArchiveOptions struct {
	URL             string
	SHA256          string
	Format          string // tar.gz | tar.bz2 | zip; inferred from URL if empty
	StripComponents int
	Name            string // destination directory name; defaults to last URL path segment
	Path            string // destination path relative to tasktree root; defaults to Name
}

// AddArchiveResult is the outcome of a successful AddArchiveService.Run call.
type AddArchiveResult struct {
	Source domain.SourceSpec
}

// AddArchiveService downloads a remote archive, verifies its digest, extracts
// it into the tasktree, and registers the source in Tasktree.yml.
type AddArchiveService struct {
	store metadata.Store
}

func NewAddArchiveService(store metadata.Store) AddArchiveService {
	return AddArchiveService{store: store}
}

func (s AddArchiveService) Run(ctx context.Context, start string, opts AddArchiveOptions) (AddArchiveResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddArchiveResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return AddArchiveResult{}, err
	}

	name := opts.Name
	if name == "" {
		name = deriveHTTPName(opts.URL) // same URL-basename logic works here
	}
	if err := domain.ValidateSourceName(name); err != nil {
		return AddArchiveResult{}, err
	}
	destRelPath := opts.Path
	if destRelPath == "" {
		destRelPath = name
	}
	for _, src := range spec.Spec.Sources {
		if src.Name == name || src.Path == destRelPath {
			return AddArchiveResult{}, domain.DuplicateSourceNameError{Name: name}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddArchiveResult{}, err
	}
	if exists {
		return AddArchiveResult{}, domain.DestinationExistsError{Path: destPath}
	}

	archiveSpec := &domain.ArchiveSourceSpec{
		URL:             opts.URL,
		SHA256:          opts.SHA256,
		Format:          opts.Format,
		StripComponents: opts.StripComponents,
	}
	if err := materialize.Archive(ctx, destPath, archiveSpec); err != nil {
		return AddArchiveResult{}, err
	}

	source := domain.SourceSpec{
		Name:    name,
		Path:    destRelPath,
		Type:    domain.SourceTypeArchive,
		Archive: archiveSpec,
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddArchiveResult{}, fmt.Errorf("save metadata: %w", err)
	}
	return AddArchiveResult{Source: source}, nil
}
