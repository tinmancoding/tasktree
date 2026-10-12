package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func TestCascadePreflightAbortsThenClosesCheckedOutChild(t *testing.T) {
	f := newCloseFixture(t)
	parent, _, _ := createAndOpen(t, "Cascade parent", filepath.Join(f.tasks, "parent"))
	child, _, childRoot := createAndOpen(t, "Cascade child", filepath.Join(f.tasks, "child"))
	if _, _, err := run(t, "info", "set", "parent", parent, "-t", child); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(childRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "--branch", "topic"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "repo", "local"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(childRoot, "repo"), "add", "local")
	gitRun(t, filepath.Join(childRoot, "repo"), "commit", "-m", "unpushed")
	_, _, err := run(t, "mark", "done", "--cascade", "-t", parent)
	if cli.Code(err) != 6 || !strings.Contains(err.Error(), "child:") {
		t.Fatalf("cascade gate code=%d err=%v", cli.Code(err), err)
	}
	if got := taskInfo(t, parent).Status; got != "incoming" {
		t.Fatalf("parent changed on preflight abort: %s", got)
	}
	if got := taskInfo(t, child).Status; got != "incoming" {
		t.Fatalf("child changed on preflight abort: %s", got)
	}
	_, forcedLoss, err := run(t, "mark", "done", "--cascade", "--force", "-t", parent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forcedLoss, child+": repo has unpushed commits") {
		t.Fatalf("cascade forced loss=%q", forcedLoss)
	}
	if got := taskInfo(t, parent).Status; got != "done" {
		t.Fatalf("parent=%s", got)
	}
	if got := taskInfo(t, child).Status; got != "done" {
		t.Fatalf("child=%s", got)
	}
	if _, err := os.Stat(childRoot); !os.IsNotExist(err) {
		t.Fatalf("checked-out child was not torn down: %v", err)
	}
}
