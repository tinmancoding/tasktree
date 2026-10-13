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

func setupRecords(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("TASKTREE_HOME", home)
	t.Setenv("TASKTREE_CACHE_DIR", filepath.Join(home, "cache"))
	t.Setenv("TASKTREE_V1_ALIASES_FILE", filepath.Join(home, "missing"))
	if _, _, err := run(t, "setup", "--author", "test", "--machine", "box"); err != nil {
		t.Fatal(err)
	}
	return home
}
func newRecord(t *testing.T, title, handle string, extra ...string) string {
	t.Helper()
	args := append([]string{"new", title, "--handle", handle}, extra...)
	out, _, err := run(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(out)[1]
}

func TestRecordCommandsAndHierarchy(t *testing.T) {
	home := setupRecords(t)
	parent := newRecord(t, "Parent", "parent")
	child := newRecord(t, "Child", "child", "--parent", "parent")
	_ = newRecord(t, "Grandchild", "grandchild", "--parent", "child")
	file := filepath.Join(t.TempDir(), "description")
	if err := os.WriteFile(file, []byte("new description"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "info", "edit", "--file", file, "-t", "parent"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "info", "edit", "--file", file, "-t", "parent"); err != nil {
		t.Fatal(err)
	}
	if got := gitRun(t, filepath.Join(home, "git"), "log", "-1", "--format=%s"); got != "task("+parent+"): description updated" {
		t.Fatalf("description commit %q", got)
	}
	if err := os.WriteFile(file, []byte("bad\n## Log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "info", "edit", "--file", file, "-t", "parent"); err == nil || cli.Code(err) != 2 {
		t.Fatalf("reserved heading: %v", err)
	}
	if _, _, err := run(t, "info", "set", "handle", "parent", "-t", "child"); err == nil || cli.Code(err) != 5 {
		t.Fatalf("handle collision: %v", err)
	}
	if _, _, err := run(t, "info", "set", "parent", "grandchild", "-t", "parent"); err == nil || cli.Code(err) != 2 {
		t.Fatalf("cycle: %v", err)
	}
	if _, _, err := run(t, "label", "add", "one", "two", "-t", "child"); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "list", "--label", "one", "--label", "two", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []record.Task
	if err := json.Unmarshal([]byte(out), &listed); err != nil || len(listed) != 1 || listed[0].ID != child {
		t.Fatalf("labels list=%s err=%v", out, err)
	}
	out, _, err = run(t, "list", "--label", "one", "--label", "two")
	parentRow := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "parent ") {
			parentRow = true
		}
	}
	if err != nil || !strings.Contains(out, "child") || parentRow {
		t.Fatalf("filtered rows=%q err=%v", out, err)
	}
	out, _, err = run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "  child") || !strings.Contains(out, "    grandchild") || !strings.Contains(out, "0/1") {
		t.Fatalf("tree output:\n%s", out)
	}
	jsonInfo, _, err := run(t, "info", "--json", "-t", "parent")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Focus    *string                               `json:"focus"`
		Subtasks []struct{ ID, Handle, Status string } `json:"subtasks"`
	}
	if err := json.Unmarshal([]byte(jsonInfo), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Focus != nil || len(payload.Subtasks) != 1 || payload.Subtasks[0].ID != child || payload.Subtasks[0].Handle != "child" {
		t.Fatalf("info json=%s", jsonInfo)
	}
	if _, _, err := run(t, "mark", "done", "--cascade", "--keep", "-t", "parent"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, parent).Status; got != "done" {
		t.Fatal(got)
	}
	if got := taskInfo(t, child).Status; got != "done" {
		t.Fatal(got)
	}
}

func TestRefsURLsAndHistory(t *testing.T) {
	home := setupRecords(t)
	_ = newRecord(t, "Refs", "refs")
	if _, _, err := run(t, "config", "set", "jira.baseUrl", "https://jira.example/"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"ref", "add", "jira", "PROJ-1", "-t", "refs"}, {"ref", "add", "pr", "owner/repo#12", "-t", "refs"}} {
		if _, _, err := run(t, args...); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := run(t, "info", "-t", "refs")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "https://jira.example/browse/PROJ-1") || !strings.Contains(out, "https://github.com/owner/repo/pull/12") {
		t.Fatalf("urls: %s", out)
	}
	out, _, err = run(t, "info", "-t", "refs", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"url":"https://jira.example/browse/PROJ-1"`) {
		t.Fatalf("json refs: %s", out)
	}
	out, _, err = run(t, "history", "--json", "-t", "refs")
	if err != nil {
		t.Fatal(err)
	}
	var history []map[string]any
	if err := json.Unmarshal([]byte(out), &history); err != nil || len(history) != 3 {
		t.Fatalf("history=%s err=%v", out, err)
	}
	if !strings.Contains(history[0]["subject"].(string), "ref added") {
		t.Fatalf("newest history: %#v", history[0])
	}
	if _, _, err := run(t, "config", "set", "jira.baseUrl", ""); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "info", "-t", "refs", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	for _, raw := range payload["refs"].([]any) {
		r := raw.(map[string]any)
		if r["type"] == "jira" {
			if _, ok := r["url"]; ok {
				t.Fatalf("jira url without base: %#v", r)
			}
		}
	}
	if got := gitRun(t, filepath.Join(home, "git"), "status", "--porcelain"); got != "" {
		t.Fatalf("dirty hub: %q", got)
	}
}
