package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestStatusServiceReportsCleanModifiedAndDetached(t *testing.T) {
	ctx := context.Background()
	remoteURL, mutateRemote := testutil.CreateRemoteRepo(t)
	_ = mutateRemote
	root := createTasktree(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store := metadata.NewStore()
	git := gitx.NewClient()
	addService := app.NewAddService(store, cache.NewManager(cacheRoot, git), git)
	statusService := app.NewStatusService(store, git)

	cleanResult, err := addService.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Name: "clean-app"})
	if err != nil {
		t.Fatalf("add clean repo: %v", err)
	}

	work := t.TempDir()
	testutil.RunGit(t, work, "clone", remoteURL, filepath.Join(work, "repo"))
	tagRepo := filepath.Join(work, "repo")
	commit := strings.TrimSpace(testutil.RunGit(t, tagRepo, "rev-parse", "HEAD"))
	testutil.RunGit(t, tagRepo, "tag", "v1.2.0", commit)
	testutil.RunGit(t, tagRepo, "push", "origin", "v1.2.0")
	_, err = addService.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, From: "v1.2.0", Name: "tagged-app"})
	if err != nil {
		t.Fatalf("add tagged repo: %v", err)
	}

	modifiedResult, err := addService.Run(ctx, root, app.AddOptions{RepoURL: remoteURL, Name: "modified-app-2"})
	if err != nil {
		t.Fatalf("add modified repo: %v", err)
	}
	modifiedSourcePath := modifiedResult.Source.Path
	if modifiedSourcePath == "" {
		modifiedSourcePath = modifiedResult.Source.Name
	}
	modifiedFile := filepath.Join(root, modifiedSourcePath, "README.md")
	if err := os.WriteFile(modifiedFile, []byte("locally modified\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

	status, err := statusService.Run(ctx, root)
	if err != nil {
		t.Fatalf("run status: %v", err)
	}

	if status.TasktreeName == "" || status.Root != root {
		t.Fatalf("unexpected status header: %#v", status)
	}
	got := map[string]app.RepoStatus{}
	for _, repo := range status.Repos {
		got[repo.Name] = repo
	}
	if got[cleanResult.Source.Name].Head != "main" || got[cleanResult.Source.Name].State != "clean" {
		t.Fatalf("clean repo status = %#v", got[cleanResult.Source.Name])
	}
	if got["tagged-app"].Head != "v1.2.0" || got["tagged-app"].State != "detached, clean" {
		t.Fatalf("tagged repo status = %#v", got["tagged-app"])
	}
	if got[modifiedResult.Source.Name].Head != "main" || got[modifiedResult.Source.Name].State != "modified" {
		t.Fatalf("modified repo status = %#v", got[modifiedResult.Source.Name])
	}
}
