package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type closeFixture struct {
	base, home, tasks, upstream string
}

func newCloseFixture(t *testing.T) closeFixture {
	t.Helper()
	base := t.TempDir()
	f := closeFixture{
		base:     base,
		home:     filepath.Join(base, "home"),
		tasks:    filepath.Join(base, "tasks"),
		upstream: filepath.Join(base, "upstream"),
	}
	t.Setenv("TASKTREE_HOME", f.home)
	t.Setenv("TASKTREE_CACHE_DIR", filepath.Join(base, "cache"))
	t.Setenv("TASKTREE_V1_ALIASES_FILE", filepath.Join(base, "missing"))
	if err := os.MkdirAll(f.tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.upstream, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, f.upstream, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(f.upstream, "README"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, f.upstream, "add", ".")
	gitRun(t, f.upstream, "commit", "-m", "initial")
	gitRun(t, f.upstream, "config", "receive.denyCurrentBranch", "updateInstead")
	if _, _, err := run(t, "setup", "--author", "test", "--machine", "box", "--tasks-root", f.tasks); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	return f
}

func createAndOpen(t *testing.T, title, path string) (handle, id, root string) {
	t.Helper()
	out, _, err := run(t, "new", title)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		t.Fatalf("new output: %q", out)
	}
	out, _, err = run(t, "open", fields[0], "--path", path)
	if err != nil {
		t.Fatal(err)
	}
	return fields[0], fields[1], strings.TrimSpace(out)
}

