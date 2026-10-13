// Package schema_test checks the JSON Schemas in /schema against what the
// tool itself produces (M42).
package schema_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tinmancoding/tasktree/v2/internal/app"
	"github.com/tinmancoding/tasktree/v2/internal/bootstrap"
	"github.com/tinmancoding/tasktree/v2/internal/record"
	"github.com/tinmancoding/tasktree/v2/internal/template"
	"gopkg.in/yaml.v3"
)

func compile(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "schema", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err = c.AddResource(name, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(name)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}
	return s
}

// jsonValue decodes YAML into plain JSON types (timestamps become RFC 3339 strings).
func jsonValue(t *testing.T, y []byte) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal(y, &v); err != nil {
		t.Fatalf("yaml: %v\n%s", err, y)
	}
	return normalize(v)
}

func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case int:
		return float64(x)
	}
	return v
}

func frontmatter(t *testing.T, rendered []byte) []byte {
	t.Helper()
	s := string(rendered)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("no frontmatter:\n%s", s)
	}
	end := strings.Index(s[4:], "\n---")
	return []byte(s[4 : 4+end])
}

func TestBuiltinTemplatesValidate(t *testing.T) {
	schema := compile(t, "template.schema.json")
	all, err := template.New(t.TempDir()).List()
	if err != nil || len(all) == 0 {
		t.Fatalf("templates: %v (%d)", err, len(all))
	}
	for _, s := range all {
		if err := schema.Validate(jsonValue(t, s.Raw)); err != nil {
			t.Errorf("builtin %s: %v", s.Metadata.Name, err)
		}
	}
}

func TestTemplateSchemaRejectsV1AndUnknownKeys(t *testing.T) {
	schema := compile(t, "template.schema.json")
	for name, doc := range map[string]string{
		"v1 apiVersion":    "apiVersion: tasktree.dev/v1\nkind: Template\nmetadata: {name: x}\ntask: {title: x}\n",
		"v1 template key":  "apiVersion: tasktree.dev/v2\nkind: Template\nmetadata: {name: x}\ntemplate: {}\n",
		"unknown task key": "apiVersion: tasktree.dev/v2\nkind: Template\nmetadata: {name: x}\ntask: {titel: x}\n",
		"missing name":     "apiVersion: tasktree.dev/v2\nkind: Template\nmetadata: {}\ntask: {title: x}\n",
	} {
		if schema.Validate(jsonValue(t, []byte(doc))) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRenderedRecordFrontmatterValidates(t *testing.T) {
	schema := compile(t, "task-record.schema.json")
	parent := "k7x2mq"
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	d, err := record.New(record.Task{
		ID: "p3n8wd", Handle: "DSA-8946", Title: "AI board member: prod admin", Status: "active", Parent: &parent,
		Labels: []string{"aibm", "x.y-z"}, CreatedAt: now, UpdatedAt: now,
		Refs: []record.Ref{{Type: "jira", Value: "DSA-8946"}, {Type: "pr", Value: "DiligentCorp/boardroom-ai#123"}, {Type: "url", Value: "https://example.com/a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(jsonValue(t, frontmatter(t, d.Render()))); err != nil {
		t.Fatalf("rendered record: %v\n%s", err, d.Render())
	}
	// A flagged record with an unknown key still validates.
	flagged := bytes.Replace(d.Render(), []byte("status: active"), []byte("status: active\nconflict: [status, mystery]\nextra: kept"), 1)
	if err = schema.Validate(jsonValue(t, frontmatter(t, flagged))); err != nil {
		t.Fatalf("flagged record: %v", err)
	}
	for name, bad := range map[string]string{
		"status":  "status: archived",
		"id":      "id: ABCDEF",
		"ref key": "refs:\n  - ticket: X-1",
		"handle":  "handle: has space",
	} {
		key := strings.SplitN(bad, ":", 2)[0]
		doc := string(frontmatter(t, d.Render()))
		lines := []string{}
		for _, l := range strings.Split(doc, "\n") {
			if strings.HasPrefix(l, key+":") {
				l = bad
			}
			lines = append(lines, l)
		}
		if schema.Validate(jsonValue(t, []byte(strings.Join(lines, "\n")))) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGeneratedConfigValidates(t *testing.T) {
	schema := compile(t, "tasktree-config.schema.json")
	for name, cfg := range map[string]app.TasktreeConfig{
		"marker only": {APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: "k7x2mq"},
		"bootstrap": {APIVersion: "tasktree.dev/v2", Kind: "TasktreeConfig", TaskID: "k7x2mq", Bootstrap: []bootstrap.Step{
			{Name: "deps", Run: "npm ci", Workdir: "api", Env: map[string]string{"CI": "1"}},
			{Name: "plain", Run: "true"},
		}},
	} {
		b, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err = schema.Validate(jsonValue(t, b)); err != nil {
			t.Errorf("%s: %v\n%s", name, err, b)
		}
	}
	for name, doc := range map[string]string{
		"wrong kind":   "apiVersion: tasktree.dev/v2\nkind: Tasktree\ntaskId: k7x2mq\n",
		"bad task id":  "apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: NOPE\n",
		"unknown key":  "apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: k7x2mq\nsources: []\n",
		"bad env key":  "apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: k7x2mq\nbootstrap:\n  - {name: a, run: b, env: {\"1X\": v}}\n",
		"step no name": "apiVersion: tasktree.dev/v2\nkind: TasktreeConfig\ntaskId: k7x2mq\nbootstrap:\n  - {run: b}\n",
	} {
		if schema.Validate(jsonValue(t, []byte(doc))) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestV1SchemaRemoved(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "schema", "tasktree.schema.json")); !os.IsNotExist(err) {
		t.Fatalf("V1 Tasktree.yml schema must be deleted: %v", err)
	}
}
