package app

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/materialize"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// AddHTTPOptions are the inputs for AddHTTPService.
type AddHTTPOptions struct {
	URL     string
	SHA256  string
	Headers map[string]string
	Name    string // destination name; defaults to the last segment of the URL path
	Path    string // destination path relative to tasktree root; defaults to Name
}

// AddHTTPResult is the outcome of a successful AddHTTPService.Run call.
type AddHTTPResult struct {
	Source domain.SourceSpec
}

// AddHTTPService downloads a single HTTPS file into the tasktree and registers
// the source in Tasktree.yml.
type AddHTTPService struct {
	store metadata.Store
}

func NewAddHTTPService(store metadata.Store) AddHTTPService {
	return AddHTTPService{store: store}
}

func (s AddHTTPService) Run(ctx context.Context, start string, opts AddHTTPOptions) (AddHTTPResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return AddHTTPResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return AddHTTPResult{}, err
	}

	name := opts.Name
	if name == "" {
		name = deriveHTTPName(opts.URL)
	}
	if err := domain.ValidateSourceName(name); err != nil {
		return AddHTTPResult{}, err
	}
	destRelPath := opts.Path
	if destRelPath == "" {
		destRelPath = name
	}
	for _, src := range spec.Spec.Sources {
		if src.Name == name || src.Path == destRelPath {
			return AddHTTPResult{}, domain.DuplicateSourceNameError{Name: name}
		}
	}
	destPath := filepath.Join(root, destRelPath)
	exists, err := fsx.Exists(destPath)
	if err != nil {
		return AddHTTPResult{}, err
	}
	if exists {
		return AddHTTPResult{}, domain.DestinationExistsError{Path: destPath}
	}

	httpSpec := &domain.HTTPSourceSpec{
		URL:     opts.URL,
		SHA256:  opts.SHA256,
		Headers: opts.Headers,
	}
	if err := materialize.HTTP(ctx, destPath, httpSpec); err != nil {
		return AddHTTPResult{}, err
	}

	source := domain.SourceSpec{
		Name: name,
		Path: destRelPath,
		Type: domain.SourceTypeHTTP,
		HTTP: httpSpec,
	}
	spec.Spec.Sources = append(spec.Spec.Sources, source)
	if err := s.store.Save(root, spec); err != nil {
		return AddHTTPResult{}, fmt.Errorf("save metadata: %w", err)
	}
	return AddHTTPResult{Source: source}, nil
}

// deriveHTTPName returns the last non-empty path segment of a URL,
// falling back to "download" if none can be determined.
func deriveHTTPName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "download"
	}
	base := path.Base(strings.TrimSuffix(u.Path, "/"))
	if base == "" || base == "." || base == "/" {
		return "download"
	}
	return base
}
