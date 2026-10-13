package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveS8RemoteGitlinkNeverRewindsCleanRepoBranch(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Resolve gitlink")
	origin, seed := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "repo"}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	rootA := p.open(p.a, task.ID)
	rootB := p.open(p.b, task.ID)
	chosen := syncRepoCommit(t, p.g, seed, "main", "file", "A gitlink\n")
	repoA := filepath.Join(rootA, "repo")
	runGitTest(t, p.g, repoA, "fetch", "origin")
	runGitTest(t, p.g, repoA, "merge", "--ff-only", chosen)
	if _, _, e := p.a.Commit(p.ctx, task.ID, rootA, "A gitlink"); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	tip := syncRepoCommit(t, p.g, seed, "main", "file", "B later gitlink\n")
	repoB := filepath.Join(rootB, "repo")
	runGitTest(t, p.g, repoB, "fetch", "origin")
	runGitTest(t, p.g, repoB, "merge", "--ff-only", tip)
	if _, _, e := p.b.Commit(p.ctx, task.ID, rootB, "B gitlink"); e != nil {
		t.Fatal(e)
	}
	comparison, e := p.b.Resolve(p.ctx, task.ID, rootB, "", nil)
	requireSyncConflict(t, e)
	if len(comparison.Paths) != 1 || comparison.Paths[0] != "repo" {
		t.Fatalf("comparison must list gitlinks: %+v", comparison)
	}
	result, e := p.b.Resolve(p.ctx, task.ID, rootB, "remote", nil)
	if e != nil {
		t.Fatalf("resolve: %v %+v", e, result)
	}
	if runGitTest(t, p.g, repoB, "rev-parse", "HEAD") != tip {
		t.Fatal("resolve rewound clean repo branch")
	}
	recorded := runGitTest(t, p.g, p.b.Home.GitPath(), "rev-parse", result.Commit+":repo")
	if recorded != chosen {
		t.Fatalf("root gitlink=%s want=%s", recorded, chosen)
	}
	skipped := strings.Join(result.Report.Skipped, "\n")
	for _, text := range []string{"branch tip " + tip, "chosen gitlink " + chosen, "reconcile inside the repo", "tt2 commit"} {
		if !strings.Contains(skipped, text) {
			t.Fatalf("missing mismatch guidance %q: %s", text, skipped)
		}
	}
	status, e := p.b.InspectTasktree(p.ctx, task.ID, rootB)
	if e != nil {
		t.Fatal(e)
	}
	if status.RootDirty != 1 {
		t.Fatalf("root must show the retained gitlink tip as modified: %+v", status)
	}
	// The S6 path must also report a plain already-ahead branch, not claim that
	// git merge --ff-only's successful no-op applied the older target.
	report := newSyncReport()
	p.b.applyOneGitlink(p.ctx, module{Name: "repo", Branch: "main"}, repoB, chosen, &report)
	if len(report.Skipped) != 1 || len(report.Steps) != 0 {
		t.Fatalf("already-ahead S6 result=%+v", report)
	}
}
