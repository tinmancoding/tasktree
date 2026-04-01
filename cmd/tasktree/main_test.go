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

	// Isolate the registry to a temp HOME.
	t.Setenv("HOME", t.TempDir())

	initOutput := testutil.RunTasktree(t, workspace, "init", tasktreeRoot)
	if !strings.Contains(initOutput, "Initialized tasktree") {
		t.Fatalf("unexpected init output: %q", initOutput)
	}

	addOutput := testutil.RunTasktree(t, tasktreeRoot, "add", remoteURL, "--branch", "feature/payments", "--name", "app")
	if !strings.Contains(addOutput, "Added app at app") {
		t.Fatalf("unexpected add output: %q", addOutput)
	}

	// `repos` lists repositories in the current tasktree.
	reposOutput := testutil.RunTasktree(t, tasktreeRoot, "repos")
	for _, expected := range []string{"NAME", "app", "feature/payments"} {
		if !strings.Contains(reposOutput, expected) {
			t.Fatalf("repos output %q missing %q", reposOutput, expected)
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
	reposAfterRemove := testutil.RunTasktree(t, tasktreeRoot, "repos")
	if strings.Contains(reposAfterRemove, "app") {
		t.Fatalf("expected app to be absent after remove, got %q", reposAfterRemove)
	}
}

func TestCLIListShowsKnownTasktrees(t *testing.T) {
	// Isolate the registry to a temp HOME.
	t.Setenv("HOME", t.TempDir())

	workspace := t.TempDir()
	ws1 := filepath.Join(workspace, "alpha")
	ws2 := filepath.Join(workspace, "beta")

	// List with no registered tasktrees.
	emptyOutput := testutil.RunTasktree(t, workspace, "list")
	if !strings.Contains(emptyOutput, "No tasktrees registered.") {
		t.Fatalf("expected empty list message, got: %q", emptyOutput)
	}

	testutil.RunTasktree(t, workspace, "init", ws1)
	testutil.RunTasktree(t, workspace, "init", ws2)

	listOutput := testutil.RunTasktree(t, workspace, "list")
	for _, expected := range []string{"NAME", "alpha", "beta"} {
		if !strings.Contains(listOutput, expected) {
			t.Fatalf("list output %q missing %q", listOutput, expected)
		}
	}
}

func TestCLIListAnnotatesMissingTasktree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	workspace := t.TempDir()
	ws := filepath.Join(workspace, "ephemeral")

	testutil.RunTasktree(t, workspace, "init", ws)

	// Remove the directory after init so the registry entry becomes stale.
	if err := os.RemoveAll(ws); err != nil {
		t.Fatalf("remove workspace: %v", err)
	}

	listOutput := testutil.RunTasktree(t, workspace, "list")
	if !strings.Contains(listOutput, "missing") {
		t.Fatalf("expected (missing) annotation, got: %q", listOutput)
	}
}

func TestCLIShowsHelpfulErrorOutsideTasktree(t *testing.T) {
	output := testutil.RunTasktreeExpectError(t, t.TempDir(), "status")
	if !strings.Contains(output, "Not inside a tasktree") {
		t.Fatalf("unexpected error output: %q", output)
	}
}

func TestCLIReposFailsOutsideTasktree(t *testing.T) {
	output := testutil.RunTasktreeExpectError(t, t.TempDir(), "repos")
	if !strings.Contains(output, "Not inside a tasktree") {
		t.Fatalf("unexpected error output: %q", output)
	}
}

func TestCLIVerbosePrintsGitOperationsToStderr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
