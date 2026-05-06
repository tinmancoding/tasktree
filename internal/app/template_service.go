package app

import (
	"github.com/tinmancoding/tasktree/internal/domain"
	tmplstore "github.com/tinmancoding/tasktree/internal/template"
)

// TemplateService orchestrates template list, show, and validate operations.
type TemplateService struct {
	store tmplstore.Store
}

// NewTemplateService creates a TemplateService using the provided store.
func NewTemplateService(store tmplstore.Store) TemplateService {
	return TemplateService{store: store}
}

// TemplateListEntry is a summary row for the template list command.
type TemplateListEntry struct {
	Name        string
	Description string
	Parameters  []domain.ParameterSpec
}

// List returns a summary of all discoverable templates.
func (s TemplateService) List() ([]TemplateListEntry, error) {
	specs, err := s.store.List()
	if err != nil {
		return nil, err
	}
	entries := make([]TemplateListEntry, len(specs))
	for i, spec := range specs {
		entries[i] = TemplateListEntry{
			Name:        spec.Metadata.Name,
			Description: spec.Metadata.Description,
			Parameters:  spec.Parameters,
		}
	}
	return entries, nil
}

// ShowResult is the full detail for a single template.
type ShowResult struct {
	Spec     domain.TemplateSpec
	Location string // file path if loaded from disk, "built-in" otherwise
}

// Show returns the full spec for a named (or path-based) template.
func (s TemplateService) Show(nameOrPath string) (ShowResult, error) {
	spec, err := s.store.Load(nameOrPath)
	if err != nil {
		return ShowResult{}, err
	}
	return ShowResult{Spec: spec}, nil
}

// Validate loads a template by name or path and checks it for errors.
// Returns nil if the template is valid.
func (s TemplateService) Validate(nameOrPath string) error {
	spec, err := s.store.Load(nameOrPath)
	if err != nil {
		return err
	}
	return s.store.Validate(spec)
}
