package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// machine is one isolated TASKTREE_HOME; use switches the process to it.
type machine struct {
	t           *testing.T
	home, cache string
	tasks       string
}

func newMachine(t *testing.T, name string) *machine {
	t.Helper()
	base := t.TempDir()
	return &machine{t: t, home: filepath.Join(base, "home"), cache: filepath.Join(base, "cache"), tasks: filepath.Join(base, "tasks")}
}

func (m *machine) use() {
	m.t.Helper()
	m.t.Setenv("TASKTREE_HOME", m.home)
	m.t.Setenv("TASKTREE_CACHE_DIR", m.cache)
	m.t.Setenv("TASKTREE_V1_ALIASES_FILE", filepath.Join(m.home, "missing"))
	m.t.Setenv("TASKTREE_AUTHOR", "")
	m.t.Setenv("TASKTREE_TASK", "")
}

func (m *machine) run(args ...string) (string, string, error) {
	m.t.Helper()
	m.use()
	return run(m.t, args...)
}

func (m *machine) must(args ...string) string {
	m.t.Helper()
	out, errOut, err := m.run(args...)
	if err != nil {
		m.t.Fatalf("%v: %v\nstdout=%s\nstderr=%s", args, err, out, errOut)
	}
	return out
}

func (m *machine) inDir(dir string, args ...string) (string, string, error) {
	m.t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		m.t.Fatal(err)
	}
	defer os.Chdir(old)
	return m.run(args...)
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// twoMachines returns two machines sharing one bare hub remote.
func twoMachines(t *testing.T) (a, b *machine, remote string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	a, b = newMachine(t, "a"), newMachine(t, "b")
	remote = filepath.Join(t.TempDir(), "hub.git")
	gitRun(t, "", "init", "--bare", "-b", "main", remote)
	a.must("setup", "--author", "alice", "--machine", "boxa", "--tasks-root", a.tasks)
	a.must("config", "set", "remote", remote)
	b.must("setup", "--author", "bob", "--machine", "boxb", "--tasks-root", b.tasks, "--remote", remote)
	return
}

// M31: a task branch that is not diverged has nothing to resolve; a one-sided
// merge says what it merged.
func TestResolveReportsNothingToResolveAndWhatItMerged(t *testing.T) {
	a, b, _ := twoMachines(t)
	out := a.must("new", "Resolve me", "--handle", "resolve-me", "--open")
	ra := strings.TrimSpace(out)
	a.inDir(ra, "sync")
	rb := strings.TrimSpace(b.must("open", "resolve-me"))

	taskRefs := func() string {
		return gitRun(t, filepath.Join(b.home, "git"), "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task/")
	}
	refsBefore := taskRefs()
	if out, _, err := b.inDir(rb, "resolve"); err != nil || !strings.Contains(out, "nothing to resolve") {
		t.Fatalf("not diverged: err=%v out=%q", err, out)
	}
	if taskRefs() != refsBefore {
		t.Fatal("resolve of a non-diverged branch made a commit")
	}

	writeFile(t, filepath.Join(ra, "X.md"), "A\n")
	if _, _, err := a.inDir(ra, "commit", "-m", "A"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.inDir(ra, "sync"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rb, "Y.md"), "B\n")
	if _, _, err := b.inDir(rb, "commit", "-m", "B"); err != nil {
		t.Fatal(err)
	}
	out, _, err := b.inDir(rb, "resolve")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Merged 1 local and 1 remote commit(s)", "auto-merged: X.md, Y.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("resolve output missing %q:\n%s", want, out)
		}
	}
}

