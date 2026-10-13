package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reviewSkippedLink(t *testing.T, pin bool) (syncPair, string, string, string, string) {
	t.Helper()
	p := newSyncPair(t)
	task := p.task(p.a, "Skipped gitlinks")
	origin, seed := syncRepo(t, p.g)
	activateSyncMachine(t, p.a.App)
	mode := "follow"
	if pin {
		mode = "pin"
	}
	if _, e := p.a.AddDefined(p.ctx, task.ID, AddOptions{URL: origin, Name: "repo", Mode: mode}); e != nil {
		t.Fatal(e)
	}
	p.sync(p.a, SyncOptions{All: true})
	ra := p.open(p.a, task.ID)
	rb := p.open(p.b, task.ID)
	newSHA := syncRepoCommit(t, p.g, seed, "main", "file", "next\n")
	if pin {
		runGitTest(t, p.g, filepath.Join(ra, "repo"), "fetch", "origin")
		runGitTest(t, p.g, filepath.Join(ra, "repo"), "checkout", "--detach", newSHA)
		if _, _, e := p.a.Commit(p.ctx, task.ID, ra, "pin moved"); e != nil {
			t.Fatal(e)
		}
		p.sync(p.a, SyncOptions{All: true})
	} else {
		p.sync(p.a, SyncOptions{ID: task.ID, Root: ra})
	}
	return p, task.ID, ra, rb, newSHA
}
func TestReviewSkippedGitlinkRetry(t *testing.T) {
	p, id, _, rb, want := reviewSkippedLink(t, false)
	dirty := filepath.Join(rb, "repo", "dirty")
	writeSyncFile(t, dirty, "work")
	r := p.sync(p.b, SyncOptions{ID: id, Root: rb, NoRepos: true})
	if len(r.Skipped) != 1 {
		t.Fatalf("skip=%+v", r)
	}
	// Dirty retry remains safe, even though the root gitlink is modified.
	r = p.sync(p.b, SyncOptions{ID: id, Root: rb, NoRepos: true})
	if len(r.Skipped) != 1 {
		t.Fatalf("dirty retry=%+v", r)
	}
	os.Remove(dirty)
	p.sync(p.b, SyncOptions{ID: id, Root: rb, NoRepos: true})
	if got := runGitTest(t, p.g, filepath.Join(rb, "repo"), "rev-parse", "HEAD"); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
func TestReviewPinGitlinkApply(t *testing.T) {
	p, id, _, rb, want := reviewSkippedLink(t, true)
	p.sync(p.b, SyncOptions{ID: id, Root: rb, NoRepos: true})
	if got := runGitTest(t, p.g, filepath.Join(rb, "repo"), "rev-parse", "HEAD"); got != want {
		t.Fatalf("pin not applied: %s want %s", got, want)
	}
	if got := runGitTest(t, p.g, filepath.Join(rb, "repo"), "branch", "--show-current"); got != "" {
		t.Fatalf("pin attached: %s", got)
	}
}
func TestReviewGitlinkRewindRefusal(t *testing.T) {
	for _, closeTask := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "close"}[closeTask], func(t *testing.T) {
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
			var gate GateError
			if !errors.As(e, &gate) || !strings.Contains(e.Error(), "would move repo back") {
				t.Fatalf("expected rewind gate, got %v", e)
			}
			if closeTask {
				_, e = p.b.Mark(p.ctx, id, "done", MarkOptions{Force: true})
				if e == nil {
					if _, statErr := os.Stat(rb); !os.IsNotExist(statErr) {
						t.Fatal("force close retained checkout")
					}
				}
			} else {
				_, _, e = p.b.CommitWithRewind(p.ctx, id, rb, "explicit rewind", true)
			}
			if e != nil {
				t.Fatalf("rewind override: %v", e)
			}
		})
	}
}
