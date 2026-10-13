package app

import (
	"context"
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
