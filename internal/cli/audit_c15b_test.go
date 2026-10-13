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

// M3: new --parent --isolated forwards --repos and --branch to the open.
func TestNewIsolatedAcceptsReposAndBranch(t *testing.T) {
	f := newCloseFixture(t)
	parent, _, root := createAndOpen(t, "Iso flags parent", filepath.Join(f.tasks, "iso-flags"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", f.upstream, "--name", "work", "--branch", "feature"}, {"add", f.upstream, "--name", "ref"}, {"add", f.upstream, "--name", "other", "--branch", "other"}} {
		if _, _, err := run(t, args...); err != nil {
			t.Fatal(err)
		}
	}
	expectCode(t, 2, "require --isolated", "new", "Nope", "--parent", parent, "--repos", "work")
	expectCode(t, 2, "require --isolated", "new", "Nope", "--parent", parent, "--branch", "x")
	out, _, err := run(t, "new", "Iso child", "--handle", "iso-flags-child", "--parent", parent, "--isolated", "--repos", "work,ref", "--branch", "custom/child")
	if err != nil {
		t.Fatal(err)
	}
	childRoot := strings.TrimSpace(out)
	if _, err = os.Stat(filepath.Join(childRoot, "other")); !os.IsNotExist(err) {
		t.Fatalf("--repos did not limit the repositories: %v", err)
	}
	if _, err = os.Stat(filepath.Join(childRoot, "ref", ".git")); err != nil {
		t.Fatalf("selected follow repo missing: %v", err)
	}
	if got := gitRun(t, filepath.Join(childRoot, "work"), "branch", "--show-current"); got != "custom/child" {
		t.Fatalf("--branch ignored: %q", got)
	}
}

// M4: --jira/--handle/--status override a template's rendered task fields;
// --isolated is not allowed with --template.
func TestNewTemplateOverrides(t *testing.T) {
	setupRecords(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.yml"), `apiVersion: tasktree.dev/v2
kind: Template
metadata: {name: plain}
parameters:
  - {name: n, default: base}
task:
  title: "Plain {{n}}"
  handle: "plain-{{n}}"
  jira: "TPL-1"
  status: active
`)
	tpl := filepath.Join(dir, "plain.yml")
	out, _, err := run(t, "new", "--template", tpl, "--handle", "custom", "--jira", "OVR-5", "--status", "blocked")
	if err != nil || !strings.HasPrefix(out, "custom ") {
		t.Fatalf("overrides: %v %q", err, out)
	}
	got := taskInfo(t, "custom")
	if got.Status != "blocked" || len(got.Refs) != 1 || got.Refs[0].Value != "OVR-5" {
		t.Fatalf("task: %+v", got)
	}
	// Without flags the template values stand (the --status default is not an override).
	if out, _, err = run(t, "new", "--template", tpl, "n=two"); err != nil || !strings.HasPrefix(out, "plain-two ") {
		t.Fatalf("defaults: %v %q", err, out)
	}
	if got = taskInfo(t, "plain-two"); got.Status != "active" || got.Refs[0].Value != "TPL-1" {
		t.Fatalf("template values lost: %+v", got)
	}
	expectCode(t, 2, "", "new", "--template", tpl, "n=bad", "--status", "sideways")
	expectCode(t, 2, "", "new", "--template", tpl, "n=bad", "--handle", "bad handle")
	expectCode(t, 2, "", "new", "--template", tpl, "n=bad", "--jira", "bad key")
	expectCode(t, 2, "cannot be used with --template", "new", "--template", tpl, "n=bad", "--parent", "plain-two", "--isolated")
}

// DN4: bootstrap add/rm/list on a checkout (structural commit + hub push) and
// on a definition without a checkout (plumbing write); run still works.
func TestBootstrapAddRmListBothPaths(t *testing.T) {
	f := newCloseFixture(t)
	_, id, root := createAndOpen(t, "Boot", filepath.Join(f.tasks, "boot"))
	hubDir := filepath.Join(f.home, "git")
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "bootstrap", "add", "greet", "echo hi > greeted && echo \"$WHO\" > who", "--env", "WHO=world", "--env", "EQ=a=b"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "bootstrap", "add", "second", "true"); err != nil {
		t.Fatal(err)
	}
	cfg := gitRun(t, hubDir, "show", "task/"+id+":.tasktree/config.yml")
	if !strings.Contains(cfg, "name: greet") || !strings.Contains(cfg, "EQ: a=b") || !strings.Contains(cfg, "taskId: "+id) {
		t.Fatalf("hub config:\n%s", cfg)
	}
	if subject := gitRun(t, hubDir, "log", "-1", "--format=%s", "task/"+id); !strings.Contains(subject, "bootstrap add second") {
		t.Fatalf("subject=%q", subject)
	}
	out, _, err := run(t, "bootstrap", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var steps []map[string]any
	if err = json.Unmarshal([]byte(out), &steps); err != nil || len(steps) != 2 || steps[0]["name"] != "greet" || steps[1]["workdir"] != "" {
		t.Fatalf("list: %v %s", err, out)
	}
	if env, _ := steps[0]["env"].(map[string]any); env["WHO"] != "world" {
		t.Fatalf("env: %s", out)
	}
	// Both the bare command and `run` execute the steps.
	for _, args := range [][]string{{"bootstrap"}, {"bootstrap", "run"}} {
		os.Remove(filepath.Join(root, "greeted"))
		if _, _, err = run(t, args...); err != nil {
			t.Fatal(err)
		}
		if b, e := os.ReadFile(filepath.Join(root, "who")); e != nil || strings.TrimSpace(string(b)) != "world" {
			t.Fatalf("%v: %v %q", args, e, b)
		}
	}
	// Validation.
	expectCode(t, 2, "already exists", "bootstrap", "add", "greet", "true")
	expectCode(t, 2, "inside the tasktree root", "bootstrap", "add", "x", "true", "--workdir", "../out")
	expectCode(t, 2, "relative", "bootstrap", "add", "x", "true", "--workdir", "/tmp")
	expectCode(t, 2, "invalid --env", "bootstrap", "add", "x", "true", "--env", "1BAD=v")
	expectCode(t, 2, "invalid --env", "bootstrap", "add", "x", "true", "--env", "NOEQUALS")
	expectCode(t, 3, "not found", "bootstrap", "rm", "missing")
	if _, _, err = run(t, "bootstrap", "rm", "second"); err != nil {
		t.Fatal(err)
	}
	if cfg = gitRun(t, hubDir, "show", "task/"+id+":.tasktree/config.yml"); strings.Contains(cfg, "second") {
		t.Fatalf("rm not committed:\n%s", cfg)
	}

	// Definition path: no checkout, -t required.
	if err = os.Chdir(f.base); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "new", "Defined boot", "--handle", "defined-boot")
	if err != nil {
		t.Fatal(err)
	}
	did := strings.Fields(out)[1]
	expectCode(t, 4, "", "bootstrap", "add", "x", "true")
	if _, _, err = run(t, "bootstrap", "add", "deps", "make deps", "--workdir", "api", "-t", "defined-boot"); err != nil {
		t.Fatal(err)
	}
	if cfg = gitRun(t, hubDir, "show", "task/"+did+":.tasktree/config.yml"); !strings.Contains(cfg, "name: deps") || !strings.Contains(cfg, "workdir: api") {
		t.Fatalf("definition config:\n%s", cfg)
	}
	if out, _, err = run(t, "bootstrap", "list", "-t", "defined-boot"); err != nil || !strings.Contains(out, "deps") || !strings.Contains(out, "make deps") {
		t.Fatalf("definition list: %v %s", err, out)
	}
	if _, _, err = run(t, "bootstrap", "rm", "deps", "-t", "defined-boot"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, "bootstrap", "list", "-t", "defined-boot", "--json"); out != "[]\n" {
		t.Fatalf("empty list: %q", out)
	}
	// Opening materializes the definition's steps (workdir must exist, so use none).
	if _, _, err = run(t, "bootstrap", "add", "mark", "echo ok > opened", "-t", "defined-boot"); err != nil {
		t.Fatal(err)
	}
	opened, _, err := run(t, "open", "defined-boot", "--path", filepath.Join(f.tasks, "defined-boot"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(strings.TrimSpace(opened), "opened")); err != nil {
		t.Fatalf("definition bootstrap step did not run on open: %v", err)
	}
}

// DN6: open on a live checkout only prints its path; it fetches and merges nothing.
func TestOpenLiveCheckoutDoesNotFetch(t *testing.T) {
	a, b, _ := twoMachines(t)
	a.must("new", "Live", "--handle", "live")
	a.must("sync")
	rb := strings.TrimSpace(b.must("open", "live"))
	// Machine A changes the record and publishes it.
	a.must("note", "from A", "-t", "live")
	a.must("sync")

	hub := filepath.Join(b.home, "git")
	refs := func() string {
		return gitRun(t, hub, "for-each-ref", "--format=%(refname) %(objectname)")
	}
	before := refs()
	out, errOut, err := b.run("open", "live")
	if err != nil || strings.TrimSpace(out) != rb {
		t.Fatalf("open: %v out=%q err=%q", err, out, errOut)
	}
	if after := refs(); after != before {
		t.Fatalf("open on a live checkout touched hub refs:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if got := b.must("info", "-t", "live"); strings.Contains(got, "from A") {
		t.Fatalf("open merged remote state:\n%s", got)
	}
	// sync is what brings the note in.
	b.must("sync", "--all")
	if got := b.must("info", "-t", "live"); !strings.Contains(got, "from A") {
		t.Fatalf("sync did not merge:\n%s", got)
	}
}
