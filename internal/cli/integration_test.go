package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tinmancoding/tasktree/v2/internal/cli"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"os"
	"os/exec"
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
	if err == nil || cli.Code(err) != 2 {
		t.Fatalf("invalid status code=%d err=%v", cli.Code(err), err)
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

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v\n%s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}

func TestTasktreeEndToEnd(t *testing.T) {
	base := t.TempDir()
	homeDir, cacheDir, tasks, upstream := filepath.Join(base, "home"), filepath.Join(base, "cache"), filepath.Join(base, "tasks"), filepath.Join(base, "upstream")
	for k, v := range map[string]string{"TASKTREE_HOME": homeDir, "TASKTREE_CACHE_DIR": cacheDir, "TASKTREE_V1_ALIASES_FILE": filepath.Join(base, "missing")} {
		old, had := os.LookupEnv(k)
		os.Setenv(k, v)
		defer func(k, old string, had bool) {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		}(k, old, had)
	}
	os.MkdirAll(tasks, 0o755)
	os.MkdirAll(upstream, 0o755)
	gitRun(t, upstream, "init", "-b", "main")
	os.WriteFile(filepath.Join(upstream, "README"), []byte("hi\n"), 0o644)
	gitRun(t, upstream, "add", ".")
	gitRun(t, upstream, "commit", "-m", "init")
	gitRun(t, upstream, "tag", "v1")
	gitRun(t, upstream, "branch", "feature")
	if _, _, e := run(t, "setup", "--author", "test", "--machine", "box", "--tasks-root", tasks); e != nil {
		t.Fatal(e)
	}
	out, _, e := run(t, "new", "E2E")
	if e != nil {
		t.Fatal(e)
	}
	fields := strings.Fields(out)
	id := fields[1]
	out, _, e = run(t, "open", fields[0])
	if e != nil {
		t.Fatal(e)
	}
	root := strings.TrimSpace(out)
	if _, e := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(e) {
		t.Fatalf("root .git exists: %v", e)
	}
	plainStatus := exec.Command("git", "status")
	plainStatus.Dir = root
	if e := plainStatus.Run(); e == nil {
		t.Fatal("plain git status unexpectedly worked in root")
	}
	cmd := exec.Command("git", "merge-base", "main", "task/"+id)
	cmd.Dir = filepath.Join(homeDir, "git")
	if e := cmd.Run(); e == nil {
		t.Fatal("task branch is not orphan")
	}
	oldwd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldwd)
	for _, args := range [][]string{{"add", upstream}, {"add", upstream, "--name", "work", "--branch", "topic"}, {"add", upstream, "--name", "pin", "--from", "v1"}} {
		if _, _, e = run(t, args...); e != nil {
			t.Fatalf("%v: %v", args, e)
		}
	}
	gm := string(mustRead(t, filepath.Join(root, ".gitmodules")))
	for _, want := range []string{"tasktree-mode = follow", "tasktree-mode = work", "tasktree-mode = pin", "branch = main", "branch = topic"} {
		if !strings.Contains(gm, want) {
			t.Errorf(".gitmodules missing %q:\n%s", want, gm)
		}
	}
	for _, name := range []string{"upstream", "work", "pin"} {
		dot := string(mustRead(t, filepath.Join(root, name, ".git")))
		if !strings.HasPrefix(dot, "gitdir: /") {
			t.Errorf("relative gitdir: %s", dot)
		}
		if worktree := gitRun(t, filepath.Join(root, name), "config", "--get", "core.worktree"); !filepath.IsAbs(worktree) {
			t.Errorf("relative core.worktree: %s", worktree)
		}
		if _, e := os.Stat(filepath.Join(homeDir, "clones", id+".git", "modules", name, "objects", "info", "alternates")); !os.IsNotExist(e) {
			t.Errorf("alternates exists: %v", e)
		}
	}
	// Context discovery works from inside a nested submodule.
	if e := os.Chdir(filepath.Join(root, "upstream")); e != nil {
		t.Fatal(e)
	}
	rootOut, _, e := run(t, "root")
	wantRoot, _ := filepath.EvalSymlinks(root)
	gotRoot, _ := filepath.EvalSymlinks(strings.TrimSpace(rootOut))
	if e != nil || gotRoot != wantRoot {
		t.Fatalf("nested root=%q err=%v", rootOut, e)
	}
	// commit records a submodule commit as a gitlink, but does not commit dirty files itself.
	os.WriteFile(filepath.Join(root, "upstream", "second"), []byte("two\n"), 0o644)
	gitRun(t, filepath.Join(root, "upstream"), "add", "second")
	gitRun(t, filepath.Join(root, "upstream"), "commit", "-m", "second")
	if e := os.Chdir(root); e != nil {
		t.Fatal(e)
	}
	if _, _, e = run(t, "commit"); e != nil {
		t.Fatal(e)
	}
	gitlink := gitRun(t, filepath.Join(homeDir, "git"), "rev-parse", "task/"+id+":upstream")
	if gitlink != gitRun(t, filepath.Join(root, "upstream"), "rev-parse", "HEAD") {
		t.Fatal("hub gitlink was not updated")
	}
	gitRun(t, upstream, "config", "receive.denyCurrentBranch", "updateInstead")
	gitRun(t, filepath.Join(root, "upstream"), "push", "origin", "main")
	// Dirty removal is gated; --force reports the loss and proceeds.
	os.WriteFile(filepath.Join(root, "work", "dirty"), []byte("x"), 0o644)
	_, _, e = run(t, "remove", "work")
	if cli.Code(e) != 6 {
		t.Fatalf("dirty remove gate code=%d err=%v", cli.Code(e), e)
	}
	gitRun(t, filepath.Join(root, "work"), "add", "dirty")
	gitRun(t, filepath.Join(root, "work"), "commit", "-m", "unpushed")
	_, _, e = run(t, "remove", "work")
	if cli.Code(e) != 6 || !strings.Contains(e.Error(), "unpushed") {
		t.Fatalf("unpushed remove gate code=%d err=%v", cli.Code(e), e)
	}
	if _, _, e = run(t, "remove", "work", "--force"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = run(t, "add", upstream, "--name", "work", "--branch", "topic"); e != nil {
		t.Fatal(e)
	}
	out, _, e = run(t, "info", "--json")
	if e != nil {
		t.Fatal(e)
	}
	var info map[string]any
	if e = json.Unmarshal([]byte(out), &info); e != nil {
		t.Fatal(e)
	}
	tree := info["tasktree"].(map[string]any)
	if len(tree["repos"].([]any)) != 3 {
		t.Fatalf("info: %s", out)
	}
	if out, _, e = run(t, "commit"); e != nil || !strings.Contains(out, "nothing to commit") {
		t.Fatalf("commit %q %v", out, e)
	}
	// Simulate another machine and an offline origin. Re-open must materialize from cache.
	if e := os.Chdir(tasks); e != nil {
		t.Fatal(e)
	}
	os.RemoveAll(root)
	os.RemoveAll(filepath.Join(homeDir, "clones", id+".git"))
	os.Remove(filepath.Join(homeDir, "local.yml"))
	if e := os.Rename(upstream, upstream+".offline"); e != nil {
		t.Fatal(e)
	}
	out, warn, e := run(t, "open", fields[0])
	if e != nil {
		t.Fatalf("offline open: %v (%s)", e, warn)
	}
	root = strings.TrimSpace(out)
	if got := gitRun(t, filepath.Join(root, "work"), "branch", "--show-current"); got != "topic" {
		t.Fatalf("work branch=%q", got)
	}
	if got := gitRun(t, filepath.Join(root, "upstream"), "branch", "--show-current"); got != "main" {
		t.Fatalf("follow branch=%q", got)
	}
	if got := gitRun(t, filepath.Join(root, "pin"), "branch", "--show-current"); got != "" {
		t.Fatalf("pin attached=%q", got)
	}
	for _, name := range []string{"upstream", "work", "pin"} {
		if got := gitRun(t, filepath.Join(root, name), "remote", "get-url", "origin"); got != upstream {
			t.Fatalf("%s origin=%q", name, got)
		}
	}
}
func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
