package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tinmancoding/tasktree/v2/internal/hub"
	"github.com/tinmancoding/tasktree/v2/internal/record"
)

type SyncOptions struct {
	All, NoRepos, AllowUnrelated bool
	ID, Root                     string
}
type SyncReport struct {
	Steps               []string `json:"steps"`
	Dirty               []string `json:"dirty"`
	Diverged            []string `json:"diverged"`
	Skipped             []string `json:"skipped"`
	UnpushedGitlinks    []string `json:"unpushedGitlinks"`
	CheckoutsBehind     []string `json:"checkoutsBehind"`
	ClosedTaskCheckouts []string `json:"closedTaskCheckouts"`
	Failed              []string `json:"failed"`
}

func newSyncReport() SyncReport {
	return SyncReport{Steps: []string{}, Dirty: []string{}, Diverged: []string{}, Skipped: []string{}, UnpushedGitlinks: []string{}, CheckoutsBehind: []string{}, ClosedTaskCheckouts: []string{}, Failed: []string{}}
}
func hasConflict(t record.Task, field string) bool {
	for _, f := range t.Conflict {
		if f == field {
			return true
		}
	}
	return false
}

func (a App) pushInitialRemote(ctx context.Context) error {
	return a.Hub.WithLock(ctx, func() error {
		refs, err := a.Hub.Refs(ctx, "refs/heads/")
		if err != nil {
			return err
		}
		args := []string{"push", "--atomic", "origin"}
		for _, ref := range sortedRefs(refs) {
			if ref == "refs/heads/main" || strings.HasPrefix(ref, "refs/heads/task/") {
				args = append(args, ref+":"+ref)
			}
		}
		_, _, err = a.Git.Run(ctx, a.Home.GitPath(), nil, args...)
		return err
	})
}
func sortedRefs(refs map[string]string) []string {
	r := []string{}
	for k := range refs {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func (a App) fetchRemote(ctx context.Context) error {
	_, _, err := a.Git.Run(ctx, a.Home.GitPath(), nil, "fetch", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*")
	return err
}
func (a App) cleanSyncRoot(ctx context.Context, id, root string) error {
	out, _, err := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "status", "--porcelain", "-z", "--ignore-submodules=dirty")
	if err != nil {
		return err
	}
	ms, err := modules(root)
	if err != nil {
		return err
	}
	byPath := map[string]module{}
	for _, m := range ms {
		byPath[m.Path] = m
	}
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) > 3 && entry[:3] == " M " {
			if m, ok := byPath[entry[3:]]; ok {
				_, _, pending, e := a.gitlinkRewind(ctx, id, root, m, true)
				if e != nil {
					return e
				}
				if pending {
					continue
				}
			}
		}
		return hub.ConflictError{Message: "root has uncommitted changes; run tt2 commit first"}
	}
	return nil
}

