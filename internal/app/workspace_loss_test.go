package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
)

func runGitTest(t *testing.T, g gitx.Client, dir string, args ...string) string {
	t.Helper()
	out, _, err := g.Run(context.Background(), dir, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

func TestRepoLossIgnoresOriginFetchedTagOnlyCommit(t *testing.T) {
	ctx := context.Background()
	g := gitx.NewClient()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGitTest(t, g, "", "init", "--bare", remote)
	seed := filepath.Join(t.TempDir(), "seed")
	runGitTest(t, g, "", "init", "-b", "main", seed)
	runGitTest(t, g, seed, "config", "user.name", "test")
	runGitTest(t, g, seed, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(seed, "main"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, g, seed, "add", ".")
	runGitTest(t, g, seed, "commit", "-m", "main")
	runGitTest(t, g, seed, "remote", "add", "origin", remote)
	runGitTest(t, g, seed, "push", "origin", "main")
	runGitTest(t, g, seed, "checkout", "--orphan", "tag-only")
	runGitTest(t, g, seed, "rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(seed, "tagged"), []byte("tagged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, g, seed, "add", ".")
	runGitTest(t, g, seed, "commit", "-m", "tag only")
	runGitTest(t, g, seed, "tag", "stash/example")
	runGitTest(t, g, seed, "push", "origin", "stash/example")
	repo := filepath.Join(t.TempDir(), "repo")
	runGitTest(t, g, "", "clone", remote, repo)
	runGitTest(t, g, repo, "fetch", "origin", "--tags")
	runGitTest(t, g, repo, "checkout", "main")
	loss, err := repoLoss(ctx, g, "repo", repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(loss) != 0 {
		t.Fatalf("origin-fetched tag caused loss: %v", loss)
	}
}

func TestRepoLossFindsDetachedCommitsAndStashesAndReportsErrors(t *testing.T) {
	ctx := context.Background()
	g := gitx.NewClient()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGitTest(t, g, "", "init", "--bare", remote)
	repo := filepath.Join(t.TempDir(), "repo")
	runGitTest(t, g, "", "init", "-b", "main", repo)
	runGitTest(t, g, repo, "config", "user.name", "test")
	runGitTest(t, g, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, g, repo, "add", ".")
	runGitTest(t, g, repo, "commit", "-m", "base")
	runGitTest(t, g, repo, "remote", "add", "origin", remote)
	runGitTest(t, g, repo, "push", "-u", "origin", "main")

	runGitTest(t, g, repo, "checkout", "--detach")
	if err := os.WriteFile(filepath.Join(repo, "detached"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, g, repo, "add", ".")
	runGitTest(t, g, repo, "commit", "-m", "detached")
	runGitTest(t, g, repo, "tag", "local-tag")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("stash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, g, repo, "stash", "push", "-m", "local stash")

	loss, err := repoLoss(ctx, g, "repo", repo, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(loss, "\n")
	if !strings.Contains(joined, "unpushed commits") || !strings.Contains(joined, "has a stash") {
		t.Fatalf("loss = %q", joined)
	}
	if _, err := repoLoss(ctx, g, "broken", filepath.Join(t.TempDir(), "missing"), false); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("missing repository error = %v", err)
	}
}
