package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/record"
)

func TestReviewMergedBlobNoFilters(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "CRLF merge")
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	path := "tasks/" + task.ID + ".md"
	base := readSyncFile(t, filepath.Join(p.a.Home.GitPath(), path))
	l, _ := record.Parse(base)
	r, _ := record.Parse(base)
	l.SetStatus("active", l.Task.UpdatedAt)
	r.SetFrontString("title", "remote title", r.Task.UpdatedAt)
	lb := bytes.ReplaceAll(l.Render(), []byte("\n"), []byte("\r\n"))
	rb := bytes.ReplaceAll(r.Render(), []byte("\n"), []byte("\r\n"))
	for i, a := range []syncMachine{p.a, p.b} {
		raw := [][]byte{lb, rb}[i]
		sha, _, e := p.g.RunInput(p.ctx, a.Home.GitPath(), nil, string(raw), "hash-object", "-w", "--no-filters", "--stdin")
		if e != nil {
			t.Fatal(e)
		}
		runGitTest(t, p.g, a.Home.GitPath(), "update-index", "--cacheinfo", "100644,"+strings.TrimSpace(sha)+","+path)
		if _, _, e := p.g.Commit(p.ctx, a.Home.GitPath(), nil, "test", "local", "CRLF fixture"); e != nil {
			t.Fatal(e)
		}
		runGitTest(t, p.g, a.Home.GitPath(), "read-tree", "--reset", "-u", "HEAD")
	}
	runGitTest(t, p.g, p.a.Home.GitPath(), "config", "core.autocrlf", "true")
	local := p.a.Hub.Ref(p.ctx, "main")
	remote := p.b.Hub.Ref(p.ctx, "main")
	runGitTest(t, p.g, p.a.Home.GitPath(), "fetch", p.b.Home.GitPath(), remote)
	provenance := runGitTest(t, p.g, p.a.Home.GitPath(), "log", "-1", "--format=%h by %an at %cI", remote, "--", path)
	want, e := record.Merge(base, lb, rb, "A", "A", "commit "+provenance)
	if e != nil {
		t.Fatal(e)
	}
	merged, e := p.a.Hub.MergeMain(p.ctx, local, remote, "A", "A", false)
	if e != nil {
		t.Fatal(e)
	}
	got, _, e := p.g.Run(p.ctx, p.a.Home.GitPath(), nil, "show", merged+":"+path)
	if e != nil || !bytes.Equal([]byte(got), want) {
		t.Fatalf("merged blob filtered: %v\ngot=%q\nwant=%q", e, got, want)
	}
}
func TestReviewPushDoesNotHoldLock(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Slow push")
	marker := filepath.Join(t.TempDir(), "pushing")
	hook := filepath.Join(p.remote, "hooks", "pre-receive")
	writeSyncFile(t, hook, fmt.Sprintf("#!/bin/sh\ntouch %q\nsleep 2\n", marker))
	os.Chmod(hook, 0755)
	a := p.a.App
	a.Hub.LockTimeout = 250 * time.Millisecond
	done := make(chan error, 1)
	go func() { _, e := a.Sync(p.ctx, SyncOptions{All: true}); done <- e }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e := os.Stat(marker); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("push never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, noteErr := a.Note(p.ctx, task.ID, "during push", "note")
	syncErr := <-done
	if noteErr != nil || syncErr != nil {
		t.Fatalf("note during push=%v sync=%v", noteErr, syncErr)
	}
	if d, e := a.Hub.Read(task.ID); e != nil || len(d.Task.Log) != 1 {
		t.Fatalf("note lost: %v", e)
	}
}
func TestReviewNoEnvironmentShellHook(t *testing.T) {
	p := newSyncPair(t)
	p.task(p.a, "No injected shell")
	marker := filepath.Join(t.TempDir(), "executed")
	t.Setenv("TASKTREE_TEST_SYNC_BEFORE_PUSH", fmt.Sprintf("touch %q", marker))
	p.sync(p.a, SyncOptions{All: true})
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("shipped sync executed an environment shell script")
	}
}
func TestReviewServerRejectionNoRetry(t *testing.T) {
	p := newSyncPair(t)
	p.task(p.a, "Rejected push")
	hook := filepath.Join(p.remote, "hooks", "pre-receive")
	count := filepath.Join(t.TempDir(), "count")
	writeSyncFile(t, hook, fmt.Sprintf("#!/bin/sh\necho attempt >> %q\necho protected-ref-policy >&2\nexit 1\n", count))
	os.Chmod(hook, 0755)
	r, e := p.a.Sync(p.ctx, SyncOptions{All: true})
	if e == nil || !strings.Contains(e.Error(), "protected-ref-policy") {
		t.Fatalf("missing rejection: %v", e)
	}
	if strings.Count(string(readSyncFile(t, count)), "attempt") != 1 || strings.Contains(strings.Join(r.Steps, "\n"), "retry") {
		t.Fatalf("policy retried: %+v %v", r, e)
	}
}
func TestReviewNetworkDeadline(t *testing.T) {
	p := newSyncPair(t)
	p.task(p.a, "Hanging transport")
	// ext invokes an offline local transport child which holds its inherited
	// pipes open after git is canceled; WaitDelay must bound that case too.
	sleeper := filepath.Join(t.TempDir(), "sleep-remote")
	writeSyncFile(t, sleeper, "#!/bin/sh\nsleep 2\n")
	os.Chmod(sleeper, 0755)
	t.Setenv("GIT_ALLOW_PROTOCOL", "file:ext")
	runGitTest(t, p.g, p.a.Home.GitPath(), "remote", "set-url", "origin", "ext::"+sleeper)
	// A supplied context bounds each caller even before configurable timeouts.
	for _, call := range []struct {
		name string
		fn   func(context.Context)
	}{
		{"sync", func(ctx context.Context) {
			_, e := p.a.Sync(ctx, SyncOptions{All: true})
			if e == nil {
				t.Error("sync succeeded")
			}
		}},
		{"open", func(ctx context.Context) { p.a.pullOpenRemote(ctx, "hanging-transport") }},
		{"close", func(ctx context.Context) { p.a.pushCloseRemote(ctx, "abc123") }},
		{"doctor", func(ctx context.Context) { p.a.Doctor(ctx, false) }},
	} {
		t.Run(call.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(p.ctx, 75*time.Millisecond)
			defer cancel()
			start := time.Now()
			call.fn(ctx)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("timeout waited for child: %s", elapsed)
			}
		})
	}
}