// Sync fetches without the hub lock, then re-reads all refs under it. Each push
// is leased against precisely the fetched snapshot. A rejected lease retries
// the entire reconciliation (including main's semantic merge) at most 3 times.
func (a App) Sync(ctx context.Context, o SyncOptions) (report SyncReport, err error) {
	report = newSyncReport()
	if !a.Hub.IsSetup() {
		return report, hub.NotSetupError{}
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		return report, err
	}
	if o.Root == "" {
		o.All = true
	}
	defer func() {
		if !o.All {
			a.reportGitlinks(ctx, o.ID, o.Root, &report)
		}
		a.reportCheckouts(ctx, &report)
		if err == nil && len(report.Failed) > 0 {
			err = fmt.Errorf("some sync steps failed")
		}
	}()
	if !o.All {
		if err = a.cleanSyncRoot(ctx, o.ID, o.Root); err != nil {
			report.Dirty = append(report.Dirty, o.Root)
			return report, err
		}
	}
	reposDone := false
	for attempt := 0; attempt < 3; attempt++ {
		fetched := map[string]string{}
		if cfg.Remote != "" {
			if err = a.fetchRemote(ctx); err != nil {
				report.Failed = append(report.Failed, "fetch hub: "+err.Error())
				return report, err
			}
			fetched, err = a.Hub.Refs(ctx, "refs/remotes/origin/")
			if err != nil {
				return report, err
			}
			report.Steps = append(report.Steps, "fetched hub origin")
		}
		cloneHead := ""
		if !o.All {
			// Import objects only: never overwrite the hub branch during fetch.
			out, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, o.ID, o.Root, "rev-parse", "HEAD")
			if e != nil {
				return report, e
			}
			cloneHead = strings.TrimSpace(out)
			if _, _, err = a.Git.Run(ctx, a.Home.GitPath(), nil, "fetch", "--no-tags", a.Home.ClonePath(o.ID), cloneHead); err != nil {
				return report, err
			}
		}
		diverged := map[string]bool{}
		err = a.Hub.WithLock(ctx, func() error {
			if e := a.Hub.EnsureClean(ctx); e != nil {
				return e
			}
			if cfg.Remote != "" {
				var e error
				fetched, e = a.Hub.Refs(ctx, "refs/remotes/origin/")
				if e != nil {
					return e
				}
			}
			local, e := a.Hub.Refs(ctx, "refs/heads/")
			if e != nil {
				return e
			}
			updates := map[string]string{}
			remoteMain := fetched["refs/remotes/origin/main"]
			merged, e := a.Hub.MergeMain(ctx, local["refs/heads/main"], remoteMain, cfg.Author, cfg.Machine, o.AllowUnrelated)
			if e != nil {
				return e
			}
			updates["refs/heads/main"] = merged
			tasks := map[string]bool{}
			if o.All {
				for ref := range local {
					if strings.HasPrefix(ref, "refs/heads/task/") {
						tasks[ref] = true
					}
				}
				for ref := range fetched {
					if strings.HasPrefix(ref, "refs/remotes/origin/task/") {
						tasks[strings.Replace(ref, "refs/remotes/origin/", "refs/heads/", 1)] = true
					}
				}
			} else {
				tasks["refs/heads/task/"+o.ID] = true
			}
			for _, ref := range sortedBoolKeys(tasks) {
				l := local[ref]
				r := fetched[strings.Replace(ref, "refs/heads/", "refs/remotes/origin/", 1)]
				next, ok := a.fastForwardPair(ctx, l, r)
				if ok && !o.All {
					next, ok = a.fastForwardPair(ctx, cloneHead, next)
				}
				if !ok {
					diverged[ref] = true
					report.Diverged = appendUnique(report.Diverged, ref)
					if !o.All {
						return hub.ConflictError{Message: ref + " diverged; run tt2 resolve to inspect and resolve it"}
					}
					continue
				}
				if next != "" {
					updates[ref] = next
				}
			}
			// Construct every merge before touching any refs, especially non-task files.
			if e = a.Hub.MoveRefs(ctx, updates, local); e != nil {
				return e
			}
			report.Steps = append(report.Steps, "reconciled main and task refs")
			return nil
		})
		if err != nil {
			return report, err
		}
		if !o.All {
			if err = a.advanceCheckout(ctx, o.ID, o.Root, &report); err != nil {
				return report, err
			}
			if !o.NoRepos && !reposDone {
				a.updateRepos(ctx, o.ID, o.Root, cfg.Sync.Jobs, &report)
				reposDone = true
			}
		}
		if cfg.Remote == "" {
			report.Steps = append(report.Steps, "no hub remote configured")
			break
		}
		// Test-only deterministic race injection. Scripts must use local remotes;
		// the hook is invoked once per Sync, before its first leased push.
		if attempt == 0 {
			if script := os.Getenv("TASKTREE_TEST_SYNC_BEFORE_PUSH"); script != "" {
				cmd := exec.CommandContext(ctx, "sh", "-c", script)
				cmd.Env = os.Environ()
				if out, e := cmd.CombinedOutput(); e != nil {
					return report, fmt.Errorf("sync test hook: %w: %s", e, out)
				}
			}
		}
		err = a.Hub.WithLock(ctx, func() error {
			if e := a.Hub.EnsureClean(ctx); e != nil {
				return e
			}
			refs, e := a.Hub.Refs(ctx, "refs/heads/")
			if e != nil {
				return e
			}
			args := []string{"push", "--atomic"}
			specs := []string{}
			for _, ref := range sortedRefs(refs) {
				if ref != "refs/heads/main" && (!strings.HasPrefix(ref, "refs/heads/task/") || (!o.All && ref != "refs/heads/task/"+o.ID)) {
					continue
				}
				if diverged[ref] {
					continue
				}
				expected := fetched[strings.Replace(ref, "refs/heads/", "refs/remotes/origin/", 1)]
				args = append(args, "--force-with-lease="+ref+":"+expected)
				specs = append(specs, refs[ref]+":"+ref)
			}
			args = append(args, "origin")
			args = append(args, specs...)
			_, _, e = a.Git.Run(ctx, a.Home.GitPath(), nil, args...)
			return e
		})
		if err == nil {
			report.Steps = append(report.Steps, "pushed leased hub refs")
			break
		}
		if !leaseRejected(err) {
			report.Failed = append(report.Failed, "push hub: "+err.Error())
			return report, err
		}
		report.Steps = append(report.Steps, fmt.Sprintf("push lease rejected; retry %d/3", attempt+1))
		if attempt == 2 {
			return report, hub.ConflictError{Message: "sync push lease rejected after 3 attempts"}
		}
	}
	if len(report.Diverged) > 0 {
		return report, hub.ConflictError{Message: "task branches diverged; open the tasktree and run tt2 resolve"}
	}
	if len(report.Failed) > 0 {
		return report, fmt.Errorf("some sync steps failed")
	}
	return report, nil
}
func leaseRejected(err error) bool {
	s := err.Error()
	return strings.Contains(s, "stale info") || strings.Contains(s, "fetch first") || strings.Contains(s, "non-fast-forward") || strings.Contains(s, "[rejected]") || strings.Contains(s, "[remote rejected]") || strings.Contains(s, "cannot lock ref")
}
func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
func sortedBoolKeys(m map[string]bool) []string {
	r := []string{}
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func (a App) fastForwardPair(ctx context.Context, l, r string) (string, bool) {
	if l == "" {
		return r, true
	}
	if r == "" || l == r || a.Hub.Ancestor(ctx, r, l) {
		return l, true
	}
	if a.Hub.Ancestor(ctx, l, r) {
		return r, true
	}
	return "", false
}
func (a App) reportCheckouts(ctx context.Context, r *SyncReport) {
	local, err := a.Home.LoadLocal()
	if err != nil {
		r.Failed = append(r.Failed, err.Error())
		return
	}
	ids := []string{}
	for id := range local.Checkouts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		co := local.Checkouts[id]
		out, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, co.Path, "rev-parse", "HEAD")
		h := a.Hub.Ref(ctx, "refs/heads/task/"+id)
		if e == nil && strings.TrimSpace(out) != h && a.Hub.Ancestor(ctx, strings.TrimSpace(out), h) {
			r.CheckoutsBehind = appendUnique(r.CheckoutsBehind, co.Path)
		}
		if t, e := a.Info(id); e == nil && !record.IsOpen(t.Status) {
			r.ClosedTaskCheckouts = appendUnique(r.ClosedTaskCheckouts, co.Path)
		}
	}
}

