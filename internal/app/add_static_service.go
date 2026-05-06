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

// AddStaticOptions are the inputs for AddStaticService.
type AddStaticOptions struct {
	Name    string
	Path    string // destination path relative to tasktree root; defaults to Name
	Content string
	Mode    string // octal string e.g. "0644"; defaults to "0644"
}

// AddStaticResult is the outcome of a successful AddStaticService.Run call.
type AddStaticResult struct {
	Source domain.SourceSpec
}

// AddStaticService writes inline content to a file inside the tasktree and
// registers the source in Tasktree.yml.
type AddStaticService struct {
	store metadata.Store
}

func NewAddStaticService(store metadata.Store) AddStaticService {
	return AddStaticService{store: store}
}

func (s AddStaticService) Run(ctx context.Context, start string, opts AddStaticOptions) (AddStaticResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddStaticResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return AddStaticResult{}, err
	}

	name := opts.Name
	if err := domain.ValidateSourceName(name); err != nil {
		return AddStaticResult{}, err
	}
	destRelPath := opts.Path
	if destRelPath == "" {
		destRelPath = name
	}
	for _, src := range spec.Spec.Sources {
		if src.Name == name || src.Path == destRelPath {
			return AddStaticResult{}, domain.DuplicateSourceNameError{Name: name}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddStaticResult{}, err
	}
	if exists {
		return AddStaticResult{}, domain.DestinationExistsError{Path: destPath}
	}

	staticSpec := &domain.StaticSourceSpec{
		Content: opts.Content,
		Mode:    opts.Mode,
	}
	if err := materialize.Static(destPath, staticSpec); err != nil {
		return AddStaticResult{}, err
	}

	source := domain.SourceSpec{
		Name:   name,
		Path:   destRelPath,
		Type:   domain.SourceTypeStatic,
		Static: staticSpec,
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddStaticResult{}, fmt.Errorf("save metadata: %w", err)
	}
	return AddStaticResult{Source: source}, nil
}
