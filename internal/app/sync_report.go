package app

import (
	"context"
	"path/filepath"
	"strings"
)

// reportGitlinks checks the committed definition, not an uncommitted repo
// HEAD. It deliberately uses the last fetched origin refs (no extra network).
func (a App) reportGitlinks(ctx context.Context, id, root string, r *SyncReport) {
	ms, err := modules(root)
	if err != nil {
		r.Failed = append(r.Failed, err.Error())
		return
	}
	for _, m := range ms {
		if !safeModulePath(root, m) {
			continue
		}
		target, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "rev-parse", "HEAD:"+m.Path)
		if e != nil {
			continue
		}
		target = strings.TrimSpace(target)
		out, _, e := git(ctx, a.Git.WithDefaults(), filepath.Join(root, m.Path), "for-each-ref", "--format=%(refname)", "--contains="+target, "refs/remotes/origin/")
		if e != nil || strings.TrimSpace(out) == "" {
			r.UnpushedGitlinks = appendUnique(r.UnpushedGitlinks, m.Name+": gitlink "+target+" is not reachable from origin")
		}
	}
}