func (a App) advanceCheckout(ctx context.Context, id, root string, r *SyncReport) error {
	g := a.Git.WithDefaults()
	if _, _, err := rootGit(ctx, g, a.Home, id, root, "fetch", "--no-recurse-submodules", "origin", "refs/heads/task/"+id); err != nil {
		return err
	}
	old, _, err := rootGit(ctx, g, a.Home, id, root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	old = strings.TrimSpace(old)
	next := a.Hub.Ref(ctx, "refs/heads/task/"+id)
	if next == old {
		a.applyPendingGitlinks(ctx, id, root, r)
		return nil
	}
	if err = a.cleanSyncRoot(ctx, id, root); err != nil {
		r.Dirty = appendUnique(r.Dirty, root)
		return err
	}
	before, err := modules(root)
	if err != nil {
		return err
	}
	if _, _, err = rootGit(ctx, g, a.Home, id, root, "-c", "submodule.recurse=false", "merge", "--ff-only", next); err != nil {
		return hub.ConflictError{Message: "checkout diverged from hub; run tt2 resolve"}
	}
	a.applyGitlinks(ctx, id, root, old, before, r)
	r.Steps = append(r.Steps, "fast-forwarded task checkout")
	a.refreshTaskRecord(ctx, id)
	return nil
}

// applyGitlinks never resets an existing repository. Removed repositories are
// independently loss-gated; a retained checkout is explicitly reported.
func (a App) applyGitlinks(ctx context.Context, id, root, old string, before []module, r *SyncReport) {
	after, err := modules(root)
	if err != nil {
		r.Failed = append(r.Failed, err.Error())
		return
	}
	kept := map[string]bool{}
	for _, m := range after {
		if !safeModulePath(root, m) {
			r.Failed = append(r.Failed, "unsafe submodule path: "+m.Path)
			continue
		}
		kept[m.Path] = true
		target, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "rev-parse", "HEAD:"+m.Path)
		if e != nil {
			r.Failed = append(r.Failed, e.Error())
			continue
		}
		target = strings.TrimSpace(target)
		oldTarget, _, _ := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "rev-parse", old+":"+m.Path)
		if strings.TrimSpace(oldTarget) == target {
			continue
		}
		path := filepath.Join(root, m.Path)
		if _, e = os.Stat(filepath.Join(path, ".git")); os.IsNotExist(e) {
			if e = a.materializeSelected(ctx, id, root, m); e != nil {
				if strings.Contains(e.Error(), "gitlink commit") {
					r.UnpushedGitlinks = appendUnique(r.UnpushedGitlinks, m.Name+": "+e.Error())
				} else {
					r.Failed = append(r.Failed, m.Name+": "+e.Error())
				}
			} else {
				r.Steps = append(r.Steps, "materialized "+m.Name)
			}
			continue
		}
		a.applyOneGitlink(ctx, m, path, target, r)
	}
	for _, m := range before {
		if kept[m.Path] {
			continue
		}
		if !safeModulePath(root, m) {
			r.Failed = append(r.Failed, "unsafe removed submodule path: "+m.Path)
			continue
		}
		path := filepath.Join(root, m.Path)
		loss, e := repoLoss(ctx, a.Git.WithDefaults(), m.Name, path, false)
		if e != nil || len(loss) > 0 {
			reason := strings.Join(loss, "; ")
			if e != nil {
				reason = e.Error()
			}
			r.Skipped = appendUnique(r.Skipped, m.Name+" removed from definition, checkout retained: "+reason)
			continue
		}
		if e = os.RemoveAll(path); e == nil {
			e = os.RemoveAll(filepath.Join(a.Home.ClonePath(id), "modules", m.Name))
		}
		if e != nil {
			r.Failed = append(r.Failed, m.Name+": "+e.Error())
		} else {
			r.Steps = append(r.Steps, "removed "+m.Name)
		}
	}
}
func safeModulePath(root string, m module) bool {
	if !validRepoName(m.Name) || m.Path == "" || filepath.IsAbs(m.Path) {
		return false
	}
	p := filepath.Clean(m.Path)
	if p == "." || p == ".." || strings.HasPrefix(p, ".."+string(os.PathSeparator)) || strings.HasPrefix(p, ".tasktree") {
		return false
	}
	// Do not follow a symlink parent outside the checkout.
	parent := filepath.Dir(filepath.Join(root, p))
	real, e := filepath.EvalSymlinks(parent)
	if e == nil {
		canonicalRoot, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			return false
		}
		rel, e := filepath.Rel(canonicalRoot, real)
		return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}
	return os.IsNotExist(e)
}
func (a App) applyOneGitlink(ctx context.Context, m module, path, target string, r *SyncReport) {
	g := a.Git.WithDefaults()
	// Retain the target object even when a dirty checkout must be skipped,
	// so commit can later check rewind safety without touching a remote.
	if _, _, missing := git(ctx, g, path, "cat-file", "-e", target+"^{commit}"); missing != nil {
		if _, _, e := git(ctx, g, path, "fetch", "origin"); e != nil {
			r.Failed = append(r.Failed, m.Name+": "+e.Error())
			return
		}
	}
	dirty, e := g.IsDirty(ctx, path)
	if e != nil {
		r.Failed = append(r.Failed, m.Name+": "+e.Error())
		return
	}
	if dirty {
		r.Dirty = appendUnique(r.Dirty, m.Name)
		r.Skipped = appendUnique(r.Skipped, m.Name+": dirty; gitlink not applied")
		return
	}
	branch, e := g.CurrentBranch(ctx, path)
	if e != nil || (m.Mode == "pin" && branch != "") || (m.Mode != "pin" && (branch != m.Branch || branch == "")) {
		r.Skipped = appendUnique(r.Skipped, m.Name+": other branch; gitlink not applied")
		return
	}
	reachable, _, e := git(ctx, g, path, "for-each-ref", "--format=%(refname)", "--contains="+target, "refs/remotes/origin/")
	if e != nil || strings.TrimSpace(reachable) == "" {
		r.UnpushedGitlinks = appendUnique(r.UnpushedGitlinks, m.Name+": gitlink "+target+" is not reachable from origin")
		return
	}
	head, _, e := git(ctx, g, path, "rev-parse", "HEAD")
	if e != nil {
		r.Failed = append(r.Failed, m.Name+": "+e.Error())
		return
	}
	head = strings.TrimSpace(head)
	if _, _, e = git(ctx, g, path, "merge-base", "--is-ancestor", head, target); e != nil {
		r.Skipped = appendUnique(r.Skipped, fmt.Sprintf("%s: non-fast-forward gitlink; branch tip %s stays unchanged, root records chosen gitlink %s and will show it modified; reconcile inside the repo or run tt2 commit to record the repo tip", m.Name, head, target))
		return
	}
	args := []string{"merge", "--ff-only", target}
	if m.Mode == "pin" {
		args = []string{"checkout", "--detach", target}
	}
	if _, _, e = git(ctx, g, path, args...); e != nil {
		r.Skipped = appendUnique(r.Skipped, m.Name+": non-fast-forward gitlink")
		return
	}
	r.Steps = append(r.Steps, "applied gitlink "+m.Name)
}

