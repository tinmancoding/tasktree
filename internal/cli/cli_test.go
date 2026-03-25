package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

func TestInitCreatesMetadataInCurrentDirectory(t *testing.T) {
	store := metadata.NewStore()
	deps := dependencies{
		initService: app.NewInitService(store),
		rootService: app.NewRootService(),
		listService: app.NewListService(store),
	}
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"init"})

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".tasktree.toml")); err != nil {
		t.Fatalf("stat metadata: %v", err)
	}
	if !strings.Contains(out.String(), "Initialized tasktree") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestInitCreatesExplicitPath(t *testing.T) {
	store := metadata.NewStore()
	deps := dependencies{
		initService: app.NewInitService(store),
		rootService: app.NewRootService(),
		listService: app.NewListService(store),
	}
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	target := filepath.Join(t.TempDir(), "feature-payments")
	cmd.SetArgs([]string{"init", target})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, ".tasktree.toml")); err != nil {
		t.Fatalf("stat metadata: %v", err)
	}
}

func TestInitFailsWhenMetadataExists(t *testing.T) {
	store := metadata.NewStore()
	deps := dependencies{
		initService: app.NewInitService(store),
		rootService: app.NewRootService(),
		listService: app.NewListService(store),
	}
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	root := t.TempDir()
	metadataPath := filepath.Join(root, ".tasktree.toml")
	if err := os.WriteFile(metadataPath, []byte("version = 1\nname = \"demo\"\ncreated_at = 2026-03-25T12:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	cmd.SetArgs([]string{"init", root})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tasktree metadata already exists") {
		t.Fatalf("error = %q", err)
	}
}

func TestRootFindsTasktreeFromNestedDirectory(t *testing.T) {
	store := metadata.NewStore()
	deps := dependencies{
		initService: app.NewInitService(store),
		rootService: app.NewRootService(),
		listService: app.NewListService(store),
	}
	root := t.TempDir()
	nested := filepath.Join(root, "api", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := store.Save(root, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"root"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root symlink: %v", err)
	}
	if strings.TrimSpace(out.String()) != resolvedRoot {
		t.Fatalf("root output = %q, want %q", out.String(), resolvedRoot)
	}
}

func TestListPrintsConfiguredRepositories(t *testing.T) {
	store := metadata.NewStore()
	deps := dependencies{
		initService: app.NewInitService(store),
		rootService: app.NewRootService(),
		listService: app.NewListService(store),
	}
	root := t.TempDir()
	if err := store.Save(root, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		Repos: []domain.RepoSpec{{
			Name:     "api",
			Path:     "api",
			Checkout: "main",
			Branch:   "feature/payments",
		}, {
			Name:     "web",
			Path:     "web",
			Checkout: "v1.4.0",
		}},
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute list: %v", err)
	}
	stdout := out.String()
	for _, expected := range []string{"NAME", "api", "feature/payments", "web", "v1.4.0"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout %q does not contain %q", stdout, expected)
		}
	}
}
