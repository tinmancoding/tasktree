package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/app"
	"github.com/tinmancoding/tasktree/v2/internal/cli"
	"github.com/tinmancoding/tasktree/v2/internal/home"
)

func TestSyncCLIOutsideMeansAllAndConflictDisplay(t *testing.T) {
	t.Setenv("TASKTREE_HOME", t.TempDir())
	t.Setenv("TASKTREE_CACHE_DIR", t.TempDir())
	t.Setenv("TASKTREE_V1_ALIASES_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := filepath.Join(t.TempDir(), "tasks")
	if _, _, e := run(t, "setup", "--author", "test", "--machine", "local", "--tasks-root", root); e != nil {
		t.Fatal(e)
	}
	if _, _, e := run(t, "new", "Task"); e != nil {
		t.Fatal(e)
	}
	out, _, e := run(t, "sync", "--json")
	if e != nil {
		t.Fatal(e)
	}
	var implicit app.SyncReport
	if e = json.Unmarshal([]byte(out), &implicit); e != nil {
		t.Fatal(e)
	}
	out, _, e = run(t, "sync", "--all", "--json")
	if e != nil {
		t.Fatal(e)
	}
	var explicit app.SyncReport
	if e = json.Unmarshal([]byte(out), &explicit); e != nil {
		t.Fatal(e)
	}
	if strings.Join(implicit.Steps, "\n") != strings.Join(explicit.Steps, "\n") {
		t.Fatalf("outside differs from --all: %+v / %+v", implicit, explicit)
	}
	// A flagged record received from another machine must be visible in both
	// human and JSON output. This fixture is committed as an incoming hub file.
	h, _ := home.Resolve()
	a := app.New(h, nil)
	task, e := a.Info("task")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(h.GitPath(), "tasks", task.ID+".md")
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	b = []byte(strings.Replace(string(b), "status: incoming", "status: incoming\nconflict: [status, description]", 1))
	if e = os.WriteFile(path, b, 0644); e != nil {
		t.Fatal(e)
	}
	gitRun(t, h.GitPath(), "add", ".")
	gitRun(t, h.GitPath(), "commit", "-m", "fixture")
	out, _, e = run(t, "list")
	if e != nil || !strings.Contains(out, "incoming!") {
		t.Fatalf("list=%s err=%v", out, e)
	}
	out, _, e = run(t, "info", "-t", "task")
	if e != nil || !strings.Contains(out, "Conflict: status, description") {
		t.Fatalf("info=%s err=%v", out, e)
	}
	out, _, e = run(t, "info", "-t", "task", "--json")
	if e != nil || !strings.Contains(out, `"conflict":["status","description"]`) {
		t.Fatalf("info json=%s err=%v", out, e)
	}
	if _, _, e = run(t, "info", "ack", "status", "-t", "task"); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "empty")
	if e = os.WriteFile(file, nil, 0644); e != nil {
		t.Fatal(e)
	}
	if _, _, e = run(t, "info", "edit", "--file", file, "-t", "task"); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "conflict:") {
		t.Fatal("info edit/set did not remove empty flag")
	}
	// Short summaries are present and usable without initializing a real home.
	for _, command := range []string{"sync", "resolve"} {
		out, _, e = run(t, command, "--help")
		if e != nil || !strings.Contains(out, command) {
			t.Fatalf("help %s: %s %v", command, out, e)
		}
	}
	_, _, e = run(t, "resolve")
	if cli.Code(e) != 4 {
		t.Fatalf("resolve outside code=%d err=%v", cli.Code(e), e)
	}
	if _, _, e = run(t, "config", "set", "sync.jobs", "0"); cli.Code(e) != 2 {
		t.Fatalf("jobs code=%d err=%v", cli.Code(e), e)
	}
}
