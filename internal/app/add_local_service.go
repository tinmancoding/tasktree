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

// AddLocalOptions are the inputs for AddLocalService.
type AddLocalOptions struct {
	SourcePath string // path on the local filesystem to link or copy
	Name       string // destination name; defaults to the base of SourcePath
	Path       string // destination path relative to tasktree root; defaults to Name
	Copy       bool   // if true, copy instead of symlinking
}

// AddLocalResult is the outcome of a successful AddLocalService.Run call.
type AddLocalResult struct {
	Source    domain.SourceSpec
	Symlinked bool // true when a symlink was created; false when a copy was made
}

// AddLocalService symlinks or copies a local filesystem path into the tasktree
// and registers the source in Tasktree.yml.
type AddLocalService struct {
	store metadata.Store
}

func NewAddLocalService(store metadata.Store) AddLocalService {
	return AddLocalService{store: store}
}

func (s AddLocalService) Run(ctx context.Context, start string, opts AddLocalOptions) (AddLocalResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddLocalResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return AddLocalResult{}, err
	}

	name := opts.Name
	if name == "" {
		name = filepath.Base(opts.SourcePath)
	}
	if err := domain.ValidateSourceName(name); err != nil {
		return AddLocalResult{}, err
	}
	destRelPath := opts.Path
	if destRelPath == "" {
		destRelPath = name
	}
	for _, src := range spec.Spec.Sources {
		if src.Name == name || src.Path == destRelPath {
			return AddLocalResult{}, domain.DuplicateSourceNameError{Name: name}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddLocalResult{}, err
	}
	if exists {
		return AddLocalResult{}, domain.DestinationExistsError{Path: destPath}
	}

	localSpec := &domain.LocalSourceSpec{
		SourcePath: opts.SourcePath,
		Copy:       opts.Copy,
	}
	if err := materialize.Local(root, destPath, localSpec); err != nil {
		return AddLocalResult{}, err
	}

	source := domain.SourceSpec{
		Name:  name,
		Path:  destRelPath,
		Type:  domain.SourceTypeLocal,
		Local: localSpec,
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddLocalResult{}, fmt.Errorf("save metadata: %w", err)
	}
	return AddLocalResult{Source: source, Symlinked: !opts.Copy}, nil
}
