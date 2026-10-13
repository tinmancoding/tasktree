package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func TestDoctorHealthyAndMalformedRecord(t *testing.T) {
	f := newCloseFixture(t)
	out, _, err := run(t, "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Findings []struct {
			Code, Severity, Subject, Message string
			Fixable, Fixed                   bool
		}
		Errors, Warnings int
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Errors != 0 || got.Warnings != 1 || len(got.Findings) != 1 || got.Findings[0].Code != "no-remote" {
		t.Fatalf("healthy doctor: %s", out)
	}
	bad := filepath.Join(f.home, "git", "tasks", "broken.md")
	if err := os.WriteFile(bad, []byte("not a task record\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "doctor", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor code=%d err=%v", cli.Code(err), err)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Errors != 2 {
		t.Fatalf("malformed record also makes the hub dirty: %s", out)
	}
	found := false
	for _, finding := range got.Findings {
		if finding.Code == "record-unparseable" && !finding.Fixable && strings.Contains(finding.Subject, "broken.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing malformed finding: %s", out)
	}
	if _, _, err = run(t, "doctor", "--fix", "--json"); cli.Code(err) != 1 {
		t.Fatalf("unsafe finding was unexpectedly fixed: %v", err)
	}
	if _, statErr := os.Stat(bad); statErr != nil {
		t.Fatalf("doctor removed malformed record: %v", statErr)
	}
}

func TestMoveSuccessAndInjectedRollback(t *testing.T) {
	f := newCloseFixture(t)
	old := filepath.Join(f.tasks, "old")
	handle, _, _ := createAndOpen(t, "Move me", old)
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "-t", handle); err != nil {
		t.Fatalf("add repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(old, "repo", "dirty.txt"), []byte("preserved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeHead := gitRun(t, filepath.Join(old, "repo"), "rev-parse", "HEAD")
	newPath := filepath.Join(f.tasks, "new")
	out, hint, err := run(t, "move", newPath, "-t", handle)
	if err != nil {
		t.Fatalf("move: %v (%s)", err, hint)
	}
	if strings.TrimSpace(out) != newPath || !strings.Contains(hint, "$PWD") {
		t.Fatalf("output=%q hint=%q", out, hint)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old checkout remains: %v", err)
	}
	if _, _, err = run(t, "info", "-t", handle, "--json"); err != nil {
		t.Fatalf("info after move: %v", err)
	}
	if got := gitRun(t, filepath.Join(newPath, "repo"), "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("module HEAD changed: %s != %s", got, beforeHead)
	}
	if got, readErr := os.ReadFile(filepath.Join(newPath, "repo", "dirty.txt")); readErr != nil || string(got) != "preserved\n" {
		t.Fatalf("dirty file not preserved: %q %v", got, readErr)
	}

	rollbackTarget := filepath.Join(f.tasks, "rollback-target")
	t.Setenv("TASKTREE_TEST_MOVE_FAIL", "1")
	_, _, err = run(t, "move", rollbackTarget, "-t", handle)
	if err == nil {
		t.Fatal("injected failure succeeded")
	}
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Fatalf("source not restored: %v", statErr)
	}
	if _, statErr := os.Stat(rollbackTarget); !os.IsNotExist(statErr) {
		t.Fatalf("target remains: %v", statErr)
	}
	if _, _, infoErr := run(t, "info", "-t", handle, "--json"); infoErr != nil {
		t.Fatalf("info after rollback: %v", infoErr)
	}
	t.Setenv("TASKTREE_TEST_MOVE_FAIL", "")
	gitRun(t, filepath.Join(newPath, "repo"), "add", "dirty.txt")
	gitRun(t, filepath.Join(newPath, "repo"), "commit", "-m", "preserve dirty file")
	gitRun(t, filepath.Join(newPath, "repo"), "push", "origin", "HEAD:main")
	if _, _, commitErr := run(t, "commit", "-t", handle, "-m", "preserve moved work"); commitErr != nil {
		t.Fatalf("commit after move: %v", commitErr)
	}
	if _, _, markErr := run(t, "mark", "done", "-t", handle); markErr != nil {
		t.Fatalf("mark done after move: %v", markErr)
	}
}
