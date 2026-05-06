package template_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
	tmplstore "github.com/tinmancoding/tasktree/internal/template"
)

// helper: write a template YAML to a file in dir.
func writeTemplate(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
}

const validBugfixYAML = `
apiVersion: tasktree.dev/v1
kind: Template
metadata:
  name: bugfix
  description: Single-repo bugfix workspace
parameters:
  - name: ticket_number
    required: true
    description: Ticket/issue identifier
  - name: repo
    required: true
    description: Repository alias or URL
  - name: base_branch
    default: main
template:
  metadata:
    name: "bugfix-{{ticket_number}}"
    annotations:
      ticket: "{{ticket_number}}"
      type: bugfix
  spec:
    sources:
      - name: repo
        type: git
        git:
          url: "{{repo}}"
          ref: "{{base_branch}}"
          branch: "fix/{{ticket_number}}"
`

// ---------------------------------------------------------------------------
// List / LoadByName
// ---------------------------------------------------------------------------

func TestList_BuiltinsAlwaysPresent(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	specs, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(specs) == 0 {
		t.Error("expected at least one built-in template")
	}
	// bugfix is a known built-in
	found := false
	for _, s := range specs {
		if s.Metadata.Name == "bugfix" {
			found = true
			break
		}
	}
	if !found {
		t.Error("built-in 'bugfix' template not found")
	}
}

func TestLoadByName_BuiltinBugfix(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec, err := store.LoadByName("bugfix")
	if err != nil {
		t.Fatalf("LoadByName error: %v", err)
	}
	if spec.Metadata.Name != "bugfix" {
		t.Errorf("Name = %q, want bugfix", spec.Metadata.Name)
	}
	if spec.Kind != domain.KindTemplate {
		t.Errorf("Kind = %q, want Template", spec.Kind)
	}
}

func TestLoadByName_NotFound(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	_, err := store.LoadByName("does-not-exist")
	var notFound domain.TemplateNotFoundError
	if _, ok := err.(domain.TemplateNotFoundError); !ok {
		t.Errorf("expected TemplateNotFoundError, got %T: %v", err, err)
		_ = notFound
	}
}

func TestLoadByName_UserDirShadowsBuiltin(t *testing.T) {
	userDir := t.TempDir()
	// Create a user-level template named "bugfix" with a different description.
	writeTemplate(t, userDir, "bugfix.yml", `
apiVersion: tasktree.dev/v1
kind: Template
metadata:
  name: bugfix
  description: Custom user bugfix
parameters:
  - name: ticket_number
    required: true
  - name: repo
    required: true
template:
  metadata:
    name: "bugfix-{{ticket_number}}"
  spec:
    sources:
      - name: repo
        type: git
        git:
          url: "{{repo}}"
`)
	store := tmplstore.NewStoreForTest("", userDir)
	spec, err := store.LoadByName("bugfix")
	if err != nil {
		t.Fatalf("LoadByName error: %v", err)
	}
	if spec.Metadata.Description != "Custom user bugfix" {
		t.Errorf("expected user template description, got %q", spec.Metadata.Description)
	}
}

func TestLoad_ByPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-tmpl.yml")
	writeTemplate(t, dir, "my-tmpl.yml", validBugfixYAML)

	store := tmplstore.NewStoreForTest("", "")
	spec, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if spec.Metadata.Name != "bugfix" {
		t.Errorf("Name = %q, want bugfix", spec.Metadata.Name)
	}
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_Valid(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec, err := store.LoadByName("bugfix")
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if err := store.Validate(spec); err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidate_WrongKind(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec := domain.TemplateSpec{
		Kind:     "Tasktree",
		Metadata: domain.TemplateMetadata{Name: "foo"},
	}
	if err := store.Validate(spec); err == nil {
		t.Error("expected error for wrong kind")
	}
}

func TestValidate_EmptyName(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec := domain.TemplateSpec{
		Kind: domain.KindTemplate,
	}
	if err := store.Validate(spec); err == nil {
		t.Error("expected error for empty metadata.name")
	}
}

func TestValidate_UnknownVariable(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec := domain.TemplateSpec{
		Kind:     domain.KindTemplate,
		Metadata: domain.TemplateMetadata{Name: "test"},
		Parameters: []domain.ParameterSpec{
			{Name: "ticket_number", Required: true},
		},
		Template: domain.TasktreeTemplate{
			Metadata: domain.SpecMetadata{
				Name: "bugfix-{{tocket_number}}", // typo
			},
		},
	}
	err := store.Validate(spec)
	if err == nil {
		t.Fatal("expected error for unknown variable")
	}
	var unknownErr domain.UnknownVariableError
	if _, ok := err.(domain.UnknownVariableError); !ok {
		t.Errorf("expected UnknownVariableError, got %T: %v", err, err)
		_ = unknownErr
	}
}

func TestValidate_SuggestionInError(t *testing.T) {
	store := tmplstore.NewStoreForTest("", "")
	spec := domain.TemplateSpec{
		Kind:     domain.KindTemplate,
		Metadata: domain.TemplateMetadata{Name: "test"},
		Parameters: []domain.ParameterSpec{
			{Name: "ticket_number", Required: true},
		},
		Template: domain.TasktreeTemplate{
			Metadata: domain.SpecMetadata{
				Name: "{{tocket_number}}", // one-char typo of "ticket_number"
			},
		},
	}
	err := store.Validate(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	ue, ok := err.(domain.UnknownVariableError)
	if !ok {
		t.Fatalf("expected UnknownVariableError, got %T", err)
	}
	if ue.Suggestion != "ticket_number" {
		t.Errorf("Suggestion = %q, want ticket_number", ue.Suggestion)
	}
}
