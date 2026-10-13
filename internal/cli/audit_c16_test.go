package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H4: cascading from inside a checkout that the cascade deletes must not trip
// over the vanished working directory.
func TestCascadeFromInsideDeletedSubtaskCheckout(t *testing.T) {
	f := newCloseFixture(t)
	parent, _, parentRoot := createAndOpen(t, "Cascade cwd parent", filepath.Join(f.tasks, "parent"))
	child, _, childRoot := createAndOpen(t, "Cascade cwd child", filepath.Join(f.tasks, "child"))
	if _, _, err := run(t, "info", "set", "parent", parent, "-t", child); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(parentRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "--branch", "ptopic"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "commit", "-m", "parent work"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(childRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "--branch", "ctopic"); err != nil {
		t.Fatal(err)
	}
	_, hint, err := run(t, "mark", "done", "--cascade", "-t", parent)
	if err != nil {
		t.Fatalf("cascade from deleted cwd: %v", err)
	}
	if !strings.Contains(hint, "cd ..") {
		t.Fatalf("missing cd hint: %q", hint)
	}
	if _, err := os.Stat(childRoot); !os.IsNotExist(err) {
		t.Fatalf("child checkout remains: %v", err)
	}
	if got := taskInfo2(t, parent); got != "done" {
		t.Fatalf("parent=%s", got)
	}
	_ = child
}

func taskInfo2(t *testing.T, ref string) string {
	t.Helper()
	if err := os.Chdir(os.TempDir()); err != nil {
		t.Fatal(err)
	}
	return taskInfo(t, ref).Status
}

// H5: clearing a parent can never create a cycle, so it must work on a task
// that is already part of one.
func TestInfoSetParentClearBreaksExistingCycle(t *testing.T) {
	f := newCloseFixture(t)
	a, aID, _ := createAndOpen(t, "Cycle a", filepath.Join(f.tasks, "a"))
	b, bID, _ := createAndOpen(t, "Cycle b", filepath.Join(f.tasks, "b"))
	if _, _, err := run(t, "info", "set", "parent", a, "-t", b); err != nil {
		t.Fatal(err)
	}
	// Simulate two machines reparenting in opposite directions.
	hubTasks := filepath.Join(f.home, "git", "tasks")
	recA := filepath.Join(hubTasks, aID+".md")
	recB := filepath.Join(hubTasks, bID+".md")
	if _, err := os.Stat(recA); err != nil {
		matches, _ := filepath.Glob(filepath.Join(hubTasks, "*"))
		t.Fatalf("record layout: %v %v", err, matches)
	}
	body, err := os.ReadFile(recA)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "parent: null") {
		t.Fatalf("expected null parent in record:\n%s", body)
	}
	body = []byte(strings.Replace(string(body), "parent: null", "parent: "+bID, 1))
	if err := os.WriteFile(recA, body, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = recB
	hub := filepath.Join(f.home, "git")
	gitRun(t, hub, "add", "-A")
	gitRun(t, hub, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "make a cycle")
	if _, _, err := run(t, "info", "-t", a, "--json"); err == nil {
		t.Fatal("expected cycle error from info")
	}
	if out, _, _ := run(t, "doctor", "--json"); !strings.Contains(out, "info set parent --clear") {
		t.Fatalf("doctor parent-cycle lacks the repair hint: %s", out)
	}
	if _, _, err := run(t, "info", "set", "parent", "--clear", "-t", a); err != nil {
		t.Fatalf("clear on a cycle member: %v", err)
	}
	if got := taskInfo(t, a); got.Parent != nil {
		t.Fatalf("parent not cleared: %v", *got.Parent)
	}
	if got := taskInfo(t, b); got.Parent == nil {
		t.Fatal("unrelated parent was cleared")
	}
}
