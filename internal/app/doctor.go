package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tinmancoding/tasktree/v2/internal/product"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinmancoding/tasktree/v2/internal/home"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"gopkg.in/yaml.v3"
)

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
	Fixable  bool   `json:"fixable"`
	Fixed    bool   `json:"fixed"`
	fix      func(context.Context) error
}

type DoctorResult struct {
	Findings []Finding `json:"findings"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
}

func (r *DoctorResult) count() {
	r.Errors, r.Warnings = 0, 0
	for _, f := range r.Findings {
		if f.Fixed {
			continue
		}
		if f.Severity == "error" {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
}

// Doctor audits both durable hub state and this machine's checkout registry.
// With fix it applies only repairs which cannot discard repository work.
func (a App) Doctor(ctx context.Context, fix bool) (DoctorResult, error) {
	initial, err := a.doctorFindings(ctx)
	if err != nil {
		return DoctorResult{}, err
	}
	if !fix {
		r := DoctorResult{Findings: initial}
		r.count()
		return r, nil
	}
	fixed := []Finding{}
	for _, f := range initial {
		if !f.Fixable || f.fix == nil {
			continue
		}
		if err := f.fix(ctx); err != nil {
			fixed = append(fixed, Finding{Code: "check-failed", Severity: "error", Subject: f.Subject, Message: fmt.Sprintf("fix %s: %v", f.Code, err)})
			continue
		}
		f.Fixed = true
		f.fix = nil
		fixed = append(fixed, f)
	}
	remaining, err := a.doctorFindings(ctx)
	if err != nil {
		return DoctorResult{}, err
	}
	r := DoctorResult{Findings: append(fixed, remaining...)}
	r.count()
	return r, nil
}

func (a App) doctorFindings(ctx context.Context) ([]Finding, error) {
	var out []Finding
	add := func(code, severity, subject, message string, fix func(context.Context) error) {
		out = append(out, Finding{Code: code, Severity: severity, Subject: subject, Message: message, Fixable: fix != nil, fix: fix})
	}
	cfg, err := a.Home.LoadConfig()
	if err != nil {
		add("check-failed", "error", a.Home.ConfigPath(), err.Error(), nil)
		return out, nil
	}
	if cfg.Remote == "" {
		add("no-remote", "warning", a.Home.Root, "no remote configured; ~/.tasktree is the only copy; make a backup", nil)
	}
	if cfg.Remote != "" && a.Hub.IsSetup() {
		short, e := a.implicitNetworkContext(ctx)
		if e == nil {
			_, _, e = a.Git.Run(short, a.Home.GitPath(), nil, "ls-remote", "origin", "refs/heads/main")
		}
		if e != nil {
			add("remote-unreachable", "warning", cfg.Remote, "hub remote is unreachable: "+e.Error(), nil)
		}
		counts, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "rev-list", "--left-right", "--count", "main...refs/remotes/origin/main")
		if e == nil {
			var ahead, behind int
			fmt.Sscanf(counts, "%d %d", &ahead, &behind)
			if ahead > 0 {
				add("main-ahead", "warning", "main", fmt.Sprintf("%d commit(s) ahead of last fetched origin/main", ahead), nil)
			}
			if behind > 0 {
				add("main-behind", "warning", "main", fmt.Sprintf("%d commit(s) behind last fetched origin/main", behind), nil)
			}
		}
	}
	if a.Hub.IsSetup() {
		interrupted, fixable := false, false
		e := a.Hub.WithLock(ctx, func() error {
			var err error
			interrupted, fixable, err = a.Hub.InterruptedSync(ctx)
			return err
		})
		if e != nil {
			add("check-failed", "error", a.Home.GitPath(), e.Error(), nil)
		}
		if interrupted {
			var repair func(context.Context) error
			if fixable {
				repair = a.Hub.RepairInterruptedSync
			}
			add("hub-interrupted-sync", "error", a.Home.GitPath(), "interrupted sync: hub index still records the pre-merge tree", repair)
		}
		status, _, e := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "status", "--porcelain")
		if e != nil {
			add("check-failed", "error", a.Home.GitPath(), e.Error(), nil)
		} else if strings.TrimSpace(status) != "" && !interrupted {
			add("hub-dirty", "error", a.Home.GitPath(), "hub has uncommitted changes", nil)
		}
	}

	// Read records directly: Hub.ReadAll deliberately skips malformed records.
	docs := []*record.Document{}
	tasksDir := filepath.Join(a.Home.GitPath(), "tasks")
	entries, e := os.ReadDir(tasksDir)
	if e != nil && !os.IsNotExist(e) {
		add("check-failed", "error", tasksDir, e.Error(), nil)
		entries = nil
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".md") {
			continue
		}
		p := filepath.Join(tasksDir, ent.Name())
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			add("check-failed", "error", p, readErr.Error(), nil)
			continue
		}
		d, parseErr := record.Parse(b)
		if parseErr != nil {
			add("record-unparseable", "error", p, parseErr.Error(), nil)
			continue
		}
		docs = append(docs, d)
		if len(d.Task.Conflict) > 0 {
			add("conflict-flag", "warning", d.Task.Handle, "unacknowledged sync conflicts: "+strings.Join(d.Task.Conflict, ", "), nil)
		}
	}
	byID := map[string]*record.Document{}
	byHandle := map[string][]string{}
	for _, d := range docs {
		byID[d.Task.ID] = d
		byHandle[d.Task.Handle] = append(byHandle[d.Task.Handle], d.Task.ID)
	}
	for h, ids := range byHandle {
		if len(ids) > 1 {
			sort.Strings(ids)
			add("duplicate-handle", "error", h, "records share this handle: "+strings.Join(ids, ", ")+"; run "+product.Name+" info set handle … -t <id>", nil)
		}
	}
	for _, d := range docs {
		if d.Task.Parent != nil {
			if _, ok := byID[*d.Task.Parent]; !ok {
				add("parent-missing", "error", d.Task.Handle, "parent id "+*d.Task.Parent+" was not found", nil)
			}
		}
	}
	seenCycles := map[string]bool{}
	for _, d := range docs {
		path, pos := []string{}, map[string]int{}
		id := d.Task.ID
		for id != "" {
			if at, ok := pos[id]; ok {
				cycle := append(path[at:], id)
				key := strings.Join(cycle, "->")
				if !seenCycles[key] {
					seenCycles[key] = true
					add("parent-cycle", "error", d.Task.Handle, "parent cycle: "+strings.Join(cycle, " -> ")+"; break it with "+product.Name+" info set parent --clear -t <id>", nil)
				}
				break
			}
			pos[id] = len(path)
			path = append(path, id)
			n := byID[id]
			if n == nil || n.Task.Parent == nil {
				break
			}
			id = *n.Task.Parent
		}
	}

	local, err := a.Home.LoadLocal()
	if err != nil {
		add("check-failed", "error", a.Home.LocalPath(), err.Error(), nil)
		local = home.LocalState{Checkouts: map[string]home.Checkout{}}
	}
	cloneIDs := map[string]bool{}
	cloneEntries, e := os.ReadDir(a.Home.ClonesPath())
	if e != nil && !os.IsNotExist(e) {
		add("check-failed", "error", a.Home.ClonesPath(), e.Error(), nil)
		cloneEntries = nil
	}
	for _, ent := range cloneEntries {
		if ent.IsDir() && strings.HasSuffix(ent.Name(), ".git") {
			cloneIDs[strings.TrimSuffix(ent.Name(), ".git")] = true
		}
	}
	importIDs := incompleteImportIDs(a.Home)
	for id, co := range local.Checkouts {
		clone := a.Home.ClonePath(id)
		if st, statErr := os.Stat(co.Path); statErr != nil || !st.IsDir() {
			if importIDs[id] {
				// The clone holds absorbed repositories that only the import journal can restore.
				add("checkout-missing", "error", id, "recorded checkout path does not exist and an import for this task is incomplete; run "+product.Name+" import --recover <path>", nil)
				continue
			}
			if found := a.findMovedCheckout(cfg.TasksRoot, id, co.Path); found != "" {
				// The folder was moved by hand: never delete anything, re-point instead.
				idCopy, foundCopy := id, found
				add("checkout-missing", "error", id, "recorded checkout path does not exist; matching checkout found at "+found+"; --fix re-points local.yml and the repository paths there", func(context.Context) error {
					return a.repointCheckout(ctx, idCopy, foundCopy)
				})
				continue
			}
			if !cloneIDs[id] {
				idCopy := id
				add("checkout-missing", "error", id, "recorded checkout path does not exist", func(context.Context) error {
					return a.Home.UpdateLocal(func(s *home.LocalState) error { delete(s.Checkouts, idCopy); return nil })
				})
			} else {
				loss, lossErr := lossInClone(ctx, a, clone)
				if lossErr != nil {
					add("check-failed", "error", clone, lossErr.Error(), nil)
					continue
				}
				if len(loss) > 0 {
					add("checkout-missing", "error", id, strings.Join(loss, "; ")+"; restore the folder or run "+product.Name+" open "+handleFor(byID, id)+" --force", nil)
				} else {
					idCopy, cloneCopy := id, clone
					add("checkout-missing", "error", id, "recorded checkout path does not exist; hidden clone has no work at risk", func(context.Context) error {
						if err := os.RemoveAll(cloneCopy); err != nil {
							return err
						}
						return a.Home.UpdateLocal(func(s *home.LocalState) error { delete(s.Checkouts, idCopy); return nil })
					})
				}
			}
			continue
		}
		marker, markerErr := markerTaskID(co.Path)
		if markerErr != nil || marker != id {
			add("marker-mismatch", "error", co.Path, "missing marker or taskId does not match "+id, nil)
			continue
		}
		ms, modErr := modules(co.Path)
		if modErr != nil {
			add("check-failed", "error", co.Path, modErr.Error(), nil)
			continue
		}
		for _, m := range ms {
			expectedGit := filepath.Join(clone, "modules", m.Name)
			expectedWT := filepath.Join(co.Path, m.Path)
			if _, gitdirErr := os.Stat(expectedGit); os.IsNotExist(gitdirErr) && !moduleInitialized(co.Path, m) {
				continue // never materialized: its gitlink commit was unavailable (DN5)
			}
			dotBytes, dotErr := os.ReadFile(filepath.Join(expectedWT, ".git"))
			wt, _, wtErr := git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(expectedGit, "config"), "--get", "core.worktree")
			actualGit := strings.TrimSpace(strings.TrimPrefix(string(dotBytes), "gitdir:"))
			actualWT := strings.TrimSpace(wt)
			if dotErr != nil || wtErr != nil || filepath.Clean(actualGit) != filepath.Clean(expectedGit) || filepath.Clean(actualWT) != filepath.Clean(expectedWT) {
				mm := m
				idCopy, rootCopy := id, co.Path
				add("path-drift", "error", expectedWT, "submodule gitdir/core.worktree does not match checkout location", func(context.Context) error { return rewriteSubmodule(a.Home, idCopy, rootCopy, mm) })
			}
		}
		rootWT, _, rootErr := git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(clone, "config"), "--get", "core.worktree")
		if rootErr == nil && strings.TrimSpace(rootWT) != "" {
			cloneCopy := clone
			add("root-core-worktree", "warning", id, "root clone config has core.worktree set", func(context.Context) error {
				_, _, e := git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(cloneCopy, "config"), "--unset-all", "core.worktree")
				return e
			})
		}
		cloneHead, _, chErr := git(ctx, a.Git.WithDefaults(), "", "--git-dir", clone, "rev-parse", "HEAD")
		hubRef, _, hrErr := git(ctx, a.Git.WithDefaults(), a.Home.GitPath(), "rev-parse", "refs/heads/task/"+id)
		if chErr == nil && hrErr == nil && strings.TrimSpace(cloneHead) != strings.TrimSpace(hubRef) {
			_, _, anc := git(ctx, a.Git.WithDefaults(), "", "--git-dir", clone, "merge-base", "--is-ancestor", strings.TrimSpace(hubRef), strings.TrimSpace(cloneHead))
			if anc == nil {
				idCopy, rootCopy := id, co.Path
				add("branch-unpushed-to-hub", "warning", handleFor(byID, id), "checkout HEAD is ahead of hub task branch", func(context.Context) error {
					_, _, e := rootGit(ctx, a.Git.WithDefaults(), a.Home, idCopy, rootCopy, "push", "origin", "HEAD:refs/heads/task/"+idCopy)
					return e
				})
			} else {
				add("branch-unpushed-to-hub", "warning", handleFor(byID, id), "checkout and hub task branch have diverged", nil)
			}
		}
		if d := byID[id]; d != nil && !record.IsOpen(d.Task.Status) {
			add("closed-but-checked-out", "warning", d.Task.Handle, "closed task still has a checkout; run "+product.Name+" mark <status> -t "+d.Task.Handle, nil)
		}
	}
	for id := range cloneIDs {
		if _, ok := local.Checkouts[id]; ok {
			continue
		}
		if importIDs[id] {
			continue // owned by an incomplete import; reported under import-journal only
		}
		found := findCheckoutMarker(cfg.TasksRoot, id, 3)
		if found != "" {
			idCopy, pathCopy := id, found
			add("orphan-clone", "error", id, "hidden clone has no local.yml entry; matching checkout found at "+found, func(context.Context) error {
				return a.Home.UpdateLocal(func(s *home.LocalState) error { s.Checkouts[idCopy] = home.Checkout{Path: pathCopy}; return nil })
			})
			continue
		}
		clone := a.Home.ClonePath(id)
		loss, lossErr := lossInClone(ctx, a, clone)
		if lossErr != nil {
			add("check-failed", "error", clone, lossErr.Error(), nil)
			continue
		}
		if len(loss) > 0 {
			add("orphan-clone", "error", id, strings.Join(loss, "; ")+"; restore its checkout before removing the clone", nil)
		} else {
			cloneCopy := clone
			add("orphan-clone", "error", id, "hidden clone has no checkout and no work at risk", func(context.Context) error { return os.RemoveAll(cloneCopy) })
		}
	}
	imports := filepath.Join(a.Home.Root, "imports")
	if xs, e := os.ReadDir(imports); e == nil {
		for _, x := range xs {
			if x.IsDir() {
				p := filepath.Join(imports, x.Name())
				add("import-journal", "error", p, "incomplete import or quarantine; run "+product.Name+" import --recover <path>", nil)
			}
		}
	} else if !os.IsNotExist(e) {
		add("check-failed", "error", imports, e.Error(), nil)
	}

	// An isolated child's work branch should retain its parent's branch prefix.
	for _, d := range docs {
		if d.Task.Parent == nil {
			continue
		}
		child, cok := local.Checkouts[d.Task.ID]
		parent, pok := local.Checkouts[*d.Task.Parent]
		if !cok || !pok {
			continue
		}
		pmods, _ := modules(parent.Path)
		cmods, _ := modules(child.Path)
		pm := map[string]module{}
		for _, m := range pmods {
			pm[m.Name] = m
		}
		for _, m := range cmods {
			p, ok := pm[m.Name]
			if !ok || p.Branch == "" {
				continue
			}
			branch, _ := a.Git.CurrentBranch(ctx, filepath.Join(child.Path, m.Path))
			if branch != "" && !strings.HasPrefix(branch, p.Branch+"-") {
				add("subtask-branch-mismatch", "warning", d.Task.Handle+":"+m.Name, "branch "+branch+" no longer starts with "+p.Branch+"- after reparent", nil)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Subject < out[j].Subject
	})
	return out, nil
}

// findMovedCheckout looks for a directory whose marker names the task: first
// below tasksRoot (depth 3), then among the siblings of the recorded path.
func (a App) findMovedCheckout(tasksRoot, id, recorded string) string {
	if found := findCheckoutMarker(tasksRoot, id, 3); found != "" {
		return found
	}
	if recorded == "" {
		return ""
	}
	if parent := filepath.Dir(filepath.Clean(recorded)); parent != filepath.Clean(recorded) {
		return findCheckoutMarker(parent, id, 1)
	}
	return ""
}

// repointCheckout makes local.yml and every initialized repository's .git file
// and core.worktree agree with a checkout folder that was moved by hand. It
// never deletes anything.
func (a App) repointCheckout(ctx context.Context, id, newPath string) error {
	marker, err := markerTaskID(newPath)
	if err != nil || marker != id {
		return fmt.Errorf("%s is not the checkout of %s", newPath, id)
	}
	ms, err := modules(newPath)
	if err != nil {
		return err
	}
	if err = a.Home.UpdateLocal(func(s *home.LocalState) error {
		c := s.Checkouts[id]
		c.Path = newPath
		s.Checkouts[id] = c
		return nil
	}); err != nil {
		return err
	}
	for _, m := range ms {
		if !moduleInitialized(newPath, m) {
			continue // never materialized (DN5): nothing to re-point
		}
		if err = rewriteSubmodule(a.Home, id, newPath, m); err != nil {
			return err
		}
	}
	if _, statErr := os.Stat(a.Home.ClonePath(id)); statErr == nil {
		_, _, _ = git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(a.Home.ClonePath(id), "config"), "--unset-all", "core.worktree")
	}
	a.refreshTaskRecord(ctx, id)
	return nil
}

// incompleteImportIDs returns every task id that has a directory under
// imports/ (journal or quarantine). Its clone is never safe to delete.
func incompleteImportIDs(h home.Home) map[string]bool {
	ids := map[string]bool{}
	base := filepath.Join(h.Root, "imports")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ids
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		ids[ent.Name()] = true
		if b, e := os.ReadFile(filepath.Join(base, ent.Name(), "journal.json")); e == nil {
			var j struct{ ID string }
			if json.Unmarshal(b, &j) == nil && j.ID != "" {
				ids[j.ID] = true
			}
		}
	}
	return ids
}

// lossInClone includes the standard hidden-git-dir gate, then additionally
// checks a still-existing configured worktree. This prevents a stale registry
// from turning dirty files outside tasksRoot into apparently safe clone data.
func lossInClone(ctx context.Context, a App, clone string) ([]string, error) {
	loss, err := lossInModules(ctx, a.Git.WithDefaults(), clone)
	if err != nil {
		return nil, err
	}
	dirs, err := moduleGitDirs(clone)
	if err != nil {
		return loss, err
	}
	for _, dir := range dirs {
		wt, _, e := git(ctx, a.Git.WithDefaults(), "", "config", "--file", filepath.Join(dir, "config"), "--get", "core.worktree")
		if e != nil {
			continue
		}
		wt = strings.TrimSpace(wt)
		if wt == "" || !filepath.IsAbs(wt) {
			continue
		}
		if st, e := os.Stat(wt); e != nil || !st.IsDir() {
			continue
		}
		status, _, e := git(ctx, a.Git.WithDefaults(), wt, "status", "--porcelain", "--untracked-files=all")
		if e != nil {
			return loss, e
		}
		if strings.TrimSpace(status) != "" {
			loss = append(loss, filepath.Base(dir)+" has uncommitted changes")
		}
	}
	return loss, nil
}

func handleFor(byID map[string]*record.Document, id string) string {
	if d := byID[id]; d != nil {
		return d.Task.Handle
	}
	return id
}
func findCheckoutMarker(root, id string, depth int) string {
	if root == "" {
		return ""
	}
	root = filepath.Clean(root)
	found := ""
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		n := 0
		if rel != "." {
			n = len(strings.Split(rel, string(filepath.Separator)))
		}
		if d.IsDir() && n > depth {
			return filepath.SkipDir
		}
		if d.IsDir() {
			marker := filepath.Join(p, ".tasktree", "config.yml")
			b, err := os.ReadFile(marker)
			if err == nil {
				var c TasktreeConfig
				if yaml.Unmarshal(b, &c) == nil && c.TaskID == id {
					found = p
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return found
}
