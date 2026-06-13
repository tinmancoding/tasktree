package snapshot

import (
	"context"
	"fmt"
	"os"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
)

// GitCapture is the raw result of snapshotting a single git source. The caller
// assembles these into the manifest and tarball, choosing the in-tar paths for
// the bundle/dirty payloads.
type GitCapture struct {
	RemoteURL string
	Branch    string
	Detached  bool
	BaseSHA   string
	HeadSHA   string
	Bundle    []byte // nil if there are no local commits beyond base
	Dirty     []byte // nil if the working tree is clean
}

// Git captures the concrete state of a git source rooted at srcPath:
//   - base = merge-base(HEAD, upstream | origin/<branch> | origin/HEAD)
//   - bundle of base..HEAD when local commits exist
//   - tar of working-tree dirty content when dirty
//
// It fails if the source has no 'origin' remote (base would be unrecoverable).
func Git(ctx context.Context, srcPath, sourceName string, includeIgnored bool, git gitx.Client) (GitCapture, error) {
	remoteURL, err := git.RemoteURL(ctx, srcPath, "origin")
	if err != nil {
		return GitCapture{}, err
	}
	if remoteURL == "" {
		return GitCapture{}, domain.MissingOriginRemoteError{Name: sourceName}
	}

	headSHA, err := git.CommitSHA(ctx, srcPath)
	if err != nil {
		return GitCapture{}, err
	}

	branch, err := git.CurrentBranch(ctx, srcPath)
	if err != nil {
		return GitCapture{}, err
	}
	detached := branch == ""

	remoteRef, err := resolveRemoteRef(ctx, git, srcPath, branch)
	if err != nil {
		return GitCapture{}, err
	}

	baseSHA, err := git.MergeBase(ctx, srcPath, "HEAD", remoteRef)
	if err != nil {
		return GitCapture{}, fmt.Errorf("resolve base for %q (against %s): %w", sourceName, remoteRef, err)
	}

	cap := GitCapture{
		RemoteURL: remoteURL,
		Branch:    branch,
		Detached:  detached,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}

	// Committed delta: bundle base..HEAD when there are local commits.
	if baseSHA != headSHA {
		bundleBytes, err := createBundle(ctx, git, srcPath, baseSHA)
		if err != nil {
			return GitCapture{}, err
		}
		cap.Bundle = bundleBytes
	}

	// Dirty state.
	entries, err := git.StatusEntries(ctx, srcPath, includeIgnored)
	if err != nil {
		return GitCapture{}, err
	}
	dirtyBytes, dirty, err := BuildDirtyTar(srcPath, entries)
	if err != nil {
		return GitCapture{}, err
	}
	if dirty {
		cap.Dirty = dirtyBytes
	}

	return cap, nil
}

// resolveRemoteRef picks the ref to compute the base against: the upstream
// tracking ref if set, else origin/<branch>, else origin/HEAD (default branch).
func resolveRemoteRef(ctx context.Context, git gitx.Client, srcPath, branch string) (string, error) {
	upstream, err := git.UpstreamRef(ctx, srcPath)
	if err != nil {
		return "", err
	}
	if upstream != "" {
		return upstream, nil
	}
	if branch != "" {
		candidate := "origin/" + branch
		if _, err := git.ResolveCommit(ctx, srcPath, candidate); err == nil {
			return candidate, nil
		}
	}
	return "origin/HEAD", nil
}

func createBundle(ctx context.Context, git gitx.Client, srcPath, baseSHA string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "tasktree-bundle-*.bundle")
	if err != nil {
		return nil, fmt.Errorf("create bundle temp: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := git.BundleCreate(ctx, srcPath, tmpPath, baseSHA+"..HEAD"); err != nil {
		return nil, fmt.Errorf("create bundle: %w", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	return data, nil
}
