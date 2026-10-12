package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
)

func TestRunUsesWorkingDirectoryAndStripsInheritedGitEnv(t *testing.T) {
	dir := t.TempDir()
	client := gitx.NewClient()
	if _, _, err := client.Run(context.Background(), dir, nil, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	old, had := os.LookupEnv("GIT_DIR")
	if err := os.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "invalid")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if had {
			_ = os.Setenv("GIT_DIR", old)
		} else {
			_ = os.Unsetenv("GIT_DIR")
		}
	}()
	out, _, err := client.Run(context.Background(), dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != want {
		t.Fatalf("top level = %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestRunAcceptsPerInvocationGitEnvironment(t *testing.T) {
	client := gitx.NewClient()
	gitDir := filepath.Join(t.TempDir(), "repo.git")
	if _, _, err := client.Run(context.Background(), "", map[string]string{"GIT_DIR": gitDir}, "init", "--bare"); err != nil {
		t.Fatal(err)
	}
	out, _, err := client.Run(context.Background(), "", map[string]string{"GIT_DIR": gitDir}, "rev-parse", "--is-bare-repository")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "true" {
		t.Fatalf("got %q", out)
	}
}
