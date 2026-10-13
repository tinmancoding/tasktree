package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/home"
)

type moveModuleSnapshot struct {
	m        module
	snapshot repoSnapshot
}

// Move relocates an existing checkout without changing any repository state.
func (a App) Move(ctx context.Context, id, oldPath, newPath string) (string, error) {
	if oldPath == "" {
		return "", GateError{"task has no checkout"}
	}
	oldPath, _ = filepath.Abs(oldPath)
	var err error
	newPath, err = filepath.Abs(newPath)
	if err != nil {
		return "", err
	}
	if oldPath == newPath {
		return oldPath, nil
	}
	st, err := os.Stat(oldPath)
	if err != nil || !st.IsDir() {
		return "", GateError{"recorded checkout does not exist"}
	}
	local, err := a.Home.LoadLocal()
	if err != nil {
		return "", err
	}
	co, ok := local.Checkouts[id]
	if !ok || filepath.Clean(co.Path) != oldPath {
		return "", GateError{"task has no matching checkout in local.yml"}
	}
	if rel, e := filepath.Rel(oldPath, newPath); e == nil && (rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return "", GateError{"move target must not be inside the current checkout"}
	}
	if err = preflightMove(oldPath, newPath); err != nil {
		return "", GateError{err.Error()}
	}
	if err = requireSameFilesystem(oldPath, newPath); err != nil {
		return "", GateError{err.Error()}
	}
	ms, err := modules(oldPath)
	if err != nil {
		return "", err
	}
	for _, m := range ms {
		gd := filepath.Join(a.Home.ClonePath(id), "modules", m.Name)
		for _, marker := range []string{"index.lock", "rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "sequencer"} {
			if _, e := os.Stat(filepath.Join(gd, marker)); e == nil {
				return "", GateError{fmt.Sprintf("repository %s has operation in progress (%s)", m.Name, marker)}
			}
		}
	}
	snaps := make([]moveModuleSnapshot, 0, len(ms))
	for _, m := range ms {
		s, e := snapshotRepo(ctx, a, filepath.Join(oldPath, m.Path))
		if e != nil {
			return "", fmt.Errorf("snapshot %s: %w", m.Name, e)
		}
		snaps = append(snaps, moveModuleSnapshot{m, s})
	}
	rootHead, _, err := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, oldPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	rootStatus, _, err := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, oldPath, "status", "--porcelain=v2", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	targetWasEmpty := false
	if entries, e := os.ReadDir(newPath); e == nil && len(entries) == 0 {
		targetWasEmpty = true
		if e = os.Remove(newPath); e != nil {
			return "", e
		}
	}
	if err = os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return "", err
	}
	if err = os.Rename(oldPath, newPath); err != nil {
		return "", err
	}
	moved := true
	localChanged := false
	rollback := func(cause error) (string, error) {
		if moved {
			if e := os.Rename(newPath, oldPath); e != nil {
				return "", fmt.Errorf("move failed: %v; rollback rename failed: %w", cause, e)
			}
		}
		if targetWasEmpty {
			_ = os.Mkdir(newPath, 0755)
		}
		for _, x := range snaps {
			_ = rewriteSubmodule(a.Home, id, oldPath, x.m)
		}
		_, _, _ = git(context.Background(), a.Git.WithDefaults(), "", "config", "--file", filepath.Join(a.Home.ClonePath(id), "config"), "--unset-all", "core.worktree")
		if localChanged {
			_ = a.Home.UpdateLocal(func(s *home.LocalState) error {
				c := s.Checkouts[id]
				c.Path = oldPath
				s.Checkouts[id] = c
				return nil
			})
		}
		return "", cause
	}
	if os.Getenv("TASKTREE_TEST_MOVE_FAIL") == "1" {
		return rollback(fmt.Errorf("injected move failure (TASKTREE_TEST_MOVE_FAIL=1)"))
	}
	for _, x := range snaps {
		if err = rewriteSubmodule(a.Home, id, newPath, x.m); err != nil {
			return rollback(err)
		}
	}
	_, _, _ = git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(a.Home.ClonePath(id), "config"), "--unset-all", "core.worktree")
	if err = a.Home.UpdateLocal(func(s *home.LocalState) error {
		c, exists := s.Checkouts[id]
		if !exists || filepath.Clean(c.Path) != oldPath {
			return fmt.Errorf("checkout changed in local.yml during move")
		}
		c.Path = newPath
		s.Checkouts[id] = c
		return nil
	}); err != nil {
		return rollback(err)
	}
	localChanged = true
	a.refreshTaskRecord(ctx, id)
	for _, x := range snaps {
		after, e := snapshotRepo(ctx, a, filepath.Join(newPath, x.m.Path))
		if e != nil {
			return rollback(fmt.Errorf("verify %s: %w", x.m.Name, e))
		}
		if e = snapshotsEqual(x.snapshot, after, true); e != nil {
			return rollback(fmt.Errorf("verify %s: %w", x.m.Name, e))
		}
	}
	afterHead, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, newPath, "rev-parse", "HEAD")
	if e != nil {
		return rollback(e)
	}
	afterStatus, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, newPath, "status", "--porcelain=v2", "--untracked-files=all")
	if e != nil {
		return rollback(e)
	}
	if rootHead != afterHead || rootStatus != afterStatus {
		return rollback(fmt.Errorf("root repository snapshot changed"))
	}
	return newPath, nil
}
