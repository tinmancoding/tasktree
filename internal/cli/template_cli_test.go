package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	tmplstore "github.com/tinmancoding/tasktree/internal/template"
)

// newTestDepsWithTemplates returns a dependencies struct that includes the
// template service wired to a store that can discover templates from userDir.
// Pass "" for userDir to use only built-ins.
func newTestDepsWithTemplates(t *testing.T, userDir string) dependencies {
	t.Helper()
	store := metadata.NewStore()
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	ts := tmplstore.NewStoreForTest("", userDir)
	deps := dependencies{
		initService:     app.NewInitServiceWithTemplates(store, reg, ts),
		rootService:     app.NewRootService(),
		listService:     app.NewListService(store),
		templateService: app.NewTemplateService(ts),
	}
	return deps
}

// ---- template list ----

func TestTemplateList_ShowsBuiltins(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"template", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "bugfix") {
		t.Errorf("expected 'bugfix' in output, got: %s", output)
	}
	if !strings.Contains(output, "NAME") {
		t.Errorf("expected header row, got: %s", output)
	}
}

// ---- template show ----

func TestTemplateShow_BuiltinBugfix(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"template", "show", "bugfix"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Name:") {
		t.Errorf("expected Name: in output, got: %s", output)
	}
	if !strings.Contains(output, "ticket_number") {
		t.Errorf("expected parameter 'ticket_number' in output, got: %s", output)
	}
}

func TestTemplateShow_NotFound(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"template", "show", "does-not-exist"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---- template validate ----

func TestTemplateValidate_Valid(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"template", "validate", "bugfix"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "valid") {
		t.Errorf("expected 'valid' in output, got: %s", out.String())
	}
}

func TestTemplateValidate_InvalidTemplate(t *testing.T) {
	// Write a broken template to a temp dir.
	userDir := t.TempDir()
	brokenYAML := `
apiVersion: tasktree.dev/v1
kind: Template
metadata:
  name: broken
parameters:
  - name: ticket_number
    required: true
template:
  metadata:
    name: "{{tocket_number}}"
  spec:
    sources: []
`
	if err := os.WriteFile(filepath.Join(userDir, "broken.yml"), []byte(brokenYAML), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	deps := newTestDepsWithTemplates(t, userDir)
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"template", "validate", "broken"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "tocket_number") {
		t.Errorf("expected typo variable in error, got: %v", err)
	}
}

// ---- init --from ----

