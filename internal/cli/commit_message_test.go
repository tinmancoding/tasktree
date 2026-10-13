package cli_test

import (
	"os"
	"strings"
	"testing"
)

func TestM45CommitMessage(t *testing.T) {
	f := newCloseFixture(t)
	handle, id, root := createAndOpen(t, "Commit msg", f.tasks+"/cm")
	hidden := f.home + "/clones/" + id + ".git"
	if err := os.WriteFile(root+"/a.txt", []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "commit", "-m", "one", "-t", handle)
	short := strings.TrimSpace(gitRun(t, hidden, "rev-parse", "--short", "HEAD"))
	if err != nil || strings.TrimSpace(out) != "committed "+short+" and pushed to the local hub" {
		t.Fatalf("local-only message %q %v", out, err)
	}
	remote := t.TempDir()
	gitRun(t, remote, "init", "--bare", "-b", "main")
	if _, _, err = run(t, "config", "set", "remote", remote); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(root+"/b.txt", []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "commit", "-m", "two", "-t", handle)
	short = strings.TrimSpace(gitRun(t, hidden, "rev-parse", "--short", "HEAD"))
	if err != nil || strings.TrimSpace(out) != "committed "+short+" and pushed to the local hub; run tasktree sync to publish" {
		t.Fatalf("remote message %q %v", out, err)
	}
}
