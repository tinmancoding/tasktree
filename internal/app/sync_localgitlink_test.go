package app

import (
	"path/filepath"
	"testing"
)

func TestSyncS6LocallyAvailableGitlinkDoesNotFetch(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Local gitlink")
	origin, seed := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "repo"}); e != nil {
		t.Fatal(e)
	}
	root := p.open(p.a, task.ID)
	repo := filepath.Join(root, "repo")
	target := syncRepoCommit(t, p.g, seed, "main", "file", "fetched target\n")
	runGitTest(t, p.g, repo, "fetch", "origin") // target is locally available, HEAD still old.
	runGitTest(t, p.g, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "offline"))
	report := newSyncReport()
	p.a.applyOneGitlink(p.ctx, module{Name: "repo", Branch: "main"}, repo, target, &report)
	if len(report.Failed) > 0 || len(report.Skipped) > 0 || runGitTest(t, p.g, repo, "rev-parse", "HEAD") != target {
		t.Fatalf("S6 contacted origin or failed local fast-forward: %+v", report)
	}
}
