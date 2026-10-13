package app

import (
	"context"
	"fmt"
	"github.com/tinmancoding/tasktree/v2/internal/product"
	"path/filepath"
	"strings"
)

// gitlinkRewind identifies a checkout left behind by a skipped pulled gitlink.
// Comparing ancestry (rather than merely different SHAs) does not authorize
// overwriting a user's ahead/diverged repository branch.
func (a App) gitlinkRewind(ctx context.Context, id, root string, m module, fetchMissing bool) (recorded, head string, rewind bool, err error) {
	if !safeModulePath(root, m) {
		return "", "", false, fmt.Errorf("unsafe submodule path: %s", m.Path)
	}
	g := a.Git.WithDefaults()
	path := filepath.Join(root, m.Path)
	out, _, e := rootGit(ctx, g, a.Home, id, root, "rev-parse", "HEAD:"+m.Path)
	if e != nil {
		return "", "", false, nil
	}
	recorded = strings.TrimSpace(out)
	out, _, e = git(ctx, g, path, "rev-parse", "HEAD")
	if e != nil {
		return recorded, "", false, e
	}
	head = strings.TrimSpace(out)
	if head == recorded {
		return recorded, head, false, nil
	}
	if _, _, missing := git(ctx, g, path, "cat-file", "-e", recorded+"^{commit}"); missing != nil {
		if !fetchMissing {
			return recorded, head, false, GateError{"cannot verify recorded gitlink for " + m.Name + "; run " + product.Name + " sync, or pass --allow-rewind"}
		}
		if _, _, e = git(ctx, g, path, "fetch", "origin"); e != nil {
			return recorded, head, false, e
		}
	}
	_, _, e = git(ctx, g, path, "merge-base", "--is-ancestor", head, recorded)
	return recorded, head, e == nil, nil
}
func (a App) refuseGitlinkRewinds(ctx context.Context, id, root string) error {
	ms, e := modules(root)
	if e != nil {
		return e
	}
	for _, m := range ms {
		recorded, head, rewind, e := a.gitlinkRewind(ctx, id, root, m, false)
		if e != nil {
			return e
		}
		if rewind {
			return GateError{fmt.Sprintf("would move %s back from %s to %s; run "+product.Name+" sync, or pass --allow-rewind to record the older commit", m.Name, recorded, head)}
		}
	}
	return nil
}
func (a App) applyPendingGitlinks(ctx context.Context, id, root string, r *SyncReport) {
	ms, e := modules(root)
	if e != nil {
		r.Failed = append(r.Failed, e.Error())
		return
	}
	for _, m := range ms {
		target, _, pending, e := a.gitlinkRewind(ctx, id, root, m, true)
		if e != nil {
			r.Failed = append(r.Failed, m.Name+": "+e.Error())
			continue
		}
		if pending {
			a.applyOneGitlink(ctx, m, filepath.Join(root, m.Path), target, r)
		}
	}
}
