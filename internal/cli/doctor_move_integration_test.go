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

func TestDoctorHandlesDeletedCheckoutModuleLoss(t *testing.T) {
	f := newCloseFixture(t)
	unsafeHandle, unsafeID, unsafeRoot := createAndOpen(t, "Deleted checkout with work", filepath.Join(f.tasks, "unsafe-deleted"))
	if err := os.Chdir(unsafeRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "api", "--branch", "fd"); err != nil {
		t.Fatalf("add repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unsafeRoot, "api", "lost"), []byte("local work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, filepath.Join(unsafeRoot, "api"), "add", "lost")
	gitRun(t, filepath.Join(unsafeRoot, "api"), "commit", "-m", "local only")
	if err := os.Chdir(f.base); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(unsafeRoot); err != nil {
		t.Fatal(err)
	}
	unsafeClone := filepath.Join(f.home, "clones", unsafeID+".git")

	out, _, err := run(t, "doctor", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor code=%d err=%v output=%s", cli.Code(err), err, out)
	}
	var result struct {
		Findings []struct {
			Code, Subject, Message string
			Fixable                bool
		}
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	var unsafeFinding *struct {
		Code, Subject, Message string
		Fixable                bool
	}
	for i := range result.Findings {
		finding := &result.Findings[i]
		if finding.Code == "check-failed" {
			t.Fatalf("doctor failed its module loss check: %s", out)
		}
		if finding.Code == "checkout-missing" && finding.Subject == unsafeID {
			unsafeFinding = finding
		}
	}
	if unsafeFinding == nil || unsafeFinding.Fixable || !strings.Contains(unsafeFinding.Message, "api has unpushed commits") {
		t.Fatalf("unsafe deleted checkout finding: %s", out)
	}

	out, _, err = run(t, "doctor", "--fix", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor --fix code=%d err=%v output=%s", cli.Code(err), err, out)
	}
	if _, err := os.Stat(unsafeClone); err != nil {
		t.Fatalf("doctor --fix removed clone with unpushed work: %v", err)
	}
	if local, err := os.ReadFile(filepath.Join(f.home, "local.yml")); err != nil || !strings.Contains(string(local), unsafeID) {
		t.Fatalf("doctor --fix removed local checkout with unpushed work: %q %v", local, err)
	}

	_, forceErr, err := run(t, "open", unsafeHandle, "--force")
	if err != nil || !strings.Contains(forceErr, "api has unpushed commits") {
		t.Fatalf("open --force did not report lost module commit: err=%v stderr=%q", err, forceErr)
	}

	_, cleanID, cleanRoot := createAndOpen(t, "Deleted checkout without work", filepath.Join(f.tasks, "clean-deleted"))
	if err := os.RemoveAll(cleanRoot); err != nil {
		t.Fatal(err)
	}
	cleanClone := filepath.Join(f.home, "clones", cleanID+".git")
	out, _, err = run(t, "doctor", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor code=%d err=%v output=%s", cli.Code(err), err, out)
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	fixableClean := false
	for _, finding := range result.Findings {
		if finding.Code == "checkout-missing" && finding.Subject == cleanID && finding.Fixable {
			fixableClean = true
		}
	}
	if !fixableClean {
		t.Fatalf("clean deleted checkout was not fixable: %s", out)
	}
	if _, _, err = run(t, "doctor", "--fix", "--json"); err != nil {
		t.Fatalf("doctor --fix clean checkout: %v", err)
	}
	if _, err := os.Stat(cleanClone); !os.IsNotExist(err) {
		t.Fatalf("doctor --fix left clean clone: %v", err)
	}
	if local, err := os.ReadFile(filepath.Join(f.home, "local.yml")); err != nil || strings.Contains(string(local), cleanID) {
		t.Fatalf("doctor --fix left clean local checkout: %q %v", local, err)
	}
}

func TestDoctorContinuesAfterCheckFailure(t *testing.T) {
	f := newCloseFixture(t)
	_, id, root := createAndOpen(t, "Broken module check", filepath.Join(f.tasks, "broken-module"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "api", "--branch", "broken"); err != nil {
		t.Fatalf("add repository: %v", err)
	}
	if err := os.Chdir(f.base); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(f.home, "clones", id+".git")
	if err := os.WriteFile(filepath.Join(clone, "modules", "api", "HEAD"), []byte("not-a-ref\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.home, "imports", "unfinished"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, _, err := run(t, "doctor", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor code=%d err=%v output=%s", cli.Code(err), err, out)
	}
	var result struct {
		Findings []struct{ Code string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	checkFailed, importJournal := false, false
	for _, finding := range result.Findings {
		checkFailed = checkFailed || finding.Code == "check-failed"
		importJournal = importJournal || finding.Code == "import-journal"
	}
	if !checkFailed || !importJournal {
		t.Fatalf("doctor did not continue after failed check: %s", out)
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

// H3: a hand-moved checkout must be re-pointed, never treated as "no work at risk".
func TestDoctorFixRepointsHandMovedCheckout(t *testing.T) {
	f := newCloseFixture(t)
	old := filepath.Join(f.tasks, "old")
	handle, id, _ := createAndOpen(t, "Moved by hand", old)
	if _, _, err := run(t, "add", f.upstream, "--name", "repo", "-t", handle); err != nil {
		t.Fatalf("add repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(old, "repo", "dirty.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(f.tasks, "group", "moved")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "doctor", "--json")
	if cli.Code(err) != 1 {
		t.Fatalf("doctor code=%d out=%s", cli.Code(err), out)
	}
	if strings.Contains(out, "no work at risk") {
		t.Fatalf("doctor claims no work at risk for a moved checkout: %s", out)
	}
	if _, _, err = run(t, "doctor", "--fix", "--json"); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.home, "clones", id+".git", "modules", "repo")); err != nil {
		t.Fatalf("doctor --fix deleted the hidden git directory: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(moved, "repo", "dirty.txt")); err != nil || string(b) != "keep me\n" {
		t.Fatalf("dirty file: %q %v", b, err)
	}
	if status := gitRun(t, filepath.Join(moved, "repo"), "status", "--porcelain"); !strings.Contains(status, "dirty.txt") {
		t.Fatalf("repository does not see the dirty file: %q", status)
	}
	local, err := os.ReadFile(filepath.Join(f.home, "local.yml"))
	if err != nil || !strings.Contains(string(local), moved) {
		t.Fatalf("local.yml not re-pointed: %q %v", local, err)
	}
	if _, _, err = run(t, "info", "-t", handle, "--json"); err != nil {
		t.Fatalf("info after doctor --fix: %v", err)
	}
	out, _, err = run(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor still unhappy: %v %s", err, out)
	}
}
