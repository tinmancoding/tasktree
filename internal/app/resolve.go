package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/hub"
)

type ResolveResult struct {
	LocalCommits  []string   `json:"localCommits"`
	RemoteCommits []string   `json:"remoteCommits"`
	Paths         []string   `json:"paths"`
	AutoMerged    []string   `json:"autoMerged"`
	Commit        string     `json:"commit,omitempty"`
	Report        SyncReport `json:"report"`
}

// Resolve explicitly authorizes a non-fast-forward tree merge. All unchosen
// paths must have a clean git three-way result; it never silently resolves an
// additional conflict in favor of either side.
func (a App) Resolve(ctx context.Context, id, root, take string, paths []string) (ResolveResult, error) {
	result := ResolveResult{LocalCommits: []string{}, RemoteCommits: []string{}, Paths: []string{}, AutoMerged: []string{}, Report: newSyncReport()}
	if take != "" && take != "local" && take != "remote" {
		return result, ValidationError{"--take must be local or remote"}
	}
	if err := a.cleanSyncRoot(ctx, id, root); err != nil {
		return result, err
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return result, err
	}
	if cfg.Remote != "" {
		if err = a.fetchRemote(ctx); err != nil {
			return result, err
		}
	}
	head, _, err := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	local := strings.TrimSpace(head)
	if _, _, err = a.Git.Run(ctx, a.Home.GitPath(), nil, "fetch", "--no-tags", a.Home.ClonePath(id), local); err != nil {
		return result, err
	}
	ref := "refs/heads/task/" + id
	var remote, merged string
	err = a.Hub.WithLock(ctx, func() error {
		if err := a.Hub.EnsureClean(ctx); err != nil {
			return err
		}
		hubHead := a.Hub.Ref(ctx, ref)
		remoteHead := a.Hub.Ref(ctx, "refs/remotes/origin/task/"+id)
		next, ok := a.fastForwardPair(ctx, hubHead, remoteHead)
		if ok {
			remote = next
		} else {
			if !a.Hub.Ancestor(ctx, hubHead, local) {
				return hub.ConflictError{Message: "checkout, hub and remote have three independent histories; commit/preserve the hub history before resolving"}
			}
			remote = remoteHead
		}
		if local == "" || remote == "" {
			return hub.ConflictError{Message: "both task branch histories must exist to resolve"}
		}
		for i, pair := range []string{remote + ".." + local, local + ".." + remote} {
			out, _, e := a.Git.Run(ctx, a.Home.GitPath(), nil, "log", "--oneline", pair)
			if e != nil {
				return e
			}
			list := []string{}
			if strings.TrimSpace(out) != "" {
				list = strings.Split(strings.TrimSpace(out), "\n")
			}
			if i == 0 {
				result.LocalCommits = list
			} else {
				result.RemoteCommits = list
			}
		}
		base, _, e := a.Git.Run(ctx, a.Home.GitPath(), nil, "merge-base", local, remote)
		if e != nil {
			return hub.ConflictError{Message: "task branch histories have no merge base"}
		}
		trees := make([]map[string]string, 3)
		all := map[string]bool{}
		for i, rev := range []string{strings.TrimSpace(base), local, remote} {
			out, _, e := a.Git.Run(ctx, a.Home.GitPath(), nil, "ls-tree", "-r", "-z", rev)
			if e != nil {
				return e
			}
			trees[i] = map[string]string{}
			for _, entry := range strings.Split(out, "\x00") {
				metadata, path, ok := strings.Cut(entry, "\t")
				if ok {
					trees[i][path] = metadata
					all[path] = true
				}
			}
		}
		for _, p := range sortedBoolKeys(all) {
			b, l, r := trees[0][p], trees[1][p], trees[2][p]
			if l == b && r == b {
				continue
			}
			if l != b && r != b && l != r {
				result.Paths = append(result.Paths, p)
			} else {
				result.AutoMerged = append(result.AutoMerged, p)
			}
		}
		selected := map[string]bool{}
		if len(paths) == 0 {
			paths = result.Paths
		}
		for _, p := range paths {
			found := false
			for _, diff := range result.Paths {
				if diff == p {
					found = true
				}
			}
			if !found {
				return ValidationError{fmt.Sprintf("path %q was not changed on both sides with different results", p)}
			}
			selected[p] = true
		}
		if take == "" && len(result.Paths) > 0 {
			return hub.ConflictError{Message: "paths changed on both sides; use resolve --take local|remote [path...]"}
		}
		for _, p := range result.Paths {
			if !selected[p] {
				return hub.ConflictError{Message: "unresolved path " + p + "; include it in resolve --take"}
			}
		}
		args := []string{"merge-tree", "--write-tree", "-z", "--allow-unrelated-histories", local, remote}
		out, _, mergeErr := a.Git.Run(ctx, a.Home.GitPath(), nil, args...)
		parts := strings.Split(out, "\x00")
		if len(parts) == 0 || len(parts[0]) != 40 {
			return fmt.Errorf("merge task branch: %w", mergeErr)
		}
		if mergeErr != nil {
			for _, line := range parts[1:] {
				if line == "" {
					break
				}
				_, path, ok := strings.Cut(line, "\t")
				if !ok || !selected[path] {
					return hub.ConflictError{Message: "unresolved path " + path + "; include it in resolve --take"}
				}
			}
		}
		f, e := os.CreateTemp("", "tt2-resolve-index-*")
		if e != nil {
			return e
		}
		name := f.Name()
		f.Close()
		os.Remove(name)
		defer os.Remove(name)
		env := map[string]string{"GIT_INDEX_FILE": name}
		if _, _, e = a.Git.Run(ctx, a.Home.GitPath(), env, "read-tree", parts[0]); e != nil {
			return e
		}
		chosen := local
		if take == "remote" {
			chosen = remote
		}
		names := []string{}
		for p := range selected {
			names = append(names, p)
		}
		sort.Strings(names)
		for _, p := range names {
			out, _, e = a.Git.Run(ctx, a.Home.GitPath(), nil, "ls-tree", "-z", chosen, "--", p)
			if e != nil {
				return e
			}
			if out == "" {
				_, _, e = a.Git.Run(ctx, a.Home.GitPath(), env, "update-index", "--force-remove", p)
			} else {
				metadata, _, _ := strings.Cut(out, "\t")
				f := strings.Fields(metadata)
				if len(f) != 3 {
					return fmt.Errorf("invalid tree entry for %s", p)
				}
				_, _, e = a.Git.Run(ctx, a.Home.GitPath(), env, "update-index", "--add", "--cacheinfo", f[0]+","+f[2]+","+p)
			}
			if e != nil {
				return e
			}
		}
		tree, _, e := a.Git.Run(ctx, a.Home.GitPath(), env, "write-tree")
		if e != nil {
			return e
		}
		commit, _, e := a.Git.CommitTreeParents(ctx, a.Home.GitPath(), nil, cfg.Author, cfg.Machine, strings.TrimSpace(tree), []string{local, remote}, "tasktree("+id+"): resolve take "+take)
		if e != nil {
			return e
		}
		merged = strings.TrimSpace(commit)
		return a.Hub.MoveRef(ctx, ref, merged, hubHead)
	})
	if err != nil {
		return result, err
	}
	if err = a.advanceCheckout(ctx, id, root, &result.Report); err != nil {
		return result, err
	}
	result.Commit = merged
	if cfg.Remote != "" {
		r, e := a.Sync(ctx, SyncOptions{All: true, NoRepos: true})
		result.Report.Steps = append(result.Report.Steps, r.Steps...)
		result.Report.Failed = append(result.Report.Failed, r.Failed...)
		if e != nil {
			return result, e
		}
	}
	return result, nil
}
