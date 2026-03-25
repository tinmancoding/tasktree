package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/internal/testutil"
)

func TestCLIEndToEndFlow(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	workspace := t.TempDir()
	tasktreeRoot := filepath.Join(workspace, "feature-payments")

	initOutput := testutil.RunTasktree(t, workspace, "init", tasktreeRoot)
	if !strings.Contains(initOutput, "Initialized tasktree") {
		t.Fatalf("unexpected init output: %q", initOutput)
	}

	addOutput := testutil.RunTasktree(t, tasktreeRoot, "add", remoteURL, "--branch", "feature/payments", "--name", "app")
	if !strings.Contains(addOutput, "Added app at app") {
		t.Fatalf("unexpected add output: %q", addOutput)
	}

	listOutput := testutil.RunTasktree(t, tasktreeRoot, "list")
	for _, expected := range []string{"NAME", "app", "main", "feature/payments"} {
		if !strings.Contains(listOutput, expected) {
			t.Fatalf("list output %q missing %q", listOutput, expected)
		}
	}

	nestedDir := filepath.Join(tasktreeRoot, "app")
	statusOutput := testutil.RunTasktree(t, nestedDir, "status")
	for _, expected := range []string{"Tasktree: feature-payments", "REPO", "app", "feature/payments", "clean"} {
		if !strings.Contains(statusOutput, expected) {
			t.Fatalf("status output %q missing %q", statusOutput, expected)
		}
	}

	rootOutput := testutil.RunTasktree(t, filepath.Join(tasktreeRoot, "app"), "root")
	resolvedRoot, err := filepath.EvalSymlinks(tasktreeRoot)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if strings.TrimSpace(rootOutput) != resolvedRoot {
		t.Fatalf("root output = %q, want %q", rootOutput, resolvedRoot)
	}

	removeOutput := testutil.RunTasktree(t, tasktreeRoot, "remove", "app")
	if !strings.Contains(removeOutput, "Removed") {
		t.Fatalf("unexpected remove output: %q", removeOutput)
	}
	if _, err := os.Stat(filepath.Join(tasktreeRoot, "app")); !os.IsNotExist(err) {
		t.Fatalf("expected checkout to be removed, got %v", err)
	}
	listAfterRemove := testutil.RunTasktree(t, tasktreeRoot, "list")
	if strings.Contains(listAfterRemove, "app") {
		t.Fatalf("expected app to be absent after remove, got %q", listAfterRemove)
	}
}

func TestCLIShowsHelpfulErrorOutsideTasktree(t *testing.T) {
	output := testutil.RunTasktreeExpectError(t, t.TempDir(), "status")
	if !strings.Contains(output, "Not inside a tasktree") {
		t.Fatalf("unexpected error output: %q", output)
	}
}

func TestCLIVerbosePrintsGitOperationsToStderr(t *testing.T) {
	remoteURL, _ := testutil.CreateRemoteRepo(t)
	workspace := t.TempDir()
	tasktreeRoot := filepath.Join(workspace, "feature-payments")

	testutil.RunTasktree(t, workspace, "init", tasktreeRoot)

	stdout, stderr := testutil.RunTasktreeSplit(t, tasktreeRoot, "--verbose", "add", remoteURL, "--branch", "feature/payments", "--name", "app")
	if !strings.Contains(stdout, "Added app at app") {
		t.Fatalf("unexpected add stdout: %q", stdout)
	}
	for _, expected := range []string{"git clone --bare", "git clone ", "remote set-url origin", "checkout main", "checkout -b feature/payments"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("add stderr %q missing %q", stderr, expected)
		}
	}

	_, statusStderr := testutil.RunTasktreeSplit(t, filepath.Join(tasktreeRoot, "app"), "-v", "status")
	for _, expected := range []string{"git -C ", "symbolic-ref --quiet --short HEAD", "status --porcelain"} {
		if !strings.Contains(statusStderr, expected) {
			t.Fatalf("status stderr %q missing %q", statusStderr, expected)
		}
	}
}
