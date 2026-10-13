package app

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestReviewResolveThreeWay(t *testing.T) {
	for _, take := range []string{"", "remote", "local"} {
		t.Run("take-"+take, func(t *testing.T) {
			p := newSyncPair(t)
			task := p.task(p.a, "One-sided resolve")
			ra := p.open(p.a, task.ID)
			p.sync(p.a, SyncOptions{All: true})
			rb := p.open(p.b, task.ID)
			writeSyncFile(t, filepath.Join(ra, "X.md"), "A\n")
			if _, _, e := p.a.Commit(p.ctx, task.ID, ra, "A"); e != nil {
				t.Fatal(e)
			}
			p.sync(p.a, SyncOptions{All: true})
			writeSyncFile(t, filepath.Join(rb, "Y.md"), "B\n")
			if _, _, e := p.b.Commit(p.ctx, task.ID, rb, "B"); e != nil {
				t.Fatal(e)
			}
			_, e := p.b.Sync(p.ctx, SyncOptions{ID: task.ID, Root: rb, NoRepos: true})
			requireSyncConflict(t, e)
			r, e := p.b.Resolve(p.ctx, task.ID, rb, take, nil)
			if e != nil {
				t.Fatalf("one-sided resolve should finish: %v %+v", e, r)
			}
			if len(r.Paths) != 0 {
				t.Fatalf("one-sided paths listed as conflicts: %+v", r)
			}
			for f, want := range map[string]string{"X.md": "A\n", "Y.md": "B\n"} {
				if string(readSyncFile(t, filepath.Join(rb, f))) != want {
					t.Fatal("lost", f)
				}
			}
		})
	}
}
func TestReviewResolveRejectOneSidedSelection(t *testing.T) {
	p := newSyncPair(t)
	task := p.task(p.a, "Both-sides resolve")
	ra := p.open(p.a, task.ID)
	p.sync(p.a, SyncOptions{All: true})
	rb := p.open(p.b, task.ID)
	writeSyncFile(t, filepath.Join(ra, "conflict"), "A\n")
	writeSyncFile(t, filepath.Join(ra, "only-A"), "A\n")
	p.a.Commit(p.ctx, task.ID, ra, "A")
	p.sync(p.a, SyncOptions{All: true})
	writeSyncFile(t, filepath.Join(rb, "conflict"), "B\n")
	writeSyncFile(t, filepath.Join(rb, "only-B"), "B\n")
	p.b.Commit(p.ctx, task.ID, rb, "B")
	r, e := p.b.Resolve(p.ctx, task.ID, rb, "", nil)
	requireSyncConflict(t, e)
	if len(r.Paths) != 1 || r.Paths[0] != "conflict" {
		t.Fatalf("conflicts=%v", r.Paths)
	}
	_, e = p.b.Resolve(p.ctx, task.ID, rb, "remote", []string{"only-A"})
	var validation ValidationError
	if !errors.As(e, &validation) {
		t.Fatalf("want usage error, got %v", e)
	}
	r, e = p.b.Resolve(p.ctx, task.ID, rb, "remote", nil)
	if e != nil {
		t.Fatal(e)
	}
	for f, want := range map[string]string{"conflict": "A\n", "only-A": "A\n", "only-B": "B\n"} {
		if string(readSyncFile(t, filepath.Join(rb, f))) != want {
			t.Fatal("lost", f)
		}
	}
}
