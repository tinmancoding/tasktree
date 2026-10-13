package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type syncMachine struct {
	App
	warnings *bytes.Buffer
}
type syncPair struct {
	a, b   syncMachine
	remote string
	g      gitx.Client
	ctx    context.Context
	t      *testing.T
}

func newSyncMachine(t *testing.T, name, remote string) syncMachine {
	t.Helper()
	h := home.Home{Root: filepath.Join(t.TempDir(), name)}
	a := New(h, nil)
	w := &bytes.Buffer{}
	a.WarningWriter = w
	a.Hub.WarningWriter = w
	activateSyncMachine(t, a)
	_, e := a.Setup(context.Background(), SetupOptions{Author: name, AuthorSet: true, Machine: name, MachineSet: true, TasksRoot: filepath.Join(t.TempDir(), "tasks-"+name), TasksRootSet: true, Remote: remote, RemoteSet: remote != ""})
	if e != nil {
		t.Fatal(e)
	}
	return syncMachine{a, w}
}
func activateSyncMachine(t *testing.T, a App) {
	t.Helper()
	t.Setenv("TASKTREE_HOME", a.Home.Root)
	t.Setenv("TASKTREE_CACHE_DIR", filepath.Join(a.Home.Root, "cache"))
}
func newSyncPair(t *testing.T) syncPair {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("TASKTREE_AUTHOR", "")
	g := gitx.NewClient()
	remote := filepath.Join(t.TempDir(), "hub.git")
	runGitTest(t, g, "", "init", "--bare", "-b", "main", remote)
	a := newSyncMachine(t, "A", "")
	if e := a.ConfigSetContext(context.Background(), "remote", remote); e != nil {
		t.Fatal(e)
	}
	b := newSyncMachine(t, "B", remote)
	return syncPair{a, b, remote, g, context.Background(), t}
}
func (p syncPair) task(a syncMachine, title string) record.Task {
	p.t.Helper()
	activateSyncMachine(p.t, a.App)
	v, e := a.NewTask(p.ctx, NewOptions{Title: title})
	if e != nil {
		p.t.Fatal(e)
	}
	return v
}
func (p syncPair) sync(a syncMachine, o SyncOptions) SyncReport {
	p.t.Helper()
	activateSyncMachine(p.t, a.App)
	r, e := a.Sync(p.ctx, o)
	if e != nil {
		p.t.Fatalf("sync %s: %v\n%+v", a.Home.Root, e, r)
	}
	return r
}
func (p syncPair) open(a syncMachine, id string) string {
	p.t.Helper()
	activateSyncMachine(p.t, a.App)
	root, e := a.Open(p.ctx, id, OpenOptions{SkipBootstrap: true}, a.warnings)
	if e != nil {
		p.t.Fatal(e)
	}
	return root
}
func writeSyncFile(t *testing.T, path, text string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(text), 0644); e != nil {
		t.Fatal(e)
	}
}
func readSyncFile(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func requireSyncConflict(t *testing.T, e error) {
	t.Helper()
	var conflict hub.ConflictError
	if !errors.As(e, &conflict) {
		t.Fatalf("expected conflict (exit 5), got %v", e)
	}
}
func hubSyncCommit(t *testing.T, a App, path, text string) {
	t.Helper()
	writeSyncFile(t, filepath.Join(a.Home.GitPath(), path), text)
	runGitTest(t, a.Git, a.Home.GitPath(), "add", path)
	cfg, _ := a.Home.LoadConfig()
	if _, _, e := a.Git.Commit(context.Background(), a.Home.GitPath(), nil, cfg.Author, cfg.Machine, "test "+path); e != nil {
		t.Fatal(e)
	}
}

func TestSyncS1JoinAndUnrelatedS9(t *testing.T) {
	p := newSyncPair(t)
	first := p.task(p.a, "From A")
	p.sync(p.a, SyncOptions{All: true})
	// setup --remote clones all records and every definition branch.
	rootA := p.open(p.a, first.ID)
	_ = rootA
	p.sync(p.a, SyncOptions{All: true})
	c := newSyncMachine(t, "C", p.remote)
	if _, e := c.Info(first.ID); e != nil {
		t.Fatal(e)
	}
	if !c.DefinitionExists(p.ctx, first.ID) {
		t.Fatal("setup clone omitted task branch")
	}
	if branch := runGitTest(t, p.g, c.Home.GitPath(), "branch", "--show-current"); branch != "main" {
		t.Fatalf("hub branch=%s", branch)
	}
	fetch := runGitTest(t, p.g, c.Home.GitPath(), "config", "--get-all", "remote.origin.fetch")
	if fetch != "+refs/heads/*:refs/remotes/origin/*" {
		t.Fatalf("fetch=%s", fetch)
	}
	d := newSyncMachine(t, "D", "")
	own := p.task(d, "From D")
	// Existing hub setup configures only: no history rewrite or push.
	before := d.Hub.Ref(p.ctx, "main")
	if _, e := d.Setup(p.ctx, SetupOptions{Remote: p.remote, RemoteSet: true}); e != nil {
		t.Fatal(e)
	}
	if d.Hub.Ref(p.ctx, "main") != before {
		t.Fatal("setup merged existing hub")
	}
	_, e := d.Sync(p.ctx, SyncOptions{All: true})
	requireSyncConflict(t, e)
	p.sync(d, SyncOptions{All: true, AllowUnrelated: true})
	p.sync(p.a, SyncOptions{All: true})
	for _, a := range []syncMachine{d, p.a} {
		for _, id := range []string{first.ID, own.ID} {
			if _, e := a.Info(id); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := d.ConfigSetContext(p.ctx, "remote", ""); e != nil {
		t.Fatal(e)
	}
	if _, _, e := p.g.Run(p.ctx, d.Home.GitPath(), nil, "remote", "get-url", "origin"); e == nil {
		t.Fatal("origin not removed")
	}
}

func TestSyncS4RecordMatrixTwoMachines(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Shared")
	p.a.Labels(p.ctx, task.ID, "add", []string{"base", "remove"})
	p.a.Ref(p.ctx, task.ID, "add", "url", "remove")
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	// Insert notes on both machines; the local note's exact chunk must survive.
	p.a.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC) }
	p.b.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC) }
	if _, e := p.a.Note(p.ctx, task.ID, "A note  \nexact\tbytes", "note"); e != nil {
		t.Fatal(e)
	}
	local, _ := p.a.Hub.Read(task.ID)
	entry := local.Task.Log[0]
	header := "### " + entry.Timestamp.Format(time.RFC3339) + " note A@A {#e-" + entry.ID + "}\n" + entry.Body + "\n"
	if _, e := p.b.Note(p.ctx, task.ID, "B note", "note"); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct {
		a      syncMachine
		action string
		labels []string
	}{{p.a, "rm", []string{"remove"}}, {p.a, "add", []string{"a"}}, {p.b, "rm", []string{"base"}}, {p.b, "add", []string{"b"}}} {
		if _, _, e := change.a.Labels(p.ctx, task.ID, change.action, change.labels); e != nil {
			t.Fatal(e)
		}
	}
	p.a.Ref(p.ctx, task.ID, "rm", "url", "remove")
	p.a.Ref(p.ctx, task.ID, "add", "url", "local")
	p.b.Ref(p.ctx, task.ID, "add", "jira", "REMOTE")
	if _, e := p.b.Mark(p.ctx, task.ID, "next", MarkOptions{Keep: true}); e != nil {
		t.Fatal(e)
	}
	newA := p.task(p.a, "New A")
	newB := p.task(p.b, "New B")
	p.sync(p.b, SyncOptions{All: true})
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	p.sync(p.a, SyncOptions{All: true})
	path := "tasks/" + task.ID + ".md"
	aBytes := readSyncFile(t, filepath.Join(p.a.Home.GitPath(), path))
	bBytes := readSyncFile(t, filepath.Join(p.b.Home.GitPath(), path))
	if !bytes.Equal(aBytes, bBytes) || !bytes.Contains(aBytes, []byte(header)) {
		t.Fatalf("convergence/losslessness failed\nA=%s\nB=%s", aBytes, bBytes)
	}
	v, e := record.Parse(aBytes)
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Task.Log) != 2 || v.Task.Status != "next" || strings.Join(v.Task.Labels, ",") != "a,b" || len(v.Task.Refs) != 2 {
		t.Fatalf("matrix=%+v", v.Task)
	}
	for _, id := range []string{newA.ID, newB.ID} {
		if _, e := p.a.Info(id); e != nil {
			t.Fatal(e)
		}
		if _, e := p.b.Info(id); e != nil {
			t.Fatal(e)
		}
	}
	// Concurrent scalars and descriptions retain A's side and audit B's text.
	p.a.SetInfo(p.ctx, task.ID, "status", "active", false)
	p.b.SetInfo(p.ctx, task.ID, "status", "blocked", false)
	p.a.EditDescription(p.ctx, task.ID, "local description")
	p.b.EditDescription(p.ctx, task.ID, "remote description\n```\nwith fence")
	p.sync(p.b, SyncOptions{All: true})
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	v, e = p.a.Hub.Read(task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Task.Status != "active" || v.Task.Description != "local description" || len(v.Task.Conflict) != 2 || len(v.Task.Log) != 4 {
		t.Fatalf("conflict=%+v", v.Task)
	}
	if !strings.Contains(string(v.Render()), "remote value (commit ") || !strings.Contains(string(v.Render()), " by B at ") || !strings.Contains(string(v.Render()), ") was: blocked") {
		t.Fatalf("missing remote provenance: %s", v.Render())
	}
	doctor, e := p.a.Doctor(p.ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range doctor.Findings {
		if f.Code == "conflict-flag" {
			found = true
		}
	}
	if !found {
		t.Fatal("doctor omitted conflict flag")
	}
	if _, changed, e := p.a.EditDescription(p.ctx, task.ID, "local description"); e != nil || !changed {
		t.Fatalf("description ack: %v", e)
	}
	if _, changed, e := p.a.SetInfo(p.ctx, task.ID, "status", "active", false); e != nil || !changed {
		t.Fatalf("status ack: %v", e)
	}
	v, _ = p.a.Hub.Read(task.ID)
	if len(v.Task.Conflict) != 0 || bytes.Contains(v.Render(), []byte("conflict:")) {
		t.Fatal("conflicts not cleared")
	}
}

func TestSyncS4NonTaskConflictIsAtomic(t *testing.T) {
	p := newSyncPair(t)
	hubSyncCommit(t, p.a.App, "settings", "base\n")
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	hubSyncCommit(t, p.a.App, "settings", "local\n")
	hubSyncCommit(t, p.b.App, "settings", "remote\n")
	p.sync(p.b, SyncOptions{All: true})
	before := p.a.Hub.Ref(p.ctx, "main")
	work := readSyncFile(t, filepath.Join(p.a.Home.GitPath(), "settings"))
	_, e := p.a.Sync(p.ctx, SyncOptions{All: true})
	requireSyncConflict(t, e)
	if p.a.Hub.Ref(p.ctx, "main") != before || !bytes.Equal(work, readSyncFile(t, filepath.Join(p.a.Home.GitPath(), "settings"))) {
		t.Fatal("conflict changed main or worktree")
	}
	if status := runGitTest(t, p.g, p.a.Home.GitPath(), "status", "--porcelain"); status != "" {
		t.Fatalf("dirty hub: %s", status)
	}
}

func TestSyncS5LeaseRetry(t *testing.T) {
	p := newSyncPair(t)
	aTask := p.task(p.a, "A concurrent")
	bTask := p.task(p.b, "B concurrent")
	p.a.syncBeforePush = func(ctx context.Context) error {
		_, _, e := p.g.Run(ctx, p.b.Home.GitPath(), nil, "push", "origin", "refs/heads/main:refs/heads/main")
		return e
	}
	r := p.sync(p.a, SyncOptions{All: true})
	if !strings.Contains(strings.Join(r.Steps, "\n"), "push lease rejected; retry 1/3") {
		t.Fatalf("no retry: %+v", r)
	}
	p.a.syncBeforePush = nil
	p.sync(p.b, SyncOptions{All: true})
	for _, id := range []string{aTask.ID, bTask.ID} {
		if _, e := p.a.Info(id); e != nil {
			t.Fatal(e)
		}
		if _, e := p.b.Info(id); e != nil {
			t.Fatal(e)
		}
	}
}

// A local bare repository and a seed checkout stand in for each repo origin.
func syncRepo(t *testing.T, g gitx.Client) (string, string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "repo.git")
	seed := filepath.Join(t.TempDir(), "seed")
	runGitTest(t, g, "", "init", "--bare", "-b", "main", remote)
	runGitTest(t, g, "", "init", "-b", "main", seed)
	runGitTest(t, g, seed, "config", "user.name", "test")
	runGitTest(t, g, seed, "config", "user.email", "test@local")
	writeSyncFile(t, filepath.Join(seed, "file"), "base\n")
	runGitTest(t, g, seed, "add", ".")
	runGitTest(t, g, seed, "commit", "-m", "base")
	runGitTest(t, g, seed, "remote", "add", "origin", remote)
	runGitTest(t, g, seed, "push", "-u", "origin", "main")
	return remote, seed
}
func syncRepoCommit(t *testing.T, g gitx.Client, seed, branch, file, text string) string {
	t.Helper()
	runGitTest(t, g, seed, "checkout", branch)
	writeSyncFile(t, filepath.Join(seed, file), text)
	runGitTest(t, g, seed, "add", file)
	runGitTest(t, g, seed, "commit", "-m", text)
	runGitTest(t, g, seed, "push", "origin", branch)
	return runGitTest(t, g, seed, "rev-parse", "HEAD")
}

