package gitx

import (
	"context"
	"fmt"
	"strings"
)

// StatusEntry is one parsed line of `git status --porcelain=v1 -z`.
type StatusEntry struct {
	X         byte   // index (staged) status
	Y         byte   // worktree status
	Path      string // current path (new path for renames/copies)
	OrigPath  string // original path for renames/copies; empty otherwise
	Untracked bool   // true for "??" entries
}

// Staged reports whether the entry has a staged (index) change.
func (e StatusEntry) Staged() bool {
	return e.X != ' ' && e.X != '?' && e.X != '!'
}

// RemoteURL returns the fetch URL for the named remote, or empty string if the
// remote does not exist.
func (c Client) RemoteURL(ctx context.Context, repoPath, remote string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "remote", "get-url", remote)
	if err != nil {
		if cmdErr, ok := err.(CommandError); ok && (cmdErr.ExitCode == 2 || cmdErr.ExitCode == 128) {
			return "", nil
		}
		return "", err
	}
	return trimOutput(stdout), nil
}

// UpstreamRef returns the upstream tracking ref of the current branch (e.g.
// "origin/feature"), or empty string if there is no upstream.
func (c Client) UpstreamRef(ctx context.Context, repoPath string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		return trimOutput(stdout), nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode == 128 {
		return "", nil
	}
	return "", err
}

// MergeBase returns the best common ancestor of a and b.
func (c Client) MergeBase(ctx context.Context, repoPath, a, b string) (string, error) {
	stdout, _, err := c.run(ctx, "-C", repoPath, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return trimOutput(stdout), nil
}

// BundleCreate writes a git bundle of revRange (e.g. "<base>..HEAD") to outPath.
// The caller must ensure the range is non-empty (base != HEAD).
func (c Client) BundleCreate(ctx context.Context, repoPath, outPath, revRange string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "bundle", "create", outPath, revRange)
	return err
}

// StatusEntries returns the parsed porcelain status of the working tree.
// untrackedAll lists individual untracked files; includeIgnored also reports
// ignored files.
func (c Client) StatusEntries(ctx context.Context, repoPath string, includeIgnored bool) ([]StatusEntry, error) {
	args := []string{"-C", repoPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"}
	if includeIgnored {
		args = append(args, "--ignored=matching")
	}
	stdout, _, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseStatusZ(stdout), nil
}

func parseStatusZ(data string) []StatusEntry {
	tokens := strings.Split(data, "\x00")
	entries := make([]StatusEntry, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if len(tok) < 3 {
			continue
		}
		e := StatusEntry{X: tok[0], Y: tok[1], Path: tok[3:]}
		if e.X == '?' && e.Y == '?' {
			e.Untracked = true
		}
		if e.X == 'R' || e.X == 'C' || e.Y == 'R' || e.Y == 'C' {
			if i+1 < len(tokens) {
				e.OrigPath = tokens[i+1]
				i++
			}
		}
		entries = append(entries, e)
	}
	return entries
}

// FetchBundle imports the objects from a git bundle file into repoPath, mapping
// the bundle's HEAD onto refs/snapshot/head so the objects are anchored.
func (c Client) FetchBundle(ctx context.Context, repoPath, bundlePath string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "fetch", "--no-tags", bundlePath, "+HEAD:refs/snapshot/head")
	return err
}

// FetchSHA fetches a specific commit SHA from the named remote. If the server
// rejects fetch-by-SHA, it falls back to a full fetch.
func (c Client) FetchSHA(ctx context.Context, repoPath, remote, sha string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "fetch", "--no-tags", remote, sha)
	if err == nil {
		return nil
	}
	// Fall back to a full fetch; the SHA may then be reachable.
	_, _, ferr := c.run(ctx, "-C", repoPath, "fetch", "--no-tags", remote, "+refs/heads/*:refs/remotes/"+remote+"/*")
	if ferr != nil {
		return fmt.Errorf("fetch commit %s from %s: %w", sha, remote, err)
	}
	return nil
}

// HasCommit reports whether the given commit object exists in the repo.
func (c Client) HasCommit(ctx context.Context, repoPath, sha string) (bool, error) {
	_, _, err := c.run(ctx, "-C", repoPath, "cat-file", "-e", sha+"^{commit}")
	if err == nil {
		return true, nil
	}
	if cmdErr, ok := err.(CommandError); ok && cmdErr.ExitCode != 0 {
		return false, nil
	}
	return false, err
}

// CheckoutDetached checks out a commit SHA without creating a branch.
func (c Client) CheckoutDetached(ctx context.Context, repoPath, sha string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "checkout", "--detach", sha)
	return err
}

// CreateBranchAt creates branch pointed at sha and checks it out.
func (c Client) CreateBranchAt(ctx context.Context, repoPath, branch, sha string) error {
	_, _, err := c.run(ctx, "-C", repoPath, "checkout", "-B", branch, sha)
	return err
}

// AddPaths stages the given paths (best-effort re-stage hint on restore).
func (c Client) AddPaths(ctx context.Context, repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"-C", repoPath, "add", "--force", "--"}, paths...)
	_, _, err := c.run(ctx, args...)
	return err
}
