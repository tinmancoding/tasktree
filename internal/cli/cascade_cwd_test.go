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
