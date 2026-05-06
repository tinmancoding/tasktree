package variable_test

import (
	"testing"

	"github.com/tinmancoding/tasktree/internal/variable"
)

// ---------------------------------------------------------------------------
// Parser tests
// ---------------------------------------------------------------------------

func TestParse_SimpleRef(t *testing.T) {
	refs := variable.Parse("Hello {{name}}")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	r := refs[0]
	if r.Name != "name" {
		t.Errorf("Name = %q, want %q", r.Name, "name")
	}
	if r.HasDefault {
		t.Error("HasDefault should be false")
	}
	if r.Raw != "{{name}}" {
		t.Errorf("Raw = %q, want %q", r.Raw, "{{name}}")
	}
}

func TestParse_WithDefault(t *testing.T) {
	refs := variable.Parse("{{base_branch | default:main}}")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	r := refs[0]
	if r.Name != "base_branch" {
		t.Errorf("Name = %q, want %q", r.Name, "base_branch")
	}
	if !r.HasDefault {
		t.Error("HasDefault should be true")
	}
	if r.Default != "main" {
		t.Errorf("Default = %q, want %q", r.Default, "main")
	}
}

func TestParse_Multiple(t *testing.T) {
	refs := variable.Parse("fix/{{ticket_number}} from {{base_branch | default:main}}")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Name != "ticket_number" {
		t.Errorf("refs[0].Name = %q, want ticket_number", refs[0].Name)
	}
	if refs[1].Name != "base_branch" {
		t.Errorf("refs[1].Name = %q, want base_branch", refs[1].Name)
	}
}

func TestParse_None(t *testing.T) {
	refs := variable.Parse("no variables here")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}

func TestParse_InvalidNameIgnored(t *testing.T) {
	// Capital letters and hyphens are not valid variable names — should not match.
	refs := variable.Parse("{{Invalid}} {{not-valid}}")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d (invalid names should not be parsed)", len(refs))
	}
}

// ---------------------------------------------------------------------------
// Resolver tests
// ---------------------------------------------------------------------------

func TestResolver_CLIArgsTakesPriority(t *testing.T) {
	cli := variable.CLIArgsSource{"ticket": "BUG-1"}
	defaults := variable.DefaultsSource{"ticket": "FALLBACK"}
	r := variable.NewResolver(cli, defaults)
	v, found := r.Resolve("ticket")
	if !found {
		t.Fatal("expected to find value")
	}
	if v != "BUG-1" {
		t.Errorf("value = %q, want %q", v, "BUG-1")
	}
}

func TestResolver_FallsBackToDefaults(t *testing.T) {
	cli := variable.CLIArgsSource{}
	defaults := variable.DefaultsSource{"base_branch": "main"}
	r := variable.NewResolver(cli, defaults)
	v, found := r.Resolve("base_branch")
	if !found {
		t.Fatal("expected to find value in defaults")
	}
	if v != "main" {
		t.Errorf("value = %q, want %q", v, "main")
	}
}

func TestResolver_NotFound(t *testing.T) {
	r := variable.NewResolver(variable.CLIArgsSource{})
	_, found := r.Resolve("missing")
	if found {
		t.Error("expected not found")
	}
}

func TestResolveAll_UsesInlineDefault(t *testing.T) {
	refs := variable.Parse("{{branch | default:main}}")
	r := variable.NewResolver() // no sources
	values := r.ResolveAll(refs)
	if v, ok := values["branch"]; !ok || v != "main" {
		t.Errorf("expected inline default \"main\", got %q (ok=%v)", v, ok)
	}
}

func TestResolveAll_ExplicitOverridesInlineDefault(t *testing.T) {
	refs := variable.Parse("{{branch | default:main}}")
	cli := variable.CLIArgsSource{"branch": "develop"}
	r := variable.NewResolver(cli)
	values := r.ResolveAll(refs)
	if v := values["branch"]; v != "develop" {
		t.Errorf("expected explicit value \"develop\", got %q", v)
	}
}

func TestParseKVArgs_Valid(t *testing.T) {
	src, err := variable.ParseKVArgs([]string{"ticket=BUG-1", "repo=@api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src["ticket"] != "BUG-1" {
		t.Errorf("ticket = %q, want BUG-1", src["ticket"])
	}
	if src["repo"] != "@api" {
		t.Errorf("repo = %q, want @api", src["repo"])
	}
}

func TestParseKVArgs_InvalidFormat(t *testing.T) {
	_, err := variable.ParseKVArgs([]string{"noequals"})
	if err == nil {
		t.Error("expected error for missing '='")
	}
}

func TestParseKVArgs_ValueWithEquals(t *testing.T) {
	// Value may itself contain '=' — only the first '=' is the separator.
	src, err := variable.ParseKVArgs([]string{"key=a=b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src["key"] != "a=b" {
		t.Errorf("key = %q, want a=b", src["key"])
	}
}

// ---------------------------------------------------------------------------
// Renderer tests
// ---------------------------------------------------------------------------

func TestRenderString_Simple(t *testing.T) {
	result := variable.RenderString("bugfix-{{ticket_number}}", map[string]string{
		"ticket_number": "BUG-123",
	})
	if result != "bugfix-BUG-123" {
		t.Errorf("result = %q, want bugfix-BUG-123", result)
	}
}

func TestRenderString_WithDefault(t *testing.T) {
	// When the variable IS in the map, the default clause is irrelevant.
	result := variable.RenderString("{{base_branch | default:main}}", map[string]string{
		"base_branch": "develop",
	})
	if result != "develop" {
		t.Errorf("result = %q, want develop", result)
	}
}

func TestRenderString_MissingVarLeftAsIs(t *testing.T) {
	// Missing variables are left untouched (caller validates completeness).
	result := variable.RenderString("{{missing}}", map[string]string{})
	if result != "{{missing}}" {
		t.Errorf("result = %q, want {{missing}}", result)
	}
}

func TestRenderString_NoVariables(t *testing.T) {
	result := variable.RenderString("plain string", map[string]string{})
	if result != "plain string" {
		t.Errorf("result = %q, want \"plain string\"", result)
	}
}

func TestRenderStringMap(t *testing.T) {
	m := map[string]string{
		"ticket": "{{ticket_number}}",
		"type":   "bugfix",
	}
	values := map[string]string{"ticket_number": "BUG-1"}
	result := variable.RenderStringMap(m, values)
	if result["ticket"] != "BUG-1" {
		t.Errorf("ticket = %q, want BUG-1", result["ticket"])
	}
	if result["type"] != "bugfix" {
		t.Errorf("type = %q, want bugfix", result["type"])
	}
}
