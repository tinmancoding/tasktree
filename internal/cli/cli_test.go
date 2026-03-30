package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	"github.com/tinmancoding/tasktree/internal/repoalias"
	"github.com/tinmancoding/tasktree/internal/testutil"
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
		pruneService:         app.NewPruneService(reg),
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

func TestRepoAliasCommandsManageConfigFile(t *testing.T) {
	aliasStore := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	deps := dependencies{
		aliasSet:      app.NewRepoAliasSetService(aliasStore),
		aliasRemove:   app.NewRepoAliasRemoveService(aliasStore),
		aliasList:     app.NewRepoAliasListService(aliasStore),
		aliasResolve:  app.NewRepoAliasResolveService(aliasStore),
		aliasRegister: app.NewRepoAliasRegisterDerivedService(aliasStore),
	}

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"repo", "add-alias", "api", "git@github.com:acme/api.git"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute repo add-alias: %v", err)
	}
	if !strings.Contains(out.String(), "Added alias api") {
		t.Fatalf("stdout = %q", out.String())
	}

	out.Reset()
	cmd.SetArgs([]string{"repo", "aliases"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute repo aliases: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ALIAS") || !strings.Contains(got, "api") || !strings.Contains(got, "git@github.com:acme/api.git") {
		t.Fatalf("stdout = %q", got)
	}

	out.Reset()
	cmd.SetArgs([]string{"repo", "remove-alias", "api"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute repo remove-alias: %v", err)
	}
	if !strings.Contains(out.String(), "Removed alias api") {
		t.Fatalf("stdout = %q", out.String())
	}

	file, err := aliasStore.Load()
	if err != nil {
		t.Fatalf("load alias store: %v", err)
	}
	if len(file.Repos) != 1 {
		t.Fatalf("repo count = %d, want 1", len(file.Repos))
	}
	if file.Repos[0].URL != "git@github.com:acme/api.git" {
		t.Fatalf("repo url = %q", file.Repos[0].URL)
	}
}

func TestRepoAddAliasFailsWhenAliasUsedByAnotherRepo(t *testing.T) {
	aliasStore := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	deps := dependencies{
		aliasSet: app.NewRepoAliasSetService(aliasStore),
	}

	if err := deps.aliasSet.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	cmd := NewRootCmd(deps)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"repo", "add-alias", "api", "git@github.com:acme/platform.git"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "repository alias \"api\" is already used by git@github.com:acme/api.git") {
		t.Fatalf("error = %q", err)
	}
}

func TestAddResolvesRepoAlias(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := t.TempDir()
	store := metadata.NewStore()
	if err := store.Save(root, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	aliasStore := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	if err := app.NewRepoAliasSetService(aliasStore).Run("app", remoteURL); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	deps := dependencies{
		addService:    app.NewAddService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient()),
		aliasResolve:  app.NewRepoAliasResolveService(aliasStore),
		aliasRegister: app.NewRepoAliasRegisterDerivedService(aliasStore),
	}
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"add", "app"})

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, "Added app at app") {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(stdout, "Alias app already points to "+remoteURL) {
		t.Fatalf("stdout = %q", stdout)
	}
	derivedAliases, err := domain.DeriveRepoAliases(remoteURL)
	if err != nil {
		t.Fatalf("derive aliases: %v", err)
	}
	if !strings.Contains(stdout, "Registered alias "+derivedAliases[1]+" -> "+remoteURL) {
		t.Fatalf("stdout = %q", stdout)
	}

	file, err := store.Load(root)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(file.Repos) != 1 {
		t.Fatalf("repo count = %d, want 1", len(file.Repos))
	}
	if file.Repos[0].URL != remoteURL {
		t.Fatalf("repo url = %q, want %q", file.Repos[0].URL, remoteURL)
	}

	if _, err := os.Stat(filepath.Join(root, "app")); err != nil {
		t.Fatalf("stat checkout: %v", err)
	}

	aliasFile, err := aliasStore.Load()
	if err != nil {
		t.Fatalf("load alias file: %v", err)
	}
	if len(aliasFile.Repos) != 1 {
		t.Fatalf("alias repo count = %d, want 1", len(aliasFile.Repos))
	}
	if len(aliasFile.Repos[0].Aliases) != 2 {
		t.Fatalf("alias count = %d, want 2", len(aliasFile.Repos[0].Aliases))
	}

}

func TestPruneRemovesStaleEntries(t *testing.T) {
	deps, reg := newTestDeps(t)
	store := metadata.NewStore()

	validRoot := t.TempDir()
	if err := store.Save(validRoot, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      "valid",
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := reg.Register(validRoot, "valid"); err != nil {
		t.Fatalf("register valid: %v", err)
	}

	ghostPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(ghostPath, "ghost"); err != nil {
		t.Fatalf("register ghost: %v", err)
	}

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"prune"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prune: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, "ghost") {
		t.Fatalf("expected ghost in output, got: %q", stdout)
	}
	if strings.Contains(stdout, "valid") {
		t.Fatalf("valid tasktree should not appear in output, got: %q", stdout)
	}

	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 1 || f.Tasktrees[0].Name != "valid" {
		t.Fatalf("registry after prune = %+v, want only valid entry", f.Tasktrees)
	}
}

func TestPruneDryRunDoesNotModifyRegistry(t *testing.T) {
	deps, reg := newTestDeps(t)

	ghostPath := filepath.Join(t.TempDir(), "ghost-ws")
	if err := reg.Register(ghostPath, "ghost"); err != nil {
		t.Fatalf("register ghost: %v", err)
	}

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"prune", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prune --dry-run: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, "Would remove") {
		t.Fatalf("expected 'Would remove' in dry-run output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "ghost") {
		t.Fatalf("expected ghost path in dry-run output, got: %q", stdout)
	}

	// registry must be untouched
	f, err := reg.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(f.Tasktrees) != 1 {
		t.Fatalf("registry count = %d, want 1 (dry-run must not modify)", len(f.Tasktrees))
	}
}

func TestPruneNothingToPrune(t *testing.T) {
	deps, _ := newTestDeps(t)

	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"prune"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prune: %v", err)
	}
	if !strings.Contains(out.String(), "Nothing to prune.") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestAddLogsSkippedAliasConflicts(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	root := t.TempDir()
	store := metadata.NewStore()
	if err := store.Save(root, domain.TasktreeFile{
		Version:   domain.MetadataVersion,
		Name:      filepath.Base(root),
		CreatedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	aliasStore := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	if err := app.NewRepoAliasSetService(aliasStore).Run("app", "git@github.com:someone-else/app.git"); err != nil {
		t.Fatalf("seed conflicting alias: %v", err)
	}

	deps := dependencies{
		addService:    app.NewAddService(store, cache.NewManager(cacheRoot, gitx.NewClient()), gitx.NewClient()),
		aliasResolve:  app.NewRepoAliasResolveService(aliasStore),
		aliasRegister: app.NewRepoAliasRegisterDerivedService(aliasStore),
	}
	cmd := NewRootCmd(deps)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"add", remoteURL})

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute add: %v", err)
	}
	stdout := out.String()
	if !strings.Contains(stdout, "Skipped alias app; already used by git@github.com:someone-else/app.git") {
		t.Fatalf("stdout = %q", stdout)
	}
	derivedAliases, err := domain.DeriveRepoAliases(remoteURL)
	if err != nil {
		t.Fatalf("derive aliases: %v", err)
	}
	if !strings.Contains(stdout, "Registered alias "+derivedAliases[1]+" -> "+remoteURL) {
		t.Fatalf("stdout = %q", stdout)
	}
}
