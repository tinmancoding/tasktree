package hub

import (
	"context"
	"sort"
	"strings"
)

// MoveRefs changes the entire planned ref set with one CAS transaction. The
// checked-out main index/worktree follows immediately while the hub lock is
// held; if that update fails the ref transaction and worktree are rolled back.
func (h Hub) MoveRefs(ctx context.Context, updates, old map[string]string) error {
	names := []string{}
	for ref, next := range updates {
		if next != "" && next != old[ref] {
			names = append(names, ref)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil
	}
	transaction := func(reverse bool) error {
		var b strings.Builder
		b.WriteString("start\n")
		for _, ref := range names {
			before, next := old[ref], updates[ref]
			if reverse {
				before, next = next, before
			}
			if before == "" {
				before = strings.Repeat("0", 40)
			}
			if next == "" {
				b.WriteString("delete " + ref + " " + before + "\n")
			} else {
				b.WriteString("update " + ref + " " + next + " " + before + "\n")
			}
		}
		b.WriteString("prepare\ncommit\n")
		_, _, err := h.Git.RunInput(ctx, h.Path, nil, b.String(), "update-ref", "--stdin")
		return err
	}
	if err := transaction(false); err != nil {
		return ConflictError{"hub refs changed concurrently; retry sync: " + err.Error()}
	}
	main := "refs/heads/main"
	if updates[main] != "" && updates[main] != old[main] {
		if _, _, err := h.Git.Run(ctx, h.Path, nil, "read-tree", "--reset", "-u", updates[main]); err != nil {
			rollback := transaction(true)
			_, _, _ = h.Git.Run(ctx, h.Path, nil, "read-tree", "--reset", "-u", old[main])
			if rollback != nil {
				return ConflictError{"main worktree update and rollback failed; run doctor: " + rollback.Error()}
			}
			return err
		}
	}
	return nil
}
