package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// M35: the import log entry carries the resolved author (--author →
// TASKTREE_AUTHOR → config), not just the configured one.
func TestImportLogEntryUsesResolvedAuthor(t *testing.T) {
	a, root := importFixture(t)
	a.AuthorOverride = "agent-x"
	r, err := a.Import(context.Background(), root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertImportAuthor(t, a, r.Task.ID, "agent-x")
}

func TestImportLogEntryUsesEnvAuthor(t *testing.T) {
	a, root := importFixture(t)
	t.Setenv("TASKTREE_AUTHOR", "env-agent")
	r, err := a.Import(context.Background(), root, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertImportAuthor(t, a, r.Task.ID, "env-agent")
}

func assertImportAuthor(t *testing.T, a App, id, want string) {
	t.Helper()
	got, err := a.Info(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Log) != 1 || got.Log[0].Author != want {
		t.Fatalf("log = %+v, want author %s", got.Log, want)
	}
}

// M26: the rewind refusal names the escape hatch that exists in its context.
func TestGitlinkRewindHintsAreContextSpecific(t *testing.T) {
	for _, closeTask := range []bool{false, true} {
		name := map[bool]string{false: "commit", true: "close"}[closeTask]
		t.Run(name, func(t *testing.T) {
			p, id, _, rb, _ := reviewSkippedLink(t, false)
			dirty := filepath.Join(rb, "repo", "dirty")
			writeSyncFile(t, dirty, "work")
			p.sync(p.b, SyncOptions{ID: id, Root: rb, NoRepos: true})
			os.Remove(dirty)
			var e error
			if closeTask {
				_, e = p.b.Mark(p.ctx, id, "done", MarkOptions{})
			} else {
				_, _, e = p.b.Commit(p.ctx, id, rb, "rewind")
			}
			if e == nil {
				t.Fatal("expected a rewind refusal")
			}
			msg := e.Error()
			if closeTask {
				if strings.Contains(msg, "--allow-rewind") || !strings.Contains(msg, "mark --force") || !strings.Contains(msg, "sync") {
					t.Fatalf("close hint: %s", msg)
				}
			} else if !strings.Contains(msg, "--allow-rewind") || strings.Contains(msg, "mark --force") {
				t.Fatalf("commit hint: %s", msg)
			}
		})
	}
}
