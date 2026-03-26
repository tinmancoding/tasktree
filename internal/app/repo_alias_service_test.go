package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/repoalias"
)

func TestRepoAliasSetAndListServices(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	setService := app.NewRepoAliasSetService(store)
	listService := app.NewRepoAliasListService(store)

	if err := setService.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("set first alias: %v", err)
	}
	if err := setService.Run("backend", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("set second alias: %v", err)
	}
	if err := setService.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("repeat same alias for same repo: %v", err)
	}

	aliases, err := listService.Run()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("alias count = %d, want 2", len(aliases))
	}
	got := map[string]string{}
	for _, alias := range aliases {
		got[alias.Alias] = alias.URL
	}
	if got["api"] != "git@github.com:acme/api.git" {
		t.Fatalf("api url = %q", got["api"])
	}
	if got["backend"] != "git@github.com:acme/api.git" {
		t.Fatalf("backend url = %q", got["backend"])
	}

	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("read repo alias file: %v", err)
	}
	text := string(contents)
	for _, expected := range []string{"repos:", "url: git@github.com:acme/api.git", "- backend", "- api"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("repo alias file %q missing %q", text, expected)
		}
	}
}

func TestRepoAliasSetServiceFailsWhenAliasBelongsToAnotherRepo(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	setService := app.NewRepoAliasSetService(store)

	if err := setService.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	err := setService.Run("api", "git@github.com:acme/platform.git")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "repository alias \"api\" is already used by git@github.com:acme/api.git") {
		t.Fatalf("error = %q", err)
	}
	if _, ok := err.(domain.RepoAliasInUseError); !ok {
		t.Fatalf("error type = %T, want RepoAliasInUseError", err)
	}
}

func TestRepoAliasRegisterDerivedServiceAddsAvailableAliases(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	service := app.NewRepoAliasRegisterDerivedService(store)

	registrations, err := service.Run("git@github.com:acme/api.git")
	if err != nil {
		t.Fatalf("register aliases: %v", err)
	}
	if len(registrations) != 2 {
		t.Fatalf("registration count = %d, want 2", len(registrations))
	}
	for _, registration := range registrations {
		if registration.Status != "added" {
			t.Fatalf("registration = %#v, want added", registration)
		}
	}

	file, err := store.Load()
	if err != nil {
		t.Fatalf("load repo alias file: %v", err)
	}
	if len(file.Repos) != 1 {
		t.Fatalf("repo count = %d, want 1", len(file.Repos))
	}
	if len(file.Repos[0].Aliases) != 2 {
		t.Fatalf("alias count = %d, want 2", len(file.Repos[0].Aliases))
	}
}

func TestRepoAliasRegisterDerivedServiceSkipsConflictsAndKeepsRepoEntry(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	setService := app.NewRepoAliasSetService(store)
	service := app.NewRepoAliasRegisterDerivedService(store)

	if err := setService.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	registrations, err := service.Run("git@github.com:other/api.git")
	if err != nil {
		t.Fatalf("register aliases: %v", err)
	}
	if len(registrations) != 2 {
		t.Fatalf("registration count = %d, want 2", len(registrations))
	}
	if registrations[0].Alias != "api" || registrations[0].Status != "conflict" {
		t.Fatalf("first registration = %#v", registrations[0])
	}
	if registrations[1].Alias != "other-api" || registrations[1].Status != "added" {
		t.Fatalf("second registration = %#v", registrations[1])
	}

	file, err := store.Load()
	if err != nil {
		t.Fatalf("load repo alias file: %v", err)
	}
	if len(file.Repos) != 2 {
		t.Fatalf("repo count = %d, want 2", len(file.Repos))
	}
	for _, repo := range file.Repos {
		if repo.URL == "git@github.com:other/api.git" && len(repo.Aliases) != 1 {
			t.Fatalf("other repo aliases = %v, want [other-api]", repo.Aliases)
		}
	}
}

func TestRepoAliasRemoveServiceKeepsRepoEntryWhenLastAliasRemoved(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	setService := app.NewRepoAliasSetService(store)
	removeService := app.NewRepoAliasRemoveService(store)
	listService := app.NewRepoAliasListService(store)

	if err := setService.Run("api", "git@github.com:acme/api.git"); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if err := removeService.Run("api"); err != nil {
		t.Fatalf("remove alias: %v", err)
	}

	aliases, err := listService.Run()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}
	if len(aliases) != 0 {
		t.Fatalf("alias count = %d, want 0", len(aliases))
	}

	file, err := store.Load()
	if err != nil {
		t.Fatalf("load repo alias file: %v", err)
	}
	if len(file.Repos) != 1 {
		t.Fatalf("repo count = %d, want 1", len(file.Repos))
	}
	if file.Repos[0].URL != "git@github.com:acme/api.git" {
		t.Fatalf("repo url = %q", file.Repos[0].URL)
	}
	if len(file.Repos[0].Aliases) != 0 {
		t.Fatalf("alias count = %d, want 0", len(file.Repos[0].Aliases))
	}
}

func TestRepoAliasRemoveServiceReturnsNotFound(t *testing.T) {
	store := repoalias.NewStore(filepath.Join(t.TempDir(), "repos.yml"))
	removeService := app.NewRepoAliasRemoveService(store)

	err := removeService.Run("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "repository alias \"missing\" was not found") {
		t.Fatalf("error = %q", err)
	}
	if _, ok := err.(domain.RepoAliasNotFoundError); !ok {
		t.Fatalf("error type = %T, want RepoAliasNotFoundError", err)
	}
}
