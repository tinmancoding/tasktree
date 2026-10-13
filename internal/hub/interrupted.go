package hub

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InterruptedSync recognizes the crash window between moving main and applying
// its tree: the index is still exactly the previous HEAD tree. Call under the
// hub lock. A nonmatching index is a user's staged edit, not a repair candidate.
func (h Hub) InterruptedSync(ctx context.Context) (interrupted, fixable bool, err error) {
	head, _, err := h.Git.Run(ctx, h.Path, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return false, false, err
	}
	index, _, err := h.Git.Run(ctx, h.Path, nil, "write-tree")
	if err != nil {
		return false, false, err
	}
	if strings.TrimSpace(index) == strings.TrimSpace(head) {
		return false, false, nil
	}
	previous, _, e := h.Git.Run(ctx, h.Path, nil, "rev-parse", "HEAD@{1}^{tree}")
	if e != nil || strings.TrimSpace(previous) != strings.TrimSpace(index) {
		return false, false, nil
	}
	matches, e := h.worktreeMatchesIndex(ctx)
	return true, matches, e
}

func (h Hub) worktreeMatchesIndex(ctx context.Context) (bool, error) {
	others, _, e := h.Git.Run(ctx, h.Path, nil, "ls-files", "--others", "-z")
	if e != nil {
		return false, e
	}
	if others != "" {
		return false, nil
	}
	entries, _, e := h.Git.Run(ctx, h.Path, nil, "ls-files", "--stage", "-z")
	if e != nil {
		return false, e
	}
	for _, entry := range strings.Split(entries, "\x00") {
		if entry == "" {
			continue
		}
		metadata, path, ok := strings.Cut(entry, "\t")
		f := strings.Fields(metadata)
		if !ok || len(f) != 3 || f[2] != "0" {
			return false, nil
		}
		p := filepath.Join(h.Path, filepath.FromSlash(path))
		info, e := os.Lstat(p)
		if os.IsNotExist(e) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		var actual []byte
		switch f[0] {
		case "120000":
			if info.Mode()&os.ModeSymlink == 0 {
				return false, nil
			}
			target, e := os.Readlink(p)
			if e != nil {
				return false, e
			}
			actual = []byte(target)
		case "100644", "100755":
			if !info.Mode().IsRegular() || (info.Mode().Perm()&0111 != 0) != (f[0] == "100755") {
				return false, nil
			}
			actual, e = os.ReadFile(p)
			if e != nil {
				return false, e
			}
		default:
			return false, nil
		}
		expected, _, e := h.Git.Run(ctx, h.Path, nil, "cat-file", "blob", f[1])
		if e != nil {
			return false, e
		}
		if !bytes.Equal(actual, []byte(expected)) {
			return false, nil
		}
	}
	return true, nil
}
func (h Hub) repairInterruptedSync(ctx context.Context) error {
	interrupted, fixable, e := h.InterruptedSync(ctx)
	if e != nil {
		return e
	}
	if !interrupted {
		return nil
	}
	if !fixable {
		return ConflictError{"interrupted sync with manual edits; preserve or commit them before repairing"}
	}
	_, _, e = h.Git.Run(ctx, h.Path, nil, "read-tree", "--reset", "-u", "HEAD")
	if e == nil && h.WarningWriter != nil {
		fmt.Fprintln(h.WarningWriter, "warning: interrupted sync repaired; reapplied hub HEAD tree")
	}
	return e
}

// RepairInterruptedSync rechecks the exact tree condition while holding the
// write lock; a doctor snapshot is never itself authorization to discard work.
func (h Hub) RepairInterruptedSync(ctx context.Context) error {
	return h.WithLock(ctx, func() error { return h.repairInterruptedSync(ctx) })
}
