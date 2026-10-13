package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
)

func TestSyncS1ConfigurePushesEveryDefinition(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	a := newSyncMachine(t, "A", "")
	ctx := context.Background()
	g := gitx.NewClient()
	for _, title := range []string{"One", "Two"} {
		task, e := a.NewTask(ctx, NewOptions{Title: title})
		if e != nil {
			t.Fatal(e)
		}
		repo, _ := syncRepo(t, g)
		if _, e = a.AddDefined(ctx, task.ID, AddOptions{URL: repo}); e != nil {
			t.Fatal(e)
		}
	}
	remote := filepath.Join(t.TempDir(), "hub.git")
	runGitTest(t, g, "", "init", "--bare", "-b", "main", remote)
	if e := a.ConfigSetContext(ctx, "remote", remote); e != nil {
		t.Fatal(e)
	}
	local := runGitTest(t, g, a.Home.GitPath(), "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/")
	pushed := runGitTest(t, g, remote, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/")
	if local != pushed {
		t.Fatalf("config set failed to push all refs:\n%s\n%s", local, pushed)
	}
	b := newSyncMachine(t, "B", remote)
	tasks, e := b.List(true, nil)
	if e != nil || len(tasks) != 2 {
		t.Fatalf("join=%+v err=%v", tasks, e)
	}
}

func TestSyncS5LeaseExhausted(t *testing.T) {
	p := newSyncPair(t)
	p.task(p.a, "Unpushed")
	// All push rejections are retried; no rejected server ref can be overwritten.
	hook := filepath.Join(p.remote, "hooks", "pre-receive")
	writeSyncFile(t, hook, "#!/bin/sh\necho test-rejection >&2\nexit 1\n")
	if e := os.Chmod(hook, 0755); e != nil {
		t.Fatal(e)
	}
	remoteBefore := runGitTest(t, p.g, p.remote, "rev-parse", "main")
	r, e := p.a.Sync(p.ctx, SyncOptions{All: true})
	requireSyncConflict(t, e)
	if strings.Count(strings.Join(r.Steps, "\n"), "push lease rejected") != 3 {
		t.Fatalf("retry count=%+v", r)
	}
	if runGitTest(t, p.g, p.remote, "rev-parse", "main") != remoteBefore {
		t.Fatal("rejected push moved remote")
	}
}

func TestSyncS4NormalGitMergeAndDeletion(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Delete vs modify")
	hubSyncCommit(t, p.a.App, "text", "one\ntwo\nthree\nfour\nfive\n")
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	hubSyncCommit(t, p.a.App, "text", "ONE\ntwo\nthree\nfour\nfive\n")
	hubSyncCommit(t, p.b.App, "text", "one\ntwo\nthree\nfour\nFIVE\n")
	// Remote deletes a record while local changes it: keep the modification.
	runGitTest(t, p.g, p.b.Home.GitPath(), "rm", "tasks/"+task.ID+".md")
	if _, _, e := p.b.Git.Commit(p.ctx, p.b.Home.GitPath(), nil, "B", "B", "delete record"); e != nil {
		t.Fatal(e)
	}
	if _, _, e := p.a.SetInfo(p.ctx, task.ID, "title", "Modified", false); e != nil {
		t.Fatal(e)
	}
	p.sync(p.b, SyncOptions{All: true})
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	want := "ONE\ntwo\nthree\nfour\nFIVE\n"
	if string(readSyncFile(t, filepath.Join(p.a.Home.GitPath(), "text"))) != want {
		t.Fatal("normal non-task git merge failed")
	}
	if v, e := p.b.Info(task.ID); e != nil || v.Title != "Modified" {
		t.Fatalf("delete/modify=%+v err=%v", v, e)
	}
}

func TestSyncS2DirtyRootAndS3ClosedReport(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Root gate")
	root := p.open(p.a, task.ID)
	writeSyncFile(t, filepath.Join(root, "uncommitted"), "work\n")
	before := p.a.Hub.Ref(p.ctx, "task/"+task.ID)
	r, e := p.a.Sync(p.ctx, SyncOptions{ID: task.ID, Root: root})
	requireSyncConflict(t, e)
	if len(r.Dirty) != 1 || !strings.Contains(e.Error(), "tt2 commit") {
		t.Fatalf("dirty gate=%+v %v", r, e)
	}
	if p.a.Hub.Ref(p.ctx, "task/"+task.ID) != before {
		t.Fatal("dirty root sync moved task ref")
	}
	if _, _, e = p.a.Commit(p.ctx, task.ID, root, "save user changes"); e != nil {
		t.Fatal(e)
	}
	if _, e = p.a.Mark(p.ctx, task.ID, "done", MarkOptions{Keep: true}); e != nil {
		t.Fatal(e)
	}
	r = p.sync(p.a, SyncOptions{All: true})
	if len(r.ClosedTaskCheckouts) != 1 {
		t.Fatalf("closed report=%+v", r)
	}
}

func TestSyncS6UnpushedAndOtherBranchGitlinks(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Unpushed gitlinks")
	origin, _ := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "repo"}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	rootA := p.open(p.a, task.ID)
	rootB := p.open(p.b, task.ID)
	repoA := filepath.Join(rootA, "repo")
	writeSyncFile(t, filepath.Join(repoA, "file"), "not pushed\n")
	runGitTest(t, p.g, repoA, "add", ".")
	if _, _, e := p.g.Commit(p.ctx, repoA, nil, "A", "A", "local repo commit"); e != nil {
		t.Fatal(e)
	}
	unpushed := runGitTest(t, p.g, repoA, "rev-parse", "HEAD")
	if _, _, e := p.a.Commit(p.ctx, task.ID, rootA, "record local repo commit"); e != nil {
		t.Fatal(e)
	}
	report := p.sync(p.a, SyncOptions{ID: task.ID, Root: rootA, NoRepos: true})
	if len(report.UnpushedGitlinks) == 0 {
		t.Fatal("unchanged unpushed gitlink not reported")
	}
	oldB := runGitTest(t, p.g, filepath.Join(rootB, "repo"), "rev-parse", "HEAD")
	report = p.sync(p.b, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
	if len(report.UnpushedGitlinks) == 0 || runGitTest(t, p.g, filepath.Join(rootB, "repo"), "rev-parse", "HEAD") != oldB {
		t.Fatalf("unreachable gitlink applied: %+v", report)
	}
	// Direct S6 applies allow focused testing of clean but non-tracking branches.
	runGitTest(t, p.g, filepath.Join(rootB, "repo"), "checkout", "-b", "other")
	r := newSyncReport()
	p.b.applyOneGitlink(p.ctx, module{Name: "repo", Branch: "main"}, filepath.Join(rootB, "repo"), unpushed, &r)
	if len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0], "other branch") {
		t.Fatalf("other branch report=%+v", r)
	}
}

