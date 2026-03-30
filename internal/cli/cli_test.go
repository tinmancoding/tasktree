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
	"github.com/tinmancoding/tasktree/internal/registry"
)

// newTestDeps returns a minimal dependencies struct suitable for unit tests.
// Each test gets its own isolated registry via a temp dir path.
func newTestDeps(t *testing.T) (dependencies, *registry.Store) {
	t.Helper()
	store := metadata.NewStore()
	reg := registry.NewStoreAt(filepath.Join(t.TempDir(), "registry.toml"))
	deps := dependencies{
		initService:          app.NewInitService(store, reg),
		rootService:          app.NewRootService(),
		listService:          app.NewListService(store),
		listTasktreesService: app.NewListTasktreesService(reg),
	}
	return deps, reg
}

func TestInitCreatesMetadataInCurrentDirectory(t *testing.T) {
	deps, _ := newTestDeps(t)
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
	deps, _ := newTestDeps(t)
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
	deps, _ := newTestDeps(t)
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
	deps, _ := newTestDeps(t)
	store := metadata.NewStore()
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

func TestReposPrintsConfiguredRepositories(t *testing.T) {
	deps, _ := newTestDeps(t)
	store := metadata.NewStore()
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
	cmd.SetArgs([]string{"repos"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute repos: %v", err)
	}
	stdout := out.String()
	for _, expected := range []string{"NAME", "api", "feature/payments", "web", "v1.4.0"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout %q does not contain %q", stdout, expected)
		}
	}
}

func TestReposFailsOutsideTasktree(t *testing.T) {
	deps, _ := newTestDeps(t)
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"repos"})

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error outside tasktree")
	}
	if !strings.Contains(execErr.Error(), "Not inside a tasktree") {
		t.Fatalf("error = %q", execErr)
	}
}

func TestListEmptyRegistry(t *testing.T) {
	deps, _ := newTestDeps(t)
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute list: %v", err)
	}
	if !strings.Contains(out.String(), "No tasktrees registered.") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestListShowsRegisteredTasktrees(t *testing.T) {
	deps, reg := newTestDeps(t)

	root1 := t.TempDir()
	root2 := t.TempDir()
	store := metadata.NewStore()
	if err := store.Save(root1, domain.TasktreeFile{
		Version: domain.MetadataVersion, Name: "alpha",
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(root2, domain.TasktreeFile{
		Version: domain.MetadataVersion, Name: "beta",
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(root1, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(root2, "beta"); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute list: %v", err)
	}
	stdout := out.String()
	for _, expected := range []string{"NAME", "alpha", "beta", root1, root2} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout %q does not contain %q", stdout, expected)
		}
	}
}

func TestListAnnotatesMissingTasktree(t *testing.T) {
	deps, reg := newTestDeps(t)

	// Register a path that will not exist on disk.
	ghostPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(ghostPath, "ghost"); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute list: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, "missing") {
		t.Fatalf("expected (missing) annotation, got: %q", stdout)
	}
}

func TestInitRegistersInGlobalList(t *testing.T) {
	deps, reg := newTestDeps(t)
	target := filepath.Join(t.TempDir(), "my-ws")

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", target})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	f, err := reg.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("expected 1 registry entry, got %d", len(f.Tasktrees))
	}
	// Resolve symlinks on both sides: macOS /var/folders is a symlink to
	// /private/var/folders, so filepath.Abs and filepath.EvalSymlinks diverge.
	absTarget, err := filepath.Abs(target)
	if err != nil {
		t.Fatalf("abs target: %v", err)
	}
	registryPath := f.Tasktrees[0].Path
	if registryPath != absTarget {
		t.Errorf("registry path = %q, want %q", registryPath, absTarget)
	}
	if f.Tasktrees[0].Name != "my-ws" {
		t.Errorf("registry name = %q, want %q", f.Tasktrees[0].Name, "my-ws")
	}
}
