package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func interruptReviewSync(t *testing.T, p syncPair, id string) {
	t.Helper()
	p.sync(p.a, SyncOptions{All: true})
	p.sync(p.b, SyncOptions{All: true})
	if _, e := p.b.Note(p.ctx, id, "remote change", "note"); e != nil {
		t.Fatal(e)
	}
	p.sync(p.b, SyncOptions{All: true})
	if e := p.a.fetchRemote(p.ctx); e != nil {
		t.Fatal(e)
	}
	// Simulate the crash window after update-ref, before read-tree.
	runGitTest(t, p.g, p.a.Home.GitPath(), "update-ref", "refs/heads/main", p.a.Hub.Ref(p.ctx, "origin/main"))
}
func TestReviewInterruptedSyncAutoRepair(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "note"}[write], func(t *testing.T) {
			p := newSyncPair(t)
			task := p.task(p.a, "Interrupted")
			interruptReviewSync(t, p, task.ID)
			var e error
			if write {
				_, e = p.a.Note(p.ctx, task.ID, "local change", "note")
			} else {
				_, e = p.a.Sync(p.ctx, SyncOptions{All: true})
			}
			if e != nil {
				t.Fatalf("safe interrupted sync did not repair: %v", e)
			}
			if !strings.Contains(p.a.warnings.String(), "interrupted sync") {
				t.Fatal("repair was not reported")
			}
			d, e := p.a.Hub.Read(task.ID)
			if e != nil {
				t.Fatal(e)
			}
			want := 1
			if write {
				want = 2
			}
			if len(d.Task.Log) != want {
				t.Fatalf("remote record lost: %+v", d.Task)
			}
			if status := runGitTest(t, p.g, p.a.Home.GitPath(), "status", "--porcelain"); status != "" {
				t.Fatalf("hub still dirty: %s", status)
			}
		})
	}
}
func TestReviewInterruptedSyncDoctorSafety(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(map[bool]string{false: "safe", true: "manual-edits"}[dirty], func(t *testing.T) {
			p := newSyncPair(t)
			task := p.task(p.a, "Doctor interrupted")
			interruptReviewSync(t, p, task.ID)
			path := filepath.Join(p.a.Home.GitPath(), "tasks", task.ID+".md")
			if dirty {
				writeSyncFile(t, path, string(readSyncFile(t, path))+"manual work\n")
			}
			before := string(readSyncFile(t, path))
			r, e := p.a.Doctor(p.ctx, false)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, f := range r.Findings {
				if f.Code == "hub-interrupted-sync" {
					found = true
					if f.Severity != "error" || f.Fixable == dirty {
						t.Fatalf("finding=%+v", f)
					}
				}
			}
			if !found {
				t.Fatalf("missing interrupted sync check: %+v", r)
			}
			r, e = p.a.Doctor(p.ctx, true)
			if e != nil {
				t.Fatal(e)
			}
			if dirty {
				if string(readSyncFile(t, path)) != before {
					t.Fatal("doctor overwrote manual edit")
				}
				_, e = p.a.Note(p.ctx, task.ID, "must refuse", "note")
				if e == nil || !strings.Contains(e.Error(), "interrupted sync") {
					t.Fatalf("manual edits not refused: %v", e)
				}
			} else {
				found = false
				for _, f := range r.Findings {
					if f.Code == "hub-interrupted-sync" && f.Fixed {
						found = true
					}
				}
				if !found {
					t.Fatalf("not fixed: %+v", r)
				}
			}
		})
	}
}
