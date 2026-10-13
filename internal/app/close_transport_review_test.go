package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewClosePushesMainDespiteTaskRejection(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "teardown", true: "keep"}[keep], func(t *testing.T) {
			p := newSyncPair(t)
			task := p.task(p.a, "Independent close push")
			ra := p.open(p.a, task.ID)
			p.sync(p.a, SyncOptions{All: true})
			rb := p.open(p.b, task.ID)
			writeSyncFile(t, filepath.Join(rb, "remote-file"), "B\n")
			if _, _, e := p.b.Commit(p.ctx, task.ID, rb, "B root"); e != nil {
				t.Fatal(e)
			}
			p.sync(p.b, SyncOptions{All: true})
			remoteHead := runGitTest(t, p.g, p.remote, "rev-parse", "task/"+task.ID)
			if _, e := p.a.Mark(p.ctx, task.ID, "done", MarkOptions{Keep: keep}); e != nil {
				t.Fatal(e)
			}
			raw := runGitTest(t, p.g, p.remote, "show", "main:tasks/"+task.ID+".md")
			if !strings.Contains(raw, "status: done") {
				t.Fatal("task rejection blocked main status push")
			}
			if head := runGitTest(t, p.g, p.remote, "rev-parse", "task/"+task.ID); head != remoteHead {
				t.Fatal("remote task branch overwritten")
			}
			_, e := os.Stat(ra)
			if keep && e != nil {
				t.Fatal("keep removed checkout")
			}
			if !keep && !os.IsNotExist(e) {
				t.Fatal("close retained checkout")
			}
		})
	}
}
func TestReviewConfigTimeoutAppliesToImplicitCalls(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Timeout configuration")
	sleeper := filepath.Join(t.TempDir(), "sleep-remote")
	writeSyncFile(t, sleeper, "#!/bin/sh\nsleep 2\n")
	os.Chmod(sleeper, 0755)
	t.Setenv("GIT_ALLOW_PROTOCOL", "file:ext")
	runGitTest(t, p.g, p.a.Home.GitPath(), "remote", "set-url", "origin", "ext::"+sleeper)
	if e := p.a.ConfigSet("sync.timeout", "50ms"); e != nil {
		t.Fatal(e)
	}
	if _, e := networkTimeout("bad", time.Second); e == nil {
		t.Fatal("invalid timeout accepted")
	}
	for name, call := range map[string]func(){
		"sync": func() {
			_, e := p.a.Sync(p.ctx, SyncOptions{All: true})
			if e == nil {
				t.Error("sync succeeded")
			}
		},
		"open":   func() { p.a.pullOpenRemote(p.ctx, task.ID) },
		"close":  func() { p.a.pushCloseRemote(p.ctx, task.ID) },
		"doctor": func() { p.a.Doctor(p.ctx, false) },
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			call()
			if time.Since(start) > time.Second {
				t.Fatalf("configured deadline ignored: %s", time.Since(start))
			}
		})
	}
}
