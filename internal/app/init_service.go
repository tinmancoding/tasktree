package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
)

type InitService struct {
	store    metadata.Store
	registry *registry.Store
	now      func() time.Time
}

func NewInitService(store metadata.Store, reg *registry.Store) InitService {
	return InitService{
		store:    store,
		registry: reg,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// InitOptions carries optional configuration for workspace initialization.
type InitOptions struct {
	// Annotations is an optional set of annotation key/value pairs to store in
	// the workspace metadata at creation time. Keys must satisfy
	// domain.ValidateAnnotationKey. A nil map means no annotations.
	Annotations map[string]string
}

func (s InitService) Run(targetPath string, opts InitOptions) (string, error) {
	root, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	metadataPath := s.store.Path(root)
	exists, err := fsx.Exists(metadataPath)
	if err != nil {
		return "", fmt.Errorf("check metadata: %w", err)
	}
	if exists {
		return "", domain.MetadataExistsError{Path: metadataPath}
	}

	// Validate annotation keys before writing anything to disk.
	for k := range opts.Annotations {
		if err := domain.ValidateAnnotationKey(k); err != nil {
			return "", err
		}
	}

	var annotations map[string]string
	if len(opts.Annotations) > 0 {
		annotations = opts.Annotations
	}

	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:        filepath.Base(root),
			CreatedAt:   s.now(),
			Annotations: annotations,
		},
		Spec: domain.WorkspaceSpec{
			Sources: []domain.SourceSpec{},
		},
	}
	if err := s.store.Save(root, spec); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}

	if regErr := s.registry.Register(root, spec.Metadata.Name); regErr != nil {
		// Non-fatal: the tasktree is valid on disk. Warn but do not fail.
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not update registry: %v\n", regErr)
	}

	return root, nil
}
