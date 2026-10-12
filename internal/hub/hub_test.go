package hub_test

import (
	"context"
	"fmt"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.New(filepath.Join(t.TempDir(), "git"), gitx.NewClient())
	if err := h.Init(context.Background(), "tester", "machine"); err != nil {
		t.Fatal(err)
	}
	return h
}
func taskDoc(t *testing.T) *record.Document {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := record.New(record.Task{ID: "abc123", Handle: "task", Title: "Task", Status: "incoming", Labels: []string{}, Refs: []record.Ref{}, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestInitIdempotent(t *testing.T) {
	h := newHub(t)
	if err := h.Init(context.Background(), "tester", "machine"); err != nil {
		t.Fatal(err)
	}
	out, _, err := h.Git.Run(context.Background(), h.Path, nil, "rev-list", "--count", "HEAD")
	if err != nil || strings.TrimSpace(out) != "1" {
		t.Fatalf("count=%q err=%v", out, err)
	}
}
func TestDirtyHubRefused(t *testing.T) {
	h := newHub(t)
	if err := os.WriteFile(filepath.Join(h.Path, "manual"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := h.Create(context.Background(), taskDoc(t), "create", "tester", "machine")
	if err == nil || !strings.Contains(err.Error(), "manual edits") {
		t.Fatalf("got %v", err)
	}
}
func TestConcurrentWritersNoLostEntries(t *testing.T) {
	h := newHub(t)
	if err := h.Create(context.Background(), taskDoc(t), "create", "tester", "machine"); err != nil {
		t.Fatal(err)
	}
	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := record.NewID()
			if err != nil {
				errs <- err
				return
			}
			_, err = h.Update(context.Background(), "abc123", "note added", "tester", "machine", func(d *record.Document) error {
				return d.Append(record.Entry{ID: id, Timestamp: time.Now().UTC(), Kind: "note", Author: "tester", Machine: "machine", Body: fmt.Sprintf("note-%d", i)})
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	docs, err := h.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(docs[0].Task.Log); got != n {
		t.Fatalf("entries=%d want %d", got, n)
	}
	out, _, err := h.Git.Run(context.Background(), h.Path, nil, "rev-list", "--count", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "12" {
		t.Fatalf("commits=%q want 12", out)
	}
}