// materializeSelected reuses the cache and D39 attach path, limited to a new
// submodule so an existing checkout's branch is never reset by materialize.
func (a App) materializeSelected(ctx context.Context, id, root string, m module) error {
	path := filepath.Join(root, m.Path)
	if err := ensureEmpty(path); err != nil {
		return fmt.Errorf("refuse to overwrite new submodule path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(a.Home.ClonePath(id), "modules", m.Name)); err == nil {
		return fmt.Errorf("hidden module directory already exists; run tt2 doctor before materializing %s", m.Name)
	} else if !os.IsNotExist(err) {
		return err
	}
	return a.materializeModules(ctx, id, root, []module{m}, a.WarningWriter)
}

type repoUpdate struct {
	name, path, old, new       string
	dirty, skip, failed, drift string
}

func (a App) updateRepos(ctx context.Context, id, root string, jobs int, r *SyncReport) {
	ms, err := modules(root)
	if err != nil {
		r.Failed = append(r.Failed, err.Error())
		return
	}
	if jobs < 1 {
		jobs = 4
	}
	if jobs > len(ms) {
		jobs = len(ms)
	}
	results := make([]repoUpdate, len(ms))
	work := make(chan int)
	var wg sync.WaitGroup
	for n := 0; n < jobs; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				results[i] = a.updateRepo(ctx, root, ms[i])
			}
		}()
	}
	for i := range ms {
		work <- i
	}
	close(work)
	wg.Wait()
	moves := []string{}
	paths := []string{}
	for _, u := range results {
		if u.dirty != "" {
			r.Dirty = appendUnique(r.Dirty, u.name)
		}
		if u.skip != "" {
			r.Skipped = appendUnique(r.Skipped, u.name+": "+u.skip)
		}
		if u.failed != "" {
			r.Failed = append(r.Failed, u.name+": "+u.failed)
		}
		if u.drift != "" {
			r.Steps = append(r.Steps, u.name+": "+u.drift)
		}
		if u.new != "" && u.old != u.new {
			if _, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "update-index", "--add", "--cacheinfo", "160000,"+u.new+","+u.path); e != nil {
				r.Failed = append(r.Failed, e.Error())
				continue
			}
			moves = append(moves, u.name+" "+u.old[:12]+"→"+u.new[:12])
			paths = append(paths, u.path)
		}
	}
	if len(paths) > 0 {
		if e := a.commitRootMessage(ctx, id, root, fmt.Sprintf("tasktree(%s): sync update %s", id, strings.Join(moves, ", "))); e != nil {
			r.Failed = append(r.Failed, e.Error())
			return
		}
		if _, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, id, root, "push", "origin", "HEAD:refs/heads/task/"+id); e != nil {
			r.Failed = append(r.Failed, e.Error())
			return
		}
		r.Steps = append(r.Steps, "committed gitlink moves: "+strings.Join(moves, ", "))
	}
}
func (a App) updateRepo(ctx context.Context, root string, m module) repoUpdate {
	u := repoUpdate{name: m.Name, path: m.Path}
	if m.Mode == "pin" {
		return u
	}
	if !safeModulePath(root, m) {
		u.failed = "unsafe submodule path"
		return u
	}
	path := filepath.Join(root, m.Path)
	g := a.Git.WithDefaults()
	dirty, e := g.IsDirty(ctx, path)
	if e != nil {
		u.failed = e.Error()
		return u
	}
	if dirty {
		u.dirty = "dirty"
		u.skip = "dirty"
		return u
	}
	branch, e := g.CurrentBranch(ctx, path)
	if e != nil {
		u.failed = e.Error()
		return u
	}
	if branch != m.Branch || branch == "" {
		u.skip = "other branch"
		return u
	}
	if _, _, e = git(ctx, g, path, "fetch", "origin"); e != nil {
		u.failed = e.Error()
		return u
	}
	head, _, e := git(ctx, g, path, "rev-parse", "HEAD")
	if e != nil {
		u.failed = e.Error()
		return u
	}
	u.old = strings.TrimSpace(head)
	if m.Mode == "work" {
		def, _, e := git(ctx, g, path, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if e != nil {
			out, _, x := git(ctx, g, path, "ls-remote", "--symref", "origin", "HEAD")
			if x == nil {
				for _, line := range strings.Split(out, "\n") {
					if strings.HasPrefix(line, "ref: refs/heads/") {
						f := strings.Fields(line)
						if len(f) >= 2 {
							def = "origin/" + strings.TrimPrefix(f[1], "refs/heads/")
						}
					}
				}
			}
		}
		def = strings.TrimSpace(def)
		if def != "" {
			counts, _, e := git(ctx, g, path, "rev-list", "--left-right", "--count", u.old+"..."+def)
			if e == nil {
				var ahead, behind int
				fmt.Sscanf(counts, "%d %d", &ahead, &behind)
				if behind > 0 {
					u.drift = fmt.Sprintf("upstream drift: %d commit(s) on %s (not merged into work branch)", behind, def)
				}
			}
		}
	}
	remote := "origin/" + m.Branch
	if _, _, e = git(ctx, g, path, "rev-parse", "--verify", remote); e != nil {
		u.skip = "no origin/" + m.Branch
		return u
	}
	if _, _, e = git(ctx, g, path, "merge", "--ff-only", remote); e != nil {
		u.skip = "non-fast-forward " + remote
		return u
	}
	head, _, e = git(ctx, g, path, "rev-parse", "HEAD")
	if e != nil {
		u.failed = e.Error()
		return u
	}
	u.new = strings.TrimSpace(head)
	return u
}

func (a App) pushCloseRemote(ctx context.Context, id string) {
	cfg, e := a.Home.LoadConfig()
	if e != nil || cfg.Remote == "" {
		return
	}
	// Best effort and fast-forward only: closing must not merge or overwrite a
	// remote history. The local hub already holds the complete closed task.
	args := []string{"push", "--atomic", "origin", "refs/heads/main:refs/heads/main"}
	if a.DefinitionExists(ctx, id) {
		args = append(args, "refs/heads/task/"+id+":refs/heads/task/"+id)
	}
	_, _, e = a.Git.Run(ctx, a.Home.GitPath(), nil, args...)
	if e != nil {
		a.warnf("remote close push failed (safe in local hub): %v", e)
	}
}
func (a App) pullOpenRemote(ctx context.Context, ref string) {
	cfg, e := a.Home.LoadConfig()
	if e != nil || cfg.Remote == "" {
		return
	}
	if e = a.fetchRemote(ctx); e != nil {
		a.warnf("remote open fetch failed; continuing with local state: %v", e)
		return
	}
	e = a.Hub.WithLock(ctx, func() error {
		if e := a.Hub.EnsureClean(ctx); e != nil {
			return e
		}
		old := a.Hub.Ref(ctx, "refs/heads/main")
		remote := a.Hub.Ref(ctx, "refs/remotes/origin/main")
		next, e := a.Hub.MergeMain(ctx, old, remote, cfg.Author, cfg.Machine, false)
		if e != nil {
			return e
		}
		if e = a.Hub.MoveRef(ctx, "refs/heads/main", next, old); e != nil {
			return e
		}
		refs, e := a.Hub.Refs(ctx, "refs/remotes/origin/task/")
		if e != nil {
			return e
		}
		task, e := a.Info(ref)
		if e != nil {
			return e
		}
		for _, ref := range sortedRefs(refs) {
			if ref != "refs/remotes/origin/task/"+task.ID {
				continue
			}
			localRef := strings.Replace(ref, "refs/remotes/origin/", "refs/heads/", 1)
			l := a.Hub.Ref(ctx, localRef)
			n, ok := a.fastForwardPair(ctx, l, refs[ref])
			if !ok {
				a.warnf("%s diverged; run tt2 sync and resolve", localRef)
				continue
			}
			if e = a.Hub.MoveRef(ctx, localRef, n, l); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		a.warnf("remote open reconciliation failed; continuing with local state: %v", e)
	}
}