// DN10: status is written only by mark; info ack clears any flagged field.
func TestInfoAckAndStatusRemovedFromInfoSet(t *testing.T) {
	setupRecords(t)
	id := newRecord(t, "Flagged", "flagged")
	hubDir := filepath.Join(os.Getenv("TASKTREE_HOME"), "git")
	path := filepath.Join(hubDir, "tasks", id+".md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), "status: incoming", "status: incoming\nconflict: [status, mystery, title]", 1))
	if err = os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, hubDir, "add", ".")
	gitRun(t, hubDir, "commit", "-m", "fixture")

	_, _, err = run(t, "info", "set", "status", "active", "-t", "flagged")
	if code := codeOf(err); code != 2 || !strings.Contains(err.Error(), "tasktree mark <status>") {
		t.Fatalf("info set status code=%d err=%v", code, err)
	}
	if got := taskInfo(t, "flagged"); got.Status != "incoming" {
		t.Fatalf("info set status changed the status to %s", got.Status)
	}
	if _, _, err = run(t, "info", "ack", "mystery", "-t", "flagged"); err != nil {
		t.Fatalf("ack unknown key: %v", err)
	}
	if _, _, err = run(t, "info", "ack", "status", "-t", "flagged"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, "flagged"); strings.Join(got.Conflict, ",") != "title" {
		t.Fatalf("conflict=%v", got.Conflict)
	}
	expectCode(t, 3, "not flagged", "info", "ack", "status", "-t", "flagged")
	expectCode(t, 3, "not flagged", "info", "ack", "nonsense", "-t", "flagged")
	// info set <field> still clears its own flag.
	if _, _, err = run(t, "info", "set", "title", "Flagged again", "-t", "flagged"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, "flagged"); len(got.Conflict) != 0 || got.Title != "Flagged again" {
		t.Fatalf("after set: %+v", got)
	}
}

// DN1: alias add/rm/list with V1 semantics.
func TestAliasCommands(t *testing.T) {
	setupRecords(t)
	if out, _, err := run(t, "alias", "list", "--json"); err != nil || out != "[]\n" {
		t.Fatalf("empty list: %v %q", err, out)
	}
	if _, _, err := run(t, "alias", "add", "api", "git@example.com:org/api.git"); err != nil {
		t.Fatal(err)
	}
	// Same alias → same URL is a no-op.
	out, _, err := run(t, "alias", "add", "api", "git@example.com:org/api.git")
	if err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("repeat add: %v %q", err, out)
	}
	// Alias used by another URL → conflict (5).
	expectCode(t, 5, "already used", "alias", "add", "api", "git@example.com:org/other.git")
	expectCode(t, 2, "", "alias", "add", "a/b", "git@example.com:org/x.git")
	expectCode(t, 3, "not found", "alias", "rm", "missing")
	if _, _, err = run(t, "alias", "add", "web", "git@example.com:org/web.git"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, "alias", "add", "api2", "git@example.com:org/api.git"); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "alias", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"alias":"api","url":"git@example.com:org/api.git"},{"alias":"api2","url":"git@example.com:org/api.git"},{"alias":"web","url":"git@example.com:org/web.git"}]` + "\n"
	if out != want {
		t.Fatalf("list --json:\n%s\nwant\n%s", out, want)
	}
	if _, _, err = run(t, "alias", "rm", "api2"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run(t, "alias", "list")
	if strings.Contains(out, "api2") || !strings.Contains(out, "ALIAS") || !strings.Contains(out, "web") {
		t.Fatalf("list:\n%s", out)
	}
	b, err := os.ReadFile(filepath.Join(os.Getenv("TASKTREE_HOME"), "aliases.yml"))
	if err != nil || strings.Contains(string(b), "api2") {
		t.Fatalf("aliases.yml: %v\n%s", err, b)
	}
	// An alias resolves for add.
	o, _, _ := run(t, "alias", "list", "--json")
	if !strings.HasPrefix(o, "[") {
		t.Fatal(o)
	}
}

// DN2: list --ref filters by exact typed ref, repeatable, AND.
func TestListRefFilter(t *testing.T) {
	setupRecords(t)
	newRecord(t, "One", "one", "--jira", "ABC-1")
	newRecord(t, "Two", "two", "--jira", "ABC-2")
	newRecord(t, "Three", "three")
	if _, _, err := run(t, "ref", "add", "pr", "org/repo#7", "-t", "one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "ref", "add", "url", "https://example.com/a,b?x=1", "-t", "two"); err != nil {
		t.Fatal(err)
	}
	handles := func(args ...string) string {
		t.Helper()
		out, _, err := run(t, append([]string{"list", "--json"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		var rows []struct{ Handle string }
		if err = json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		var hs []string
		for _, r := range rows {
			hs = append(hs, r.Handle)
		}
		return strings.Join(hs, ",")
	}
	for _, c := range []struct {
		want string
		args []string
	}{
		{"one", []string{"--ref", "jira:ABC-1"}},
		{"one", []string{"--ref", "jira:ABC-1", "--ref", "pr:org/repo#7"}},
		{"", []string{"--ref", "jira:ABC-2", "--ref", "pr:org/repo#7"}},
		{"two", []string{"--ref", "url:https://example.com/a,b?x=1"}},
		{"", []string{"--ref", "jira:ABC"}}, // exact, not prefix
		{"", []string{"--ref", "pr:ABC-1"}}, // type matters
	} {
		if got := handles(c.args...); got != c.want {
			t.Errorf("%v → %q, want %q", c.args, got, c.want)
		}
	}
	for _, bad := range []string{"ABC-1", "jira:", "ticket:ABC-1", ":x"} {
		expectCode(t, 2, "invalid --ref", "list", "--ref", bad)
	}
}

const fileTemplate = `apiVersion: tasktree.dev/v2
kind: Template
metadata:
  name: from-file
  description: A template loaded from a path
