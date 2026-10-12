package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tinmancoding/tasktree/v2/internal/cli"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := cli.Execute(context.Background(), args, &out, &errOut)
	return out.String(), errOut.String(), err
}
func TestCommandFlowAndExitCodes(t *testing.T) {
	home := t.TempDir()
	old, had := os.LookupEnv("TASKTREE_HOME")
	os.Setenv("TASKTREE_HOME", home)
	defer func() {
		if had {
			os.Setenv("TASKTREE_HOME", old)
		} else {
			os.Unsetenv("TASKTREE_HOME")
		}
	}()
	if _, _, err := run(t, "setup", "--author", "config-author", "--machine", "testbox"); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "new", "Fix Login")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "fix-login ") {
		t.Fatalf("new: %q", out)
	}
	out, _, err = run(t, "new", "Jira work", "--jira", "DSA-8946")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "DSA-8946 ") {
		t.Fatalf("jira: %q", out)
	}
	_, _, err = run(t, "new", "Collision", "--handle", "fix-login")
	if cli.Code(err) != 5 || !strings.Contains(err.Error(), "fix-login-2") {
		t.Fatalf("collision code=%d err=%v", cli.Code(err), err)
	}
	out, _, err = run(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []record.Task
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("list=%d", len(listed))
	}
	os.Setenv("TASKTREE_AUTHOR", "env-author")
	if _, _, err = run(t, "--author", "flag-author", "note", "-t", "fix-login", "hello"); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("TASKTREE_AUTHOR")
	if _, _, err = run(t, "mark", "done", "-t", "fix-login"); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("default list included closed: %s", out)
	}
	out, _, err = run(t, "list", "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(out), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("all=%s err=%v", out, err)
	}
	out, _, err = run(t, "info", "-t", "fix-login", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var task record.Task
	if err = json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Log) != 1 || task.Log[0].Author != "flag-author" || task.Log[0].Machine != "testbox" {
		t.Fatalf("log=%+v", task.Log)
	}
	_, _, err = run(t, "mark", "bogus", "-t", "fix-login")
	if err == nil {
		t.Fatal("invalid status accepted")
	}
	_, _, err = run(t, "info", "-t", "missing")
	if cli.Code(err) != 3 {
		t.Fatalf("not found code=%d", cli.Code(err))
	}
	_, _, err = run(t, "info")
	if cli.Code(err) != 4 {
		t.Fatalf("missing target code=%d err=%v", cli.Code(err), err)
	}
	// A duplicate handle can arrive via sync/manual edits and must be ambiguous.
	now := time.Now().UTC()
	dup, e := record.New(record.Task{ID: "def456", Handle: "fix-login", Title: "Duplicate", Status: "incoming", Labels: []string{}, Refs: []record.Ref{}, CreatedAt: now, UpdatedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(home, "git", "tasks", "def456.md"), dup.Render(), 0o644); e != nil {
		t.Fatal(e)
	}
	_, _, err = run(t, "info", "-t", "fix-login")
	if cli.Code(err) != 5 || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous code=%d err=%v", cli.Code(err), err)
	}
}