func TestInitFromTemplate_CreatesWorkspace(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "bugfix-BUG-1")
	cmd.SetArgs([]string{
		"init",
		"--from", "bugfix",
		"ticket_number=BUG-1",
		"repo=git@github.com:org/api.git",
		"--dir", targetDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Tasktree.yml must exist.
	specPath := filepath.Join(targetDir, domain.SpecFileName)
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("expected Tasktree.yml at %s: %v", specPath, err)
	}

	// Load and verify the generated spec.
	store := metadata.NewStore()
	spec, err := store.Load(targetDir)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	if spec.Metadata.Name != "bugfix-BUG-1" {
		t.Errorf("Name = %q, want bugfix-BUG-1", spec.Metadata.Name)
	}
	if spec.Metadata.Annotations["ticket"] != "BUG-1" {
		t.Errorf("annotation ticket = %q, want BUG-1", spec.Metadata.Annotations["ticket"])
	}
	if spec.Metadata.Annotations["tasktree.dev/template"] != "bugfix" {
		t.Errorf("template annotation = %q, want bugfix", spec.Metadata.Annotations["tasktree.dev/template"])
	}

	if len(spec.Spec.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(spec.Spec.Sources))
	}
	src := spec.Spec.Sources[0]
	if src.Git == nil {
		t.Fatal("expected git source")
	}
	if src.Git.Branch != "fix/BUG-1" {
		t.Errorf("branch = %q, want fix/BUG-1", src.Git.Branch)
	}
	if src.Git.Ref != "main" { // default base_branch
		t.Errorf("ref = %q, want main", src.Git.Ref)
	}

	if !strings.Contains(out.String(), "Initialized tasktree") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestInitFromTemplate_OverrideDefault(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "ws")
	cmd.SetArgs([]string{
		"init",
		"--from", "bugfix",
		"ticket_number=BUG-2",
		"repo=git@github.com:org/api.git",
		"base_branch=develop",
		"--dir", targetDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	store := metadata.NewStore()
	spec, err := store.Load(targetDir)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	if spec.Spec.Sources[0].Git.Ref != "develop" {
		t.Errorf("ref = %q, want develop", spec.Spec.Sources[0].Git.Ref)
	}
}

func TestInitFromTemplate_MissingRequiredVar(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	cmd.SetArgs([]string{
		"init",
		"--from", "bugfix",
		// Missing ticket_number (required)
		"repo=git@github.com:org/api.git",
		"--dir", t.TempDir(),
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "ticket_number") {
		t.Errorf("expected 'ticket_number' in error, got: %v", err)
	}
}

func TestInitFromTemplate_DryRun(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "ws")
	cmd.SetArgs([]string{
		"init",
		"--from", "bugfix",
		"ticket_number=BUG-3",
		"repo=git@github.com:org/api.git",
		"--dir", targetDir,
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// File must NOT exist in dry-run mode.
	if _, err := os.Stat(filepath.Join(targetDir, domain.SpecFileName)); err == nil {
		t.Error("Tasktree.yml should not exist in dry-run mode")
	}

	if !strings.Contains(out.String(), "Dry run") {
		t.Errorf("expected 'Dry run' in output, got: %s", out.String())
	}
}

func TestInitFromTemplate_WithNameOverride(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "ws")
	cmd.SetArgs([]string{
		"init",
		"--from", "bugfix",
		"ticket_number=BUG-4",
		"repo=git@github.com:org/api.git",
		"--dir", targetDir,
		"--name", "my-custom-name",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	store := metadata.NewStore()
	spec, err := store.Load(targetDir)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	if spec.Metadata.Name != "my-custom-name" {
		t.Errorf("Name = %q, want my-custom-name", spec.Metadata.Name)
	}
}

func TestInitFromTemplate_TemplateNotFound(t *testing.T) {
	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	cmd.SetArgs([]string{
		"init",
		"--from", "nonexistent-template",
		"--dir", t.TempDir(),
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestInitFromTemplate_FromFilePath(t *testing.T) {
	// Write a custom template to a temp file.
	tmplContent := `
apiVersion: tasktree.dev/v1
kind: Template
metadata:
  name: custom
  description: Custom template from file
parameters:
  - name: feature_name
    required: true
template:
  metadata:
    name: "custom-{{feature_name}}"
  spec:
    sources:
      - name: api
        type: git
        git:
          url: "git@github.com:org/api.git"
          branch: "feature/{{feature_name}}"
`
	tmplDir := t.TempDir()
	tmplPath := filepath.Join(tmplDir, "custom.yml")
	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "ws")
	cmd.SetArgs([]string{
		"init",
		"--from", tmplPath,
		"feature_name=payments",
		"--dir", targetDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	store := metadata.NewStore()
	spec, err := store.Load(targetDir)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	if spec.Metadata.Name != "custom-payments" {
		t.Errorf("Name = %q, want custom-payments", spec.Metadata.Name)
	}
}

func TestInitFromTemplate_LocalSourcePreserved(t *testing.T) {
	// A template with a local source must have its local: sub-spec
	// (sourcePath, copy) forwarded into the generated Tasktree.yml.
	// Previously renderTemplate only forwarded git sources; local sources were
	// silently dropped, so the .opencode symlink was never created.
	srcDir := t.TempDir() // stand-in for the real source path

	tmplContent := `
apiVersion: tasktree.dev/v1
kind: Template
metadata:
  name: local-test
parameters:
  - name: ticket
    required: true
template:
  metadata:
    name: "ws-{{ticket}}"
  spec:
    sources:
      - name: opencode-config
        type: local
        path: .opencode
        local:
          sourcePath: "` + srcDir + `"
          copy: false
`
	tmplDir := t.TempDir()
	tmplPath := filepath.Join(tmplDir, "local-test.yml")
	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	deps := newTestDepsWithTemplates(t, "")
	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	targetDir := filepath.Join(t.TempDir(), "ws")
	cmd.SetArgs([]string{
		"init",
		"--from", tmplPath,
		"ticket=AIBM-1234",
		"--dir", targetDir,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	store := metadata.NewStore()
	spec, err := store.Load(targetDir)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	if len(spec.Spec.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(spec.Spec.Sources))
	}
	src := spec.Spec.Sources[0]
	if src.Local == nil {
		t.Fatal("local sub-spec is nil: renderTemplate dropped the local source")
	}
	if src.Local.SourcePath != srcDir {
		t.Errorf("SourcePath = %q, want %q", src.Local.SourcePath, srcDir)
	}
	if src.Local.Copy {
		t.Error("Copy = true, want false")
	}
	if src.Path != ".opencode" {
		t.Errorf("Path = %q, want .opencode", src.Path)
	}
}