parameters:
  - {name: who, default: world}
task:
  title: "Hello {{who}}"
  handle: "hello-{{who}}"
`

// DN7: a path separator or a .yml/.yaml suffix means "load this file";
// anything else is a name looked up in user templates, then builtins.
func TestTemplateFileVersusName(t *testing.T) {
	setupRecords(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tpl.yml"), fileTemplate)
	writeFile(t, filepath.Join(dir, "tpl.yaml"), strings.Replace(fileTemplate, "from-file", "from-yaml", 1))
	writeFile(t, filepath.Join(dir, "noext"), strings.Replace(fileTemplate, "from-file", "from-noext", 1))
	writeFile(t, filepath.Join(os.Getenv("TASKTREE_HOME"), "templates", "mine.yml"), strings.Replace(fileTemplate, "from-file", "mine", 1))

	for _, ref := range []string{filepath.Join(dir, "tpl.yml"), filepath.Join(dir, "tpl.yaml"), filepath.Join(dir, "noext")} {
		if out, _, err := run(t, "template", "show", ref); err != nil || !strings.Contains(out, "who") {
			t.Fatalf("show %s: %v\n%s", ref, err, out)
		}
		if out, _, err := run(t, "template", "validate", ref); err != nil || !strings.Contains(out, "is valid") {
			t.Fatalf("validate %s: %v\n%s", ref, err, out)
		}
	}
	out, _, err := run(t, "new", "--template", filepath.Join(dir, "noext"), "who=path")
	if err != nil || !strings.HasPrefix(out, "hello-path ") {
		t.Fatalf("new --template path: %v %q", err, out)
	}
	// A relative file name ending in .yml is a file, even in the cwd.
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, "new", "--template", "tpl.yml", "who=cwd"); err != nil || !strings.HasPrefix(out, "hello-cwd ") {
		t.Fatalf("new --template tpl.yml: %v %q", err, out)
	}
	// A bare name is discovered (user templates, then builtins), never a cwd file.
	if out, _, err = run(t, "new", "--template", "mine", "who=name"); err != nil || !strings.HasPrefix(out, "hello-name ") {
		t.Fatalf("new --template mine: %v %q", err, out)
	}
	expectCode(t, 3, "not found", "new", "--template", "noext", "who=x")
	expectCode(t, 3, "not found", "new", "--template", "./missing.yml")
}

// DN3: template list/show --json with stable shapes.
func TestTemplateJSON(t *testing.T) {
	setupRecords(t)
	out, _, err := run(t, "template", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err = json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	names := []string{}
	for _, r := range rows {
		names = append(names, r["name"])
		if len(r) != 3 || r["source"] != "builtin" || r["description"] == "" {
			t.Fatalf("row shape: %v", r)
		}
	}
	if strings.Join(names, ",") != "bugfix,feature,investigation" {
		t.Fatalf("names=%v", names)
	}
	out, _, err = run(t, "template", "show", "bugfix", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var detail struct {
		Name, Description, Source string
		Parameters                []map[string]any
		Raw                       string
	}
	if err = json.Unmarshal([]byte(out), &detail); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if detail.Name != "bugfix" || detail.Source != "builtin" || len(detail.Parameters) != 3 || !strings.Contains(detail.Raw, "kind: Template") {
		t.Fatalf("detail: %+v", detail)
	}
	for _, key := range []string{"name", "required", "default", "description"} {
		if _, ok := detail.Parameters[0][key]; !ok {
			t.Fatalf("parameter missing %q: %v", key, detail.Parameters[0])
		}
	}
	expectCode(t, 3, "not found", "template", "show", "nope", "--json")
}

// M28: checkout add and definition add -t share one K2 resolver, so every
// flag combination records the same .gitmodules and gitlink on both paths.
func TestK2ResolverIsIdenticalForCheckoutAndDefinition(t *testing.T) {
	home, tasks, origin := setupDefinitionTest(t)
	work := filepath.Join(t.TempDir(), "work")
	gitRun(t, "", "clone", origin, work)
	gitRun(t, work, "config", "user.email", "t@example.com")
	gitRun(t, work, "config", "user.name", "T")
	v1 := gitRun(t, work, "rev-parse", "HEAD")
	gitRun(t, work, "tag", "v1")
	writeFile(t, filepath.Join(work, "next"), "next\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-m", "next")
	gitRun(t, work, "push", "origin", "main")
	gitRun(t, work, "checkout", "-b", "develop")
	writeFile(t, filepath.Join(work, "dev"), "dev\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-m", "develop")
	gitRun(t, work, "push", "origin", "develop", "v1")

	hubDir := filepath.Join(home, "git")
	cases := []struct {
		name string
		args []string
	}{
		{"default", nil},
		{"new branch", []string{"--branch", "feature/x"}},
		{"existing branch", []string{"--branch", "develop"}},
		{"existing default branch", []string{"--branch", "main"}},
		{"new branch from tag", []string{"--branch", "hotfix", "--from", "v1"}},
		{"new branch from branch", []string{"--branch", "off-dev", "--from", "develop"}},
		{"existing branch ignores from", []string{"--branch", "develop", "--from", "v1"}},
		{"from remote branch", []string{"--from", "develop"}},
		{"from default branch name", []string{"--from", "main"}},
		{"from tag", []string{"--from", "v1"}},
		{"from sha", []string{"--from", strings.TrimSpace(v1)}},
		{"mode pin on default", []string{"--mode", "pin"}},
		{"mode follow with branch", []string{"--branch", "develop", "--mode", "follow"}},
		{"mode pin with branch", []string{"--branch", "develop", "--mode", "pin"}},
		{"mode work from branch", []string{"--from", "develop", "--mode", "work"}},
		{"mode follow from tag fails", []string{"--from", "v1", "--mode", "follow"}},
		{"mode work from tag fails", []string{"--from", "v1", "--mode", "work"}},
		{"invalid mode fails", []string{"--mode", "sideways"}},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handle := "k2-" + strings.Repeat("x", i%3) + string(rune('a'+i))
			checkoutID := newID(t, handle+" checkout")
			defID := newID(t, handle+" definition")
			rootOut, _, err := run(t, "open", checkoutID, "--path", filepath.Join(tasks, handle))
			if err != nil {
				t.Fatal(err)
			}
			root := strings.TrimSpace(rootOut)
			base := []string{"add", origin, "--name", "repo"}
			_, _, errCheckout := run(t, append(append([]string{}, base...), append(c.args, "-t", checkoutID)...)...)
			// Force the definition path: a task without a checkout.
			_, _, errDef := run(t, append(append([]string{}, base...), append(c.args, "-t", defID)...)...)
			if (errCheckout == nil) != (errDef == nil) {
				t.Fatalf("paths disagree: checkout=%v definition=%v", errCheckout, errDef)
			}
			if errCheckout != nil {
				if codeOf(errCheckout) != 2 || codeOf(errDef) != 2 {
					t.Fatalf("want usage errors, got checkout=%v definition=%v", errCheckout, errDef)
				}
				return
			}
			_ = root
			gm := func(id string) string {
				return gitRun(t, hubDir, "show", "task/"+id+":.gitmodules")
			}
			link := func(id string) string {
				return gitRun(t, hubDir, "rev-parse", "task/"+id+":repo")
			}
			if a, b := gm(checkoutID), gm(defID); a != b {
				t.Fatalf(".gitmodules differ\ncheckout:\n%s\ndefinition:\n%s", a, b)
			}
			if a, b := link(checkoutID), link(defID); a != b {
				t.Fatalf("gitlinks differ: checkout=%s definition=%s", a, b)
			}
		})
	}
}
