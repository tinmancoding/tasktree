package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	tmplstore "github.com/tinmancoding/tasktree/internal/template"
	"github.com/tinmancoding/tasktree/internal/variable"
)

type InitService struct {
	store         metadata.Store
	registry      *registry.Store
	templateStore tmplstore.Store
	now           func() time.Time
}

func NewInitService(store metadata.Store, reg *registry.Store) InitService {
	return InitService{
		store:    store,
		registry: reg,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// NewInitServiceWithTemplates creates an InitService that also supports
// template-based initialization via RunFromTemplate.
func NewInitServiceWithTemplates(store metadata.Store, reg *registry.Store, ts tmplstore.Store) InitService {
	return InitService{
		store:         store,
		registry:      reg,
		templateStore: ts,
		now:           func() time.Time { return time.Now().UTC() },
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

// InitFromTemplateOptions carries configuration for template-based init.
type InitFromTemplateOptions struct {
	// Template is the template name or file path.
	Template string
	// Vars contains the CLI key=value variable bindings.
	Vars variable.CLIArgsSource
	// Name overrides the workspace directory name (overrides template-derived name).
	Name string
	// Dir is the target directory. When empty a new subdirectory is created
	// under the current working directory using the resolved workspace name.
	Dir string
	// DryRun prints what would be created without writing files.
	DryRun bool
}

// InitFromTemplateResult is returned by RunFromTemplate.
type InitFromTemplateResult struct {
	// Root is the absolute path of the workspace directory.
	Root string
	// Spec is the fully resolved TasktreeSpec.
	Spec domain.TasktreeSpec
	// DryRun is true when no files were written.
	DryRun bool
}

// RunFromTemplate creates a workspace from a template by resolving all
// variables and generating a concrete Tasktree.yml.
func (s InitService) RunFromTemplate(opts InitFromTemplateOptions) (InitFromTemplateResult, error) {
	// 1. Load the template.
	tmpl, err := s.templateStore.Load(opts.Template)
	if err != nil {
		return InitFromTemplateResult{}, err
	}

	// 2. Build variable resolver: CLI args → env → parameter defaults.
	defaults := make(variable.DefaultsSource, len(tmpl.Parameters))
	for _, p := range tmpl.Parameters {
		if p.Default != "" {
			defaults[p.Name] = p.Default
		}
	}
	resolver := variable.NewResolver(opts.Vars, variable.NewEnvSource(), defaults)

	// 3. Collect all refs from template body and resolve them.
	allRefs := extractAllRefs(tmpl)
	values := resolver.ResolveAll(allRefs)

	// 4. Validate required parameters have values.
	for _, p := range tmpl.Parameters {
		if p.Required {
			if _, ok := values[p.Name]; !ok {
				return InitFromTemplateResult{}, domain.MissingVariableError{Name: p.Name}
			}
		}
	}

	// 5. Resolve workspace name.
	workspaceName := opts.Name
	if workspaceName == "" {
		workspaceName = variable.RenderString(tmpl.Template.Metadata.Name, values)
	}
	if workspaceName == "" {
		workspaceName = tmpl.Metadata.Name
	}

	// 6. Determine target directory.
	targetDir := opts.Dir
	if targetDir == "" {
		targetDir = workspaceName
	}
	root, err := filepath.Abs(targetDir)
	if err != nil {
		return InitFromTemplateResult{}, fmt.Errorf("resolve path: %w", err)
	}

	// 7. Render the full template body into a concrete TasktreeSpec.
	spec := renderTemplate(tmpl, values, workspaceName, s.now())

	if opts.DryRun {
		return InitFromTemplateResult{Root: root, Spec: spec, DryRun: true}, nil
	}

	// 8. Create directory and write Tasktree.yml.
	if err := os.MkdirAll(root, 0o755); err != nil {
		return InitFromTemplateResult{}, fmt.Errorf("create directory: %w", err)
	}

	metadataPath := s.store.Path(root)
	exists, err := fsx.Exists(metadataPath)
	if err != nil {
		return InitFromTemplateResult{}, fmt.Errorf("check metadata: %w", err)
	}
	if exists {
		return InitFromTemplateResult{}, domain.MetadataExistsError{Path: metadataPath}
	}

	if err := s.store.Save(root, spec); err != nil {
		return InitFromTemplateResult{}, fmt.Errorf("save metadata: %w", err)
	}

	if regErr := s.registry.Register(root, spec.Metadata.Name); regErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not update registry: %v\n", regErr)
	}

	return InitFromTemplateResult{Root: root, Spec: spec}, nil
}

// renderTemplate produces a fully-resolved TasktreeSpec from a template and
// variable values.
func renderTemplate(tmpl domain.TemplateSpec, values map[string]string, workspaceName string, createdAt time.Time) domain.TasktreeSpec {
	t := tmpl.Template

	// Render metadata.
	description := variable.RenderString(t.Metadata.Description, values)
	annotations := buildAnnotations(tmpl, values, workspaceName)
	labels := variable.RenderStringMap(t.Metadata.Labels, values)

	// Render sources.
	sources := make([]domain.SourceSpec, 0, len(t.Spec.Sources))
	for _, src := range t.Spec.Sources {
		rendered := domain.SourceSpec{
			Name: variable.RenderString(src.Name, values),
			Type: domain.SourceType(variable.RenderString(string(src.Type), values)),
			Path: variable.RenderString(src.Path, values),
		}
		if src.Git != nil {
			rendered.Git = &domain.GitSourceSpec{
				URL:    variable.RenderString(src.Git.URL, values),
				Ref:    variable.RenderString(src.Git.Ref, values),
				Branch: variable.RenderString(src.Git.Branch, values),
			}
		}
		if src.HTTP != nil {
			rendered.HTTP = &domain.HTTPSourceSpec{
				URL:     variable.RenderString(src.HTTP.URL, values),
				SHA256:  src.HTTP.SHA256,
				Headers: variable.RenderStringMap(src.HTTP.Headers, values),
			}
		}
		if src.Archive != nil {
			rendered.Archive = &domain.ArchiveSourceSpec{
				URL:             variable.RenderString(src.Archive.URL, values),
				SHA256:          src.Archive.SHA256,
				Format:          src.Archive.Format,
				StripComponents: src.Archive.StripComponents,
			}
		}
		if src.Static != nil {
			rendered.Static = &domain.StaticSourceSpec{
				Content: variable.RenderString(src.Static.Content, values),
				Mode:    src.Static.Mode,
			}
		}
		if src.Local != nil {
			rendered.Local = &domain.LocalSourceSpec{
				SourcePath: variable.RenderString(src.Local.SourcePath, values),
				Copy:       src.Local.Copy,
			}
		}
		sources = append(sources, rendered)
	}

	// Render bootstrap steps.
	var bootstrapSteps []domain.BootstrapStep
	if len(t.Spec.Bootstrap) > 0 {
		bootstrapSteps = make([]domain.BootstrapStep, 0, len(t.Spec.Bootstrap))
		for _, b := range t.Spec.Bootstrap {
			bootstrapSteps = append(bootstrapSteps, domain.BootstrapStep{
				Name:    variable.RenderString(b.Name, values),
				Run:     variable.RenderString(b.Run, values),
				Workdir: variable.RenderString(b.Workdir, values),
				Env:     variable.RenderStringMap(b.Env, values),
			})
		}
	}

	var labelMap map[string]string
	if len(labels) > 0 {
		labelMap = labels
	}

	return domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:        workspaceName,
			Description: description,
			CreatedAt:   createdAt,
			Annotations: annotations,
			Labels:      labelMap,
		},
		Spec: domain.WorkspaceSpec{
			Sources:   sources,
			Bootstrap: bootstrapSteps,
		},
	}
}

// buildAnnotations merges template body annotations with template-origin
// tracking annotations.
func buildAnnotations(tmpl domain.TemplateSpec, values map[string]string, workspaceName string) map[string]string {
	result := variable.RenderStringMap(tmpl.Template.Metadata.Annotations, values)
	if result == nil {
		result = make(map[string]string)
	}

	// Track template origin.
	result["tasktree.dev/template"] = tmpl.Metadata.Name

	// Build variable tracking string: key=value pairs for all resolved values.
	var varParts []string
	for k, v := range values {
		varParts = append(varParts, k+"="+v)
	}
	if len(varParts) > 0 {
		result["tasktree.dev/template-vars"] = strings.Join(varParts, ",")
	}

	return result
}

// extractAllRefs collects all variable references from a template's body.
func extractAllRefs(tmpl domain.TemplateSpec) []variable.VariableRef {
	t := tmpl.Template
	var strs []string
	strs = append(strs, t.Metadata.Name, t.Metadata.Description)
	for k, v := range t.Metadata.Annotations {
		strs = append(strs, k, v)
	}
	for k, v := range t.Metadata.Labels {
		strs = append(strs, k, v)
	}
	for _, src := range t.Spec.Sources {
		strs = append(strs, src.Name, string(src.Type), src.Path)
		if src.Git != nil {
			strs = append(strs, src.Git.URL, src.Git.Ref, src.Git.Branch)
		}
		if src.HTTP != nil {
			strs = append(strs, src.HTTP.URL, src.HTTP.SHA256)
			for k, v := range src.HTTP.Headers {
				strs = append(strs, k, v)
			}
		}
		if src.Archive != nil {
			strs = append(strs, src.Archive.URL, src.Archive.SHA256, src.Archive.Format)
		}
		if src.Static != nil {
			strs = append(strs, src.Static.Content, src.Static.Mode)
		}
		if src.Local != nil {
			strs = append(strs, src.Local.SourcePath)
		}
	}
	for _, b := range t.Spec.Bootstrap {
		strs = append(strs, b.Name, b.Run, b.Workdir)
		for k, v := range b.Env {
			strs = append(strs, k, v)
		}
	}
	seen := make(map[string]struct{})
	var refs []variable.VariableRef
	for _, s := range strs {
		for _, ref := range variable.Parse(s) {
			if _, dup := seen[ref.Name]; !dup {
				seen[ref.Name] = struct{}{}
				refs = append(refs, ref)
			}
		}
	}
	return refs
}