func taskInfo(t *testing.T, ref string) record.Task {
	t.Helper()
	out, _, err := run(t, "info", "-t", ref, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var task record.Task
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestNewOpenAndSharedSubtaskCloseRules(t *testing.T) {
	f := newCloseFixture(t)
	parentPath := filepath.Join(f.tasks, "chosen-parent")
	out, errOut, err := run(t, "new", "Parent", "--open", "--path", parentPath)
	if err != nil {
		t.Fatal(err)
	}
	if out != parentPath+"\n" {
		t.Fatalf("new --open stdout=%q want exactly path", out)
	}
	parentFields := strings.Fields(errOut)
	if len(parentFields) != 2 || parentFields[0] != "parent" {
		t.Fatalf("new --open identity stderr=%q", errOut)
	}
	parentID := parentFields[1]
	if err := os.Chdir(parentPath); err != nil {
		t.Fatal(err)
	}

	out, _, err = run(t, "new", "Shared child", "--parent", "parent")
	if err != nil {
		t.Fatal(err)
	}
	child := strings.Fields(out)[0]
	if _, _, err = run(t, "note", "-t", child, "worked in parent"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, child); len(got.Log) != 1 || got.Log[0].Body != "worked in parent" {
		t.Fatalf("shared child note: %+v", got.Log)
	}
	_, _, err = run(t, "open", child)
	if cli.Code(err) != 6 || !strings.Contains(err.Error(), "isolated subtasks arrive in M2") {
		t.Fatalf("open shared child code=%d err=%v", cli.Code(err), err)
	}
	_, _, err = run(t, "mark", "done", "-t", parentID)
	if cli.Code(err) != 6 || !strings.Contains(err.Error(), child) {
		t.Fatalf("parent close with open child code=%d err=%v", cli.Code(err), err)
	}
	if _, _, err = run(t, "mark", "done", "-t", child); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(parentPath); statErr != nil {
		t.Fatalf("closing shared child affected parent checkout: %v", statErr)
	}
	if _, _, err = run(t, "mark", "done", "-t", parentID, "--keep"); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(parentPath); statErr != nil {
		t.Fatalf("--keep removed checkout: %v", statErr)
	}

	unsafeHandle, _, unsafeRoot := createAndOpen(t, "Unsafe marker", filepath.Join(f.tasks, "unsafe"))
	marker := filepath.Join(unsafeRoot, ".tasktree", "config.yml")
	if err := os.WriteFile(marker, []byte("apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: wrong1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = run(t, "mark", "done", "-t", unsafeHandle)
	if cli.Code(err) != 1 || !strings.Contains(err.Error(), "task marker missing or mismatched") {
		t.Fatalf("unsafe teardown code=%d err=%v", cli.Code(err), err)
	}
	if _, statErr := os.Stat(unsafeRoot); statErr != nil {
		t.Fatalf("unsafe checkout was deleted: %v", statErr)
	}
	if got := taskInfo(t, unsafeHandle).Status; got != "done" {
		t.Fatalf("status did not remain closed after partial teardown: %s", got)
	}
}

func TestCloseGateForceAndSuccessfulReopen(t *testing.T) {
	f := newCloseFixture(t)

	// The close gate reports every offending repository, and --force proceeds.
	forceHandle, _, forceRoot := createAndOpen(t, "Force close", filepath.Join(f.tasks, "force"))
	if err := os.Chdir(forceRoot); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", f.upstream, "--name", "dirty"}, {"add", f.upstream, "--name", "unpushed", "--branch", "topic"}} {
		if _, _, err := run(t, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(forceRoot, "dirty", "change"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forceRoot, "unpushed", "commit"), []byte("commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(forceRoot, "unpushed"), "add", "commit")
	gitRun(t, filepath.Join(forceRoot, "unpushed"), "commit", "-m", "local only")
	if _, _, err := run(t, "mark", "done", "-t", forceHandle, "--keep"); err != nil {
		t.Fatalf("--keep unexpectedly ran the close gate: %v", err)
	}
	if _, statErr := os.Stat(forceRoot); statErr != nil {
		t.Fatalf("--keep removed checkout: %v", statErr)
	}
	if _, _, err := run(t, "mark", "active", "-t", forceHandle); err != nil {
		t.Fatal(err)
	}
	_, gateOut, err := run(t, "mark", "done", "-t", forceHandle)
	if cli.Code(err) != 6 || !strings.Contains(err.Error(), "dirty has uncommitted changes") || !strings.Contains(err.Error(), "unpushed has unpushed commits") {
		t.Fatalf("close gate code=%d stderr=%q err=%v", cli.Code(err), gateOut, err)
	}
	if gateOut != "" {
		t.Fatalf("close gate details printed twice by executable path: %q", gateOut)
	}
	_, forceLoss, err := run(t, "mark", "done", "-t", forceHandle, "--force")
	if err != nil || !strings.Contains(forceLoss, "dirty has uncommitted changes") || !strings.Contains(forceLoss, "unpushed has unpushed commits") {
		t.Fatalf("forced close stderr=%q err=%v", forceLoss, err)
	}
	if _, statErr := os.Stat(forceRoot); !os.IsNotExist(statErr) {
		t.Fatalf("forced close left checkout: %v", statErr)
	}

	// A lossless close commits root artifacts and gitlinks, tears down locally,
	// and can be reopened with the same branch intent and concrete commits.
	handle, id, root := createAndOpen(t, "Lossless close", filepath.Join(f.tasks, "lossless"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "--branch", "feature"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifact.txt"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "repo", "work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(root, "repo"), "add", "work.txt")
	gitRun(t, filepath.Join(root, "repo"), "commit", "-m", "work")
	gitRun(t, filepath.Join(root, "repo"), "push", "-u", "origin", "feature")
	wantGitlink := gitRun(t, filepath.Join(root, "repo"), "rev-parse", "HEAD")
	if _, _, err := run(t, "mark", "done", "-t", handle); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, handle).Status; got != "done" {
		t.Fatalf("closed status=%s", got)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("working tree remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.home, "clones", id+".git")); !os.IsNotExist(err) {
		t.Fatalf("clone git dir remains: %v", err)
	}
	if strings.Contains(string(mustRead(t, filepath.Join(f.home, "local.yml"))), id+":") {
		t.Fatalf("local checkout entry remains for %s", id)
	}
	hub := filepath.Join(f.home, "git")
	if got := gitRun(t, hub, "show", "task/"+id+":artifact.txt"); got != "kept" {
		t.Fatalf("artifact on task branch=%q", got)
	}
	if got := gitRun(t, hub, "rev-parse", "task/"+id+":repo"); got != wantGitlink {
		t.Fatalf("closed gitlink=%s want %s", got, wantGitlink)
	}
	gitRun(t, hub, "show-ref", "--verify", "refs/heads/task/"+id)
	if subject := gitRun(t, hub, "log", "-1", "--format=%s", "task/"+id); subject != "tasktree("+id+"): close: done" {
		t.Fatalf("close commit subject=%q", subject)
	}

	if err := os.Chdir(f.tasks); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "mark", "active", "-t", handle); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "open", handle)
	if err != nil {
		t.Fatal(err)
	}
	reopened := strings.TrimSpace(out)
	if got := string(mustRead(t, filepath.Join(reopened, "artifact.txt"))); got != "kept\n" {
		t.Fatalf("reopened artifact=%q", got)
	}
	if got := gitRun(t, filepath.Join(reopened, "repo"), "rev-parse", "HEAD"); got != wantGitlink {
		t.Fatalf("reopened repo=%s want %s", got, wantGitlink)
	}
	if got := gitRun(t, filepath.Join(reopened, "repo"), "branch", "--show-current"); got != "feature" {
		t.Fatalf("reopened work branch=%q", got)
	}
}
