package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tasktree Test",
		"GIT_AUTHOR_EMAIL=tasktree@example.com",
		"GIT_COMMITTER_NAME=Tasktree Test",
		"GIT_COMMITTER_EMAIL=tasktree@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func CreateRemoteRepo(t *testing.T) (string, func(*testing.T) string) {
	t.Helper()
	base := t.TempDir()
	remotePath := filepath.Join(base, "app.git")
	workPath := filepath.Join(base, "work")
	RunGit(t, base, "init", "--bare", remotePath)
	RunGit(t, base, "clone", remotePath, workPath)
	RunGit(t, workPath, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(workPath, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	RunGit(t, workPath, "add", "README.md")
	RunGit(t, workPath, "commit", "-m", "seed repo")
	RunGit(t, workPath, "push", "-u", "origin", "main")
	RunGit(t, remotePath, "symbolic-ref", "HEAD", "refs/heads/main")

	mutate := func(t *testing.T) string {
		t.Helper()
		contentPath := filepath.Join(workPath, "README.md")
		current, err := os.ReadFile(contentPath)
		if err != nil {
			t.Fatalf("read content file: %v", err)
		}
		current = append(current, []byte("next\n")...)
		if err := os.WriteFile(contentPath, current, 0o644); err != nil {
			t.Fatalf("write content file: %v", err)
		}
		RunGit(t, workPath, "add", "README.md")
		RunGit(t, workPath, "commit", "-m", "update repo")
		RunGit(t, workPath, "push", "origin", "main")
		return strings.TrimSpace(RunGit(t, workPath, "rev-parse", "HEAD"))
	}

	return remotePath, mutate
}

func RunTasktree(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	stdout, stderr := RunTasktreeSplit(t, cwd, args...)
	return stdout + stderr
}

func RunTasktreeSplit(t *testing.T, cwd string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(tasktreeBinary(t), args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("tasktree %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func RunTasktreeExpectError(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command(tasktreeBinary(t), args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected tasktree %s to fail, output:\n%s", strings.Join(args, " "), output)
	}
	return string(output)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func tasktreeBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tasktree-test")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/tasktree")
	build.Dir = root
	build.Env = os.Environ()
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build tasktree binary failed: %v\n%s", err, output)
	}
	return binPath
}
