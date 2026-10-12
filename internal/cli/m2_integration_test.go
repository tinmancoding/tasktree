package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func TestInfoListsClosedDirectSubtasks(t *testing.T) {
	setupRecords(t)
	_ = newRecord(t, "Parent", "p")
	_ = newRecord(t, "Open", "open-child", "--parent", "p")
	_ = newRecord(t, "Closed", "closed-child", "--parent", "p")
	if _, _, err := run(t, "mark", "done", "--keep", "-t", "closed-child"); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "info", "-t", "p")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Progress: 1/2", "open-child: incoming", "closed-child: done"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info missing %q:\n%s", want, out)
		}
	}
}

func TestFocusPrecedenceAndAutoClear(t *testing.T) {
	f := newCloseFixture(t)
	parent, _, root := createAndOpen(t, "Focus parent", filepath.Join(f.tasks, "focus-parent"))
	out, _, err := run(t, "new", "Focus child", "--handle", "focus-child", "--parent", parent)
	if err != nil {
		t.Fatal(err)
	}
	child := strings.Fields(out)[0]
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "focus", child); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "note", "focused note"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, child); len(got.Log) != 1 || got.Log[0].Body != "focused note" {
		t.Fatalf("focused log=%+v", got.Log)
	}
	t.Setenv("TASKTREE_TASK", parent)
	if _, _, err = run(t, "note", "env note"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, parent); len(got.Log) != 1 || got.Log[0].Body != "env note" {
		t.Fatalf("env log=%+v", got.Log)
	}
	os.Unsetenv("TASKTREE_TASK")
	info, _, err := run(t, "info")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info, "Focus: "+child) {
		t.Fatalf("missing focus:\n%s", info)
	}
	if _, _, err = run(t, "mark", "done", "--keep", "-t", child); err != nil {
		t.Fatal(err)
	}
	local := string(mustRead(t, filepath.Join(f.home, "local.yml")))
	if strings.Contains(local, "focus:") {
		t.Fatalf("focus not cleared:\n%s", local)
	}
}

func TestIsolatedForkRecursionAndPushWarning(t *testing.T) {
	f := newCloseFixture(t)
	parent, _, root := createAndOpen(t, "Iso parent", filepath.Join(f.tasks, "iso-parent"))
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "work", "--branch", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "follow"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "work", "parent-commit"), []byte("parent\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(root, "work"), "add", "parent-commit")
	gitRun(t, filepath.Join(root, "work"), "commit", "-m", "parent")
	if err := os.WriteFile(filepath.Join(root, "work", "dirty"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, warn, err := run(t, "new", "Iso child", "--handle", "iso-child", "--parent", parent, "--isolated")
	if err != nil {
		t.Fatal(err)
	}
	childRoot := strings.TrimSpace(out)
	if !strings.Contains(warn, "work: parent has uncommitted changes") {
		t.Fatalf("dirty warning=%q", warn)
	}
	if _, err = os.Stat(filepath.Join(childRoot, "follow")); !os.IsNotExist(err) {
		t.Fatalf("default included follow repo: %v", err)
	}
	if got := gitRun(t, filepath.Join(childRoot, "work"), "branch", "--show-current"); got != "feature-iso-child" {
		t.Fatalf("branch=%q", got)
	}
	if got := gitRun(t, filepath.Join(childRoot, "work"), "show", "HEAD:parent-commit"); got != "parent" {
		t.Fatalf("fork content=%q", got)
	}
	if err = os.Chdir(childRoot); err != nil {
		t.Fatal(err)
	}
	pushOut, pushErr, err := run(t, "push", "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pushErr, "push "+parent+"'s work first") || !strings.Contains(pushOut, "gh pr create") {
		t.Fatalf("push out=%q err=%q", pushOut, pushErr)
	}
	grandOut, _, err := run(t, "new", "Tests", "--handle", "tests", "--parent", "iso-child", "--isolated")
	if err != nil {
		t.Fatal(err)
	}
	if got := gitRun(t, filepath.Join(strings.TrimSpace(grandOut), "work"), "branch", "--show-current"); got != "feature-iso-child-tests" {
		t.Fatalf("recursive branch=%q", got)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(root, "work"), "push", "-u", "origin", "feature")
	if err = os.Chdir(childRoot); err != nil {
		t.Fatal(err)
	}
	_, pushErr, err = run(t, "push", "work")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pushErr, "PR will include") {
		t.Fatalf("stale warning=%q", pushErr)
	}
}

func TestIsolatedRequiresCheckedOutParent(t *testing.T) {
	setupRecords(t)
	_ = newRecord(t, "Parent", "not-open")
	_, _, err := run(t, "new", "Child", "--parent", "not-open", "--isolated")
	if cli.Code(err) != 6 || !strings.Contains(err.Error(), "open not-open first") {
		t.Fatalf("code=%d err=%v", cli.Code(err), err)
	}
}

func TestModeAndBootstrap(t *testing.T) {
	f := newCloseFixture(t)
	handle, _, root := createAndOpen(t, "M2 commands", filepath.Join(f.tasks, "m2"))
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "--branch", "topic"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "mode", "repo", "pin"); err != nil {
		t.Fatal(err)
	}
	gm := string(mustRead(t, filepath.Join(root, ".gitmodules")))
	if strings.Contains(gm, "branch =") || !strings.Contains(gm, "tasktree-mode = pin") {
		t.Fatalf("pin config:\n%s", gm)
	}
	gitRun(t, filepath.Join(root, "repo"), "checkout", "--detach")
	if _, _, err := run(t, "mode", "repo", "work"); cli.Code(err) != 2 {
		t.Fatalf("detached mode code=%d err=%v", cli.Code(err), err)
	}
	id := taskInfo(t, handle).ID
	text := strings.Replace(`apiVersion: tasktree.dev/v2
kind: TasktreeConfig
taskId: PLACEHOLDER
bootstrap:
  - name: env
    run: test "$TASKTREE_ROOT" = "$PWD" && test -z "$GIT_DIR" && test "$GIT_SSH_COMMAND" = keep && echo ok > booted
`, "PLACEHOLDER", id, 1)
	if err := os.WriteFile(filepath.Join(root, ".tasktree", "config.yml"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", "should-be-stripped")
	t.Setenv("GIT_SSH_COMMAND", "keep")
	_, stderr, err := run(t, "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "==> [env]") {
		t.Fatalf("bootstrap output=%q", stderr)
	}
	if _, err = os.Stat(filepath.Join(root, "booted")); err != nil {
		t.Fatal(err)
	}
}