func TestSyncS2S3S6AndOpenS53Definitions(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Definitions")
	repo, seed := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: repo, Name: "first"}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	definition, e := p.b.Definition(p.ctx, task.ID)
	if e != nil || len(definition.Repos) != 1 {
		t.Fatalf("definition=%+v err=%v", definition, e)
	}
	rootB := p.open(p.b, task.ID)
	rootA := p.open(p.a, task.ID)
	newSHA := syncRepoCommit(t, p.g, seed, "main", "file", "new upstream\n")
	p.sync(p.a, SyncOptions{ID: task.ID, Root: rootA}) // D14 follow + gitlink auto-commit.
	second, _ := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.Add(p.ctx, task.ID, rootA, AddOptions{URL: second, Name: "second"}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{ID: task.ID, Root: rootA, NoRepos: true})
	before := runGitTest(t, p.g, "", "--git-dir", p.b.Home.ClonePath(task.ID), "rev-parse", "HEAD")
	r := p.sync(p.b, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
	after := runGitTest(t, p.g, "", "--git-dir", p.b.Home.ClonePath(task.ID), "rev-parse", "HEAD")
	if before == after || runGitTest(t, p.g, filepath.Join(rootB, "first"), "rev-parse", "HEAD") != newSHA {
		t.Fatalf("root/gitlink did not advance: %+v", r)
	}
	if _, e := os.Stat(filepath.Join(rootB, "second", ".git")); e != nil {
		t.Fatal("new module not materialized", e)
	}
	// --all must leave B's checkout untouched, merely report it behind.
	if _, e := p.a.Remove(p.ctx, task.ID, rootA, "second", false); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	r = p.sync(p.b, SyncOptions{All: true})
	if len(r.CheckoutsBehind) != 1 {
		t.Fatalf("behind report=%+v", r)
	}
	if runGitTest(t, p.g, "", "--git-dir", p.b.Home.ClonePath(task.ID), "rev-parse", "HEAD") != after {
		t.Fatal("--all touched checkout")
	}
	p.sync(p.b, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
	if _, e := os.Stat(filepath.Join(rootB, "second")); !os.IsNotExist(e) {
		t.Fatal("removed repo checkout retained without loss")
	}
	if _, e := os.Stat(filepath.Join(p.b.Home.ClonePath(task.ID), "modules", "second")); !os.IsNotExist(e) {
		t.Fatal("removed repo module dir retained")
	}
	// D53 open on a new machine fetches a definition added after setup.
	c := newSyncMachine(t, "C", p.remote)
	third, _ := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	if _, e := p.a.Add(p.ctx, task.ID, rootA, AddOptions{URL: third, Name: "third"}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	rootC := p.open(c, task.ID)
	if _, e := os.Stat(filepath.Join(rootC, "third", ".git")); e != nil {
		t.Fatal("implicit open omitted latest module", e)
	}
}

func TestSyncS6LossGateAndSkippedGitlinks(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Loss gates")
	repo, seed := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: repo, Name: "repo"})
	p.sync(p.a, SyncOptions{All: true})
	rootA := p.open(p.a, task.ID)
	rootB := p.open(p.b, task.ID)
	// Dirty submodule must not be reset while the root fast-forwards.
	writeSyncFile(t, filepath.Join(rootB, "repo", "dirty"), "work")
	syncRepoCommit(t, p.g, seed, "main", "file", "next\n")
	p.sync(p.a, SyncOptions{ID: task.ID, Root: rootA})
	r := p.sync(p.b, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
	if len(r.Dirty) != 1 || len(r.Skipped) != 1 {
		t.Fatalf("dirty report=%+v", r)
	}
	// The old gitlink creates a root difference; commit it to explicitly retain
	// local state before testing the next pulled definition change.
	os.Remove(filepath.Join(rootB, "repo", "dirty"))
	if _, _, e := p.b.CommitWithRewind(p.ctx, task.ID, rootB, "retain old repo", true); e != nil {
		t.Fatal(e)
	}
	p.sync(p.b, SyncOptions{All: true})
	p.sync(p.a, SyncOptions{ID: task.ID, Root: rootA, NoRepos: true})
	// Removed module with a stash is retained by the existing D46 gate.
	writeSyncFile(t, filepath.Join(rootB, "repo", "file"), "stashed\n")
	runGitTest(t, p.g, filepath.Join(rootB, "repo"), "stash", "push", "-m", "keep work")
	if _, e := p.a.Remove(p.ctx, task.ID, rootA, "repo", false); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	r = p.sync(p.b, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
	if len(r.Skipped) == 0 || !strings.Contains(strings.Join(r.Skipped, "\n"), "stash") {
		t.Fatalf("loss report=%+v", r)
	}
	if _, e := os.Stat(filepath.Join(rootB, "repo", ".git")); e != nil {
		t.Fatal("lossy removal deleted checkout", e)
	}
}

func TestSyncS8ResolveLocalAndRemote(t *testing.T) {
	for _, take := range []string{"local", "remote"} {
		t.Run(take, func(t *testing.T) {
			p := newSyncPair(t)
			task := p.task(p.a, "Resolve")
			rootA := p.open(p.a, task.ID)
			p.sync(p.a, SyncOptions{All: true})
			rootB := p.open(p.b, task.ID)
			writeSyncFile(t, filepath.Join(rootA, "file"), "A\n")
			p.a.Commit(p.ctx, task.ID, rootA, "A edit")
			p.sync(p.a, SyncOptions{All: true})
			writeSyncFile(t, filepath.Join(rootB, "file"), "B\n")
			p.b.Commit(p.ctx, task.ID, rootB, "B edit")
			local := runGitTest(t, p.g, "", "--git-dir", p.b.Home.ClonePath(task.ID), "rev-parse", "HEAD")
			remote := p.a.Hub.Ref(p.ctx, "task/"+task.ID)
			_, e := p.b.Sync(p.ctx, SyncOptions{ID: task.ID, Root: rootB, NoRepos: true})
			requireSyncConflict(t, e)
			result, e := p.b.Resolve(p.ctx, task.ID, rootB, "", nil)
			requireSyncConflict(t, e)
			if len(result.LocalCommits) != 1 || len(result.RemoteCommits) != 1 || len(result.Paths) != 1 || result.Paths[0] != "file" {
				t.Fatalf("comparison=%+v", result)
			}
			result, e = p.b.Resolve(p.ctx, task.ID, rootB, take, nil)
			if e != nil {
				t.Fatalf("resolve: %v %+v", e, result)
			}
			parents := runGitTest(t, p.g, p.b.Home.GitPath(), "show", "-s", "--format=%P", result.Commit)
			if parents != local+" "+remote {
				t.Fatalf("parents=%s", parents)
			}
			want := "B\n"
			if take == "remote" {
				want = "A\n"
			}
			if string(readSyncFile(t, filepath.Join(rootB, "file"))) != want {
				t.Fatal("wrong resolve tree")
			}
			remoteRef := runGitTest(t, p.g, p.remote, "rev-parse", "refs/heads/task/"+task.ID)
			if remoteRef != result.Commit {
				t.Fatal("resolve did not push merge")
			}
		})
	}
}

func TestSyncS7D14ParallelRepoUpdates(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Repo modes")
	activateSyncMachine(t, p.a.App)
	origins, seeds := []string{}, []string{}
	for i := 0; i < 6; i++ {
		origin, seed := syncRepo(t, p.g)
		origins = append(origins, origin)
		seeds = append(seeds, seed)
		name := fmt.Sprintf("r%d", i)
		o := AddOptions{URL: origin, Name: name}
		if i == 1 {
			runGitTest(t, p.g, seed, "checkout", "-b", "own")
			runGitTest(t, p.g, seed, "push", "origin", "own")
			o.Branch = "own"
			o.Mode = "work"
		}
		if i == 2 {
			o.Mode = "pin"
		}
		if _, e := p.a.AddDefined(p.ctx, task.ID, o); e != nil {
			t.Fatal(e)
		}
	}
	root := p.open(p.a, task.ID)
	old := p.a.Hub.Ref(p.ctx, "task/"+task.ID)
	shas := []string{}
	for i, seed := range seeds {
		shas = append(shas, syncRepoCommit(t, p.g, seed, "main", "file", fmt.Sprintf("upstream %d\n", i)))
	}
	ownSHA := syncRepoCommit(t, p.g, seeds[1], "own", "own-file", "own branch\n")
	writeSyncFile(t, filepath.Join(root, "r3", "dirty"), "dirty")
	runGitTest(t, p.g, filepath.Join(root, "r4"), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
	if e := p.a.ConfigSet("sync.jobs", "3"); e != nil {
		t.Fatal(e)
	}
	r, e := p.a.Sync(p.ctx, SyncOptions{ID: task.ID, Root: root})
	if e == nil || len(r.Failed) != 1 {
		t.Fatalf("unreachable must fail only its step: err=%v report=%+v", e, r)
	}
	for _, i := range []int{0, 5} {
		if got := runGitTest(t, p.g, filepath.Join(root, fmt.Sprintf("r%d", i)), "rev-parse", "HEAD"); got != shas[i] {
			t.Fatalf("repo %d not updated", i)
		}
	}
	if got := runGitTest(t, p.g, filepath.Join(root, "r1"), "rev-parse", "HEAD"); got != ownSHA {
		t.Fatal("work repo did not follow own branch")
	}
	if !strings.Contains(strings.Join(r.Steps, "\n"), "upstream drift") || len(r.Dirty) != 1 {
		t.Fatalf("drift/dirty report=%+v", r)
	}
	if got := runGitTest(t, p.g, filepath.Join(root, "r2"), "rev-parse", "HEAD"); got == shas[2] {
		t.Fatal("pin repo moved")
	}
	next := p.a.Hub.Ref(p.ctx, "task/"+task.ID)
	if next == old {
		t.Fatal("no auto-commit")
	}
	count := runGitTest(t, p.g, p.a.Home.GitPath(), "rev-list", "--count", old+".."+next)
	if count != "1" {
		t.Fatalf("expected one aggregate commit, got %s", count)
	}
	subject := runGitTest(t, p.g, p.a.Home.GitPath(), "log", "-1", "--format=%s", next)
	if !strings.Contains(subject, "sync update r0") || !strings.Contains(subject, "r5") {
		t.Fatalf("subject=%s", subject)
	}
	if runGitTest(t, p.g, p.remote, "rev-parse", "refs/heads/task/"+task.ID) != next {
		t.Fatal("auto-commit not pushed")
	}
}

func TestSyncD53OfflineAndCloseS10Doctor(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Close")
	root := p.open(p.a, task.ID)
	p.sync(p.a, SyncOptions{All: true})
	if _, e := p.a.Mark(p.ctx, task.ID, "done", MarkOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("close did not teardown")
	}
	recordText := runGitTest(t, p.g, p.remote, "show", "main:tasks/"+task.ID+".md")
	if !strings.Contains(recordText, "status: done") {
		t.Fatal("close did not push main")
	}
	offline := p.task(p.a, "Offline")
	missing := filepath.Join(t.TempDir(), "missing.git")
	runGitTest(t, p.g, p.a.Home.GitPath(), "remote", "set-url", "origin", missing)
	root = p.open(p.a, offline.ID)
	if !strings.Contains(p.a.warnings.String(), "remote open fetch failed") {
		t.Fatal("open did not warn offline")
	}
	if _, e := p.a.Mark(p.ctx, offline.ID, "done", MarkOptions{}); e != nil {
		t.Fatal("offline close failed", e)
	}
	if !strings.Contains(p.a.warnings.String(), "remote close push failed") {
		t.Fatal("close did not warn offline")
	}
	doctor, e := p.a.Doctor(p.ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	codes := map[string]bool{}
	for _, f := range doctor.Findings {
		codes[f.Code] = true
	}
	if !codes["remote-unreachable"] || !codes["main-ahead"] {
		t.Fatalf("doctor=%+v", doctor)
	}
	// main-behind reads the last-fetched ref, without moving main.
	runGitTest(t, p.g, p.a.Home.GitPath(), "remote", "set-url", "origin", p.remote)
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	p.task(p.b, "Remote ahead")
	p.sync(p.b, SyncOptions{All: true})
	if e = p.a.fetchRemote(p.ctx); e != nil {
		t.Fatal(e)
	}
	doctor, e = p.a.Doctor(p.ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range doctor.Findings {
		if f.Code == "main-behind" {
			found = true
		}
	}
	if !found {
		t.Fatal("doctor omitted main-behind")
	}
}
