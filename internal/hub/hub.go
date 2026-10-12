package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/fsx"
	"github.com/tinmancoding/tasktree/v2/internal/gitx"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type ConflictError struct{ Message string }

func (e ConflictError) Error() string { return e.Message }

type NotFoundError struct{ Ref string }

func (e NotFoundError) Error() string { return fmt.Sprintf("task %q not found", e.Ref) }

type NotSetupError struct{}

func (NotSetupError) Error() string { return "tasktree is not set up; run tt2 setup" }

type Hub struct {
	Path          string
	Git           gitx.Client
	LockTimeout   time.Duration
	WarningWriter io.Writer
}

func New(path string, g gitx.Client) Hub {
	return Hub{Path: path, Git: g, LockTimeout: 10 * time.Second, WarningWriter: os.Stderr}
}
func (h Hub) IsSetup() bool {
	st, err := os.Stat(filepath.Join(h.Path, ".git"))
	return err == nil && st.IsDir()
}
func (h Hub) Init(ctx context.Context, author, machine string) error {
	if h.IsSetup() {
		return nil
	}
	if err := os.MkdirAll(h.Path, 0o755); err != nil {
		return err
	}
	if _, _, err := h.Git.Run(ctx, "", nil, "init", "-b", "main", h.Path); err != nil {
		return fmt.Errorf("initialize hub: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(h.Path, "tasks"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(h.Path, "tasks", ".gitkeep"), nil, 0o644); err != nil {
		return err
	}
	if _, _, err := h.Git.Run(ctx, h.Path, nil, "add", "tasks/.gitkeep"); err != nil {
		return err
	}
	return h.commit(ctx, "setup tasktree", author, machine)
}
func (h Hub) ReadAll() ([]*record.Document, error) {
	if !h.IsSetup() {
		return nil, NotSetupError{}
	}
	entries, err := os.ReadDir(filepath.Join(h.Path, "tasks"))
	if err != nil {
		return nil, err
	}
	docs := make([]*record.Document, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(h.Path, "tasks", e.Name()))
		if err != nil {
			return nil, err
		}
		d, err := record.Parse(b)
		if err != nil {
			if h.WarningWriter != nil {
				fmt.Fprintf(h.WarningWriter, "warning: parse %s: %v\n", filepath.Join(h.Path, "tasks", e.Name()), err)
			}
			continue
		}
		docs = append(docs, d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Task.CreatedAt.Before(docs[j].Task.CreatedAt) })
	return docs, nil
}
func (h Hub) Read(ref string) (*record.Document, error) {
	if record.ValidID(ref) {
		path := filepath.Join(h.Path, "tasks", ref+".md")
		b, err := os.ReadFile(path)
		if err == nil {
			d, parseErr := record.Parse(b)
			if parseErr != nil {
				return nil, fmt.Errorf("parse %s: %w", path, parseErr)
			}
			return d, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	docs, err := h.ReadAll()
	if err != nil {
		return nil, err
	}
	return Resolve(docs, ref)
}

func Resolve(docs []*record.Document, ref string) (*record.Document, error) {
	for _, d := range docs {
		if d.Task.ID == ref {
			return d, nil
		}
	}
	var matches []*record.Document
	for _, d := range docs {
		if d.Task.Handle == ref {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		return nil, NotFoundError{ref}
	}
	if len(matches) > 1 {
		ids := make([]string, len(matches))
		for i, d := range matches {
			ids[i] = d.Task.ID
		}
		sort.Strings(ids)
		return nil, ConflictError{fmt.Sprintf("handle %q is ambiguous (ids: %s)", ref, strings.Join(ids, ", "))}
	}
	return matches[0], nil
}
func (h Hub) Create(ctx context.Context, d *record.Document, summary, author, machine string) error {
	return h.withLock(ctx, func() error {
		if err := h.ensureClean(ctx); err != nil {
			return err
		}
		docs, err := h.ReadAll()
		if err != nil {
			return err
		}
		for _, existing := range docs {
			if existing.Task.Handle == d.Task.Handle {
				n := 2
				for {
					suggestion := fmt.Sprintf("%s-%d", d.Task.Handle, n)
					used := false
					for _, candidate := range docs {
						if candidate.Task.Handle == suggestion {
							used = true
							break
						}
					}
					if !used {
						return ConflictError{fmt.Sprintf("handle %q already exists; try %q", d.Task.Handle, suggestion)}
					}
					n++
				}
			}
		}
		path := filepath.Join(h.Path, "tasks", d.Task.ID+".md")
		if _, err := os.Stat(path); err == nil {
			return ConflictError{"task id already exists"}
		}
		if _, err := record.Parse(d.Render()); err != nil {
			return fmt.Errorf("refuse to write invalid record: %w", err)
		}
		if err := fsx.AtomicWriteFile(path, d.Render(), 0o644); err != nil {
			return err
		}
		rel := "tasks/" + d.Task.ID + ".md"
		if _, _, err := h.Git.Run(ctx, h.Path, nil, "add", rel); err != nil {
			h.rollbackPath(ctx, rel)
			return err
		}
		if err := h.commit(ctx, fmt.Sprintf("task(%s): %s", d.Task.ID, summary), author, machine); err != nil {
			h.rollbackPath(ctx, rel)
			return err
		}
		return nil
	})
}
func (h Hub) Update(ctx context.Context, ref, summary, author, machine string, mutate func(*record.Document) error) (record.Task, error) {
	var result record.Task
	err := h.withLock(ctx, func() error {
		if err := h.ensureClean(ctx); err != nil {
			return err
		}
		d, err := h.Read(ref)
		if err != nil {
			return err
		}
		if err := mutate(d); err != nil {
			return err
		}
		path := filepath.Join(h.Path, "tasks", d.Task.ID+".md")
		if _, err := record.Parse(d.Render()); err != nil {
			return fmt.Errorf("refuse to write invalid record: %w", err)
		}
		if err := fsx.AtomicWriteFile(path, d.Render(), 0o644); err != nil {
			return err
		}
		rel := "tasks/" + d.Task.ID + ".md"
		if _, _, err := h.Git.Run(ctx, h.Path, nil, "add", rel); err != nil {
			h.rollbackPath(ctx, rel)
			return err
		}
		if err := h.commit(ctx, fmt.Sprintf("task(%s): %s", d.Task.ID, summary), author, machine); err != nil {
			h.rollbackPath(ctx, rel)
			return err
		}
		result = d.Task
		return nil
	})
	return result, err
}
func (h Hub) rollbackPath(ctx context.Context, rel string) {
	// Restore both index and worktree when tracked; remove a newly-created file.
	if _, _, err := h.Git.Run(ctx, h.Path, nil, "cat-file", "-e", "HEAD:"+rel); err == nil {
		_, _, _ = h.Git.Run(ctx, h.Path, nil, "restore", "--source=HEAD", "--staged", "--worktree", "--", rel)
	} else {
		_, _, _ = h.Git.Run(ctx, h.Path, nil, "reset", "HEAD", "--", rel)
		_ = os.Remove(filepath.Join(h.Path, filepath.FromSlash(rel)))
	}
}

func (h Hub) ensureClean(ctx context.Context) error {
	out, _, err := h.Git.Run(ctx, h.Path, nil, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return ConflictError{"hub has uncommitted manual edits; commit or discard them before retrying"}
	}
	return nil
}
func (h Hub) commit(ctx context.Context, message, author, machine string) error {
	env := map[string]string{}
	name, _, nerr := h.Git.Run(ctx, h.Path, nil, "config", "user.name")
	email, _, eerr := h.Git.Run(ctx, h.Path, nil, "config", "user.email")
	if nerr != nil || eerr != nil || strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" {
		if author == "" {
			author = "tasktree"
		}
		if machine == "" {
			machine = "localhost"
		}
		env["GIT_AUTHOR_NAME"] = author
		env["GIT_COMMITTER_NAME"] = author
		env["GIT_AUTHOR_EMAIL"] = author + "@" + machine
		env["GIT_COMMITTER_EMAIL"] = author + "@" + machine
	}
	_, _, err := h.Git.Run(ctx, h.Path, env, "commit", "-m", message)
	return err
}
func (h Hub) withLock(ctx context.Context, fn func() error) error {
	if !h.IsSetup() {
		return NotSetupError{}
	}
	p := filepath.Join(h.Path, ".git", "tasktree.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	timeout := h.LockTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		if time.Now().After(deadline) {
			return ConflictError{"timed out waiting for hub write lock"}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