func TestSyncS8SelectedPathsPreserveCommonMerge(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Selected paths")
	rootA := p.open(p.a, task.ID)
	p.sync(p.a, SyncOptions{All: true})
	rootB := p.open(p.b, task.ID)
	writeSyncFile(t, filepath.Join(rootA, "conflict"), "A\n")
	writeSyncFile(t, filepath.Join(rootA, "only-A"), "A file\n")
	if _, _, e := p.a.Commit(p.ctx, task.ID, rootA, "A files"); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	writeSyncFile(t, filepath.Join(rootB, "conflict"), "B\n")
	writeSyncFile(t, filepath.Join(rootB, "only-B"), "B file\n")
	if _, _, e := p.b.Commit(p.ctx, task.ID, rootB, "B files"); e != nil {
		t.Fatal(e)
	}
	_, e := p.b.Resolve(p.ctx, task.ID, rootB, "remote", []string{"only-A"})
	var validation ValidationError
	if !errors.As(e, &validation) {
		t.Fatalf("expected usage error for one-sided selection, got %v", e)
	}
	result, e := p.b.Resolve(p.ctx, task.ID, rootB, "remote", []string{"conflict"})
	if e != nil {
		t.Fatalf("selected resolve: %v %+v", e, result)
	}
	for path, want := range map[string]string{"conflict": "A\n", "only-A": "A file\n", "only-B": "B file\n"} {
		if string(readSyncFile(t, filepath.Join(rootB, path))) != want {
			t.Fatal("common merge lost path", path)
		}
	}
}

func TestSyncS7NonFastForwardAndOwnBranchOnly(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Branch modes")
	origin, seed := syncRepo(t, p.g)
	runGitTest(t, p.g, seed, "checkout", "-b", "own")
	runGitTest(t, p.g, seed, "push", "origin", "own")
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "work", Branch: "own", Mode: "work"}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "follow"}); e != nil {
		t.Fatal(e)
	}
	root := p.open(p.a, task.ID)
	workOld := runGitTest(t, p.g, filepath.Join(root, "work"), "rev-parse", "HEAD")
	syncRepoCommit(t, p.g, seed, "main", "file", "upstream only\n")
	// Follow has a local divergent commit; it must not be reset or merged.
	follow := filepath.Join(root, "follow")
	writeSyncFile(t, filepath.Join(follow, "local"), "local")
	runGitTest(t, p.g, follow, "add", ".")
	if _, _, e := p.g.Commit(p.ctx, follow, nil, "A", "A", "local follow commit"); e != nil {
		t.Fatal(e)
	}
	followOld := runGitTest(t, p.g, follow, "rev-parse", "HEAD")
	if _, _, e := p.a.Commit(p.ctx, task.ID, root, "local gitlink"); e != nil {
		t.Fatal(e)
	}
	r := p.sync(p.a, SyncOptions{ID: task.ID, Root: root})
	if runGitTest(t, p.g, filepath.Join(root, "work"), "rev-parse", "HEAD") != workOld {
		t.Fatal("work merged origin default branch")
	}
	if runGitTest(t, p.g, follow, "rev-parse", "HEAD") != followOld || !strings.Contains(strings.Join(r.Skipped, "\n"), "non-fast-forward") {
		t.Fatalf("follow reset or missing report: %+v", r)
	}
}
