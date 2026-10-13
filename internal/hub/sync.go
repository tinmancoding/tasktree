package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/record"
)

// ConfigureRemote changes only transport configuration. Existing histories are
// never merged here; Sync must explicitly authorize unrelated histories.
func (h Hub) ConfigureRemote(ctx context.Context, url string) error {
	return h.WithLock(ctx, func() error {
		_, _, exists := h.Git.Run(ctx, h.Path, nil, "remote", "get-url", "origin")
		if url == "" {
			if exists != nil {
				return nil
			}
			_, _, err := h.Git.Run(ctx, h.Path, nil, "remote", "remove", "origin")
			return err
		}
		command := "set-url"
		if exists != nil {
			command = "add"
		}
		if _, _, err := h.Git.Run(ctx, h.Path, nil, "remote", command, "origin", url); err != nil {
			return err
		}
		_, _, err := h.Git.Run(ctx, h.Path, nil, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
		return err
	})
}
func (h Hub) Clone(ctx context.Context, url string) error {
	if err := os.MkdirAll(filepath.Dir(h.Path), 0755); err != nil {
		return err
	}
	if _, _, err := h.Git.Run(ctx, "", nil, "clone", "--template=", "--branch", "main", url, h.Path); err != nil {
		return err
	}
	return h.ConfigureRemote(ctx, url)
}
func (h Hub) Ref(ctx context.Context, ref string) string {
	out, _, err := h.Git.Run(ctx, h.Path, nil, "rev-parse", "--verify", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
func (h Hub) Ancestor(ctx context.Context, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	_, _, err := h.Git.Run(ctx, h.Path, nil, "merge-base", "--is-ancestor", a, b)
	return err == nil
}
func (h Hub) Refs(ctx context.Context, prefix string) (map[string]string, error) {
	out, _, err := h.Git.Run(ctx, h.Path, nil, "for-each-ref", "--format=%(refname) %(objectname)", prefix)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 {
			m[f[0]] = f[1]
		}
	}
	return m, nil
}

// MergeMain constructs objects only. On a conflict no refs, index or work tree
// are changed. Git's worktree-free merge handles all non-record paths.
func (h Hub) MergeMain(ctx context.Context, local, remote, author, machine string, allowUnrelated bool) (string, error) {
	author, machine = record.MergeIdentity(author, machine)
	if remote == "" || remote == local || h.Ancestor(ctx, remote, local) {
		return local, nil
	}
	if h.Ancestor(ctx, local, remote) {
		return remote, nil
	}
	base, _, e := h.Git.Run(ctx, h.Path, nil, "merge-base", local, remote)
	base = strings.TrimSpace(base)
	if e != nil && !allowUnrelated {
		return "", ConflictError{"main histories are unrelated; rerun sync --allow-unrelated to join them"}
	}
	args := []string{"merge-tree", "--write-tree", "-z"}
	if base == "" {
		args = append(args, "--allow-unrelated-histories")
	}
	args = append(args, local, remote)
	out, _, mergeErr := h.Git.Run(ctx, h.Path, nil, args...)
	parts := strings.Split(out, "\x00")
	if len(parts) == 0 || len(parts[0]) != 40 {
		return "", fmt.Errorf("merge main: %w", mergeErr)
	}
	if mergeErr != nil {
		for _, line := range parts[1:] {
			if line == "" {
				break
			}
			_, path, ok := strings.Cut(line, "\t")
			if !ok || !taskPath(path) {
				return "", ConflictError{"main conflict on " + path + "; resolve the non-task file before syncing"}
			}
		}
	}
	trees := []string{base, local, remote}
	files := map[string]bool{}
	for _, tree := range trees {
		if tree == "" {
			continue
		}
		names, _, err := h.Git.Run(ctx, h.Path, nil, "ls-tree", "-r", "--name-only", "-z", tree, "--", "tasks")
		if err != nil {
			return "", err
		}
		for _, p := range strings.Split(names, "\x00") {
			if taskPath(p) {
				files[p] = true
			}
		}
	}
	idx, err := os.CreateTemp("", "tt2-sync-index-*")
	if err != nil {
		return "", err
	}
	name := idx.Name()
	idx.Close()
	os.Remove(name)
	defer os.Remove(name)
	env := map[string]string{"GIT_INDEX_FILE": name}
	if _, _, err = h.Git.Run(ctx, h.Path, env, "read-tree", parts[0]); err != nil {
		return "", err
	}
	paths := []string{}
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, path := range paths {
		versions := make([][]byte, 3)
		for i, tree := range trees {
			if tree == "" {
				continue
			}
			text, _, readErr := h.Git.Run(ctx, h.Path, nil, "show", tree+":"+path)
			if readErr == nil {
				versions[i] = []byte(text)
			}
		}
		provenance, _, err := h.Git.Run(ctx, h.Path, nil, "log", "-1", "--format=%h by %an at %cI", remote, "--", path)
		if err != nil {
			return "", err
		}
		merged, err := record.Merge(versions[0], versions[1], versions[2], author, machine, "commit "+strings.TrimSpace(provenance))
		if err != nil {
			return "", ConflictError{fmt.Sprintf("merge %s: %v", path, err)}
		}
		if merged == nil {
			_, _, err = h.Git.Run(ctx, h.Path, env, "update-index", "--force-remove", path)
		} else {
			sha, _, e := h.Git.RunInput(ctx, h.Path, nil, string(merged), "hash-object", "-w", "--no-filters", "--stdin")
			if e != nil {
				return "", e
			}
			_, _, err = h.Git.Run(ctx, h.Path, env, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(sha)+","+path)
		}
		if err != nil {
			return "", err
		}
	}
	tree, _, err := h.Git.Run(ctx, h.Path, env, "write-tree")
	if err != nil {
		return "", err
	}
	commit, _, err := h.Git.CommitTreeParents(ctx, h.Path, nil, author, machine, strings.TrimSpace(tree), []string{local, remote}, "sync: merge "+machine)
	return strings.TrimSpace(commit), err
}
func taskPath(path string) bool {
	return strings.HasPrefix(path, "tasks/") && strings.HasSuffix(path, ".md") && record.ValidID(strings.TrimSuffix(strings.TrimPrefix(path, "tasks/"), ".md"))
}

// MoveRef is a compare-and-swap. main is checked out, so update its index and
// working tree as well, without invoking hooks or changing repository config.
func (h Hub) MoveRef(ctx context.Context, ref, new, old string) error {
	if old == new {
		return nil
	}
	expected := old
	if expected == "" {
		expected = strings.Repeat("0", 40)
	}
	if _, _, err := h.Git.Run(ctx, h.Path, nil, "update-ref", ref, new, expected); err != nil {
		return ConflictError{"hub ref changed concurrently: " + ref}
	}
	if ref == "refs/heads/main" {
		if _, _, err := h.Git.Run(ctx, h.Path, nil, "read-tree", "--reset", "-u", new); err != nil {
			_, _, _ = h.Git.Run(ctx, h.Path, nil, "update-ref", ref, old, new)
			_, _, _ = h.Git.Run(ctx, h.Path, nil, "read-tree", "--reset", "-u", old)
			return err
		}
	}
	return nil
}

// EnsureClean is shared by record writes and sync.
func (h Hub) EnsureClean(ctx context.Context) error { return h.ensureClean(ctx) }
