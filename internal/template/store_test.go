package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsAndUserOverride(t *testing.T) {
	d := t.TempDir()
	s := New(d)
	all, err := s.List()
	if err != nil || len(all) != 3 {
		t.Fatalf("builtins=%d err=%v", len(all), err)
	}
	raw := []byte("apiVersion: tasktree.dev/v2\nkind: Template\nmetadata: {name: bugfix, description: override}\ntask: {title: custom}\n")
	if err = os.WriteFile(filepath.Join(d, "override.yml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	v, err := s.Load("bugfix")
	if err != nil || v.Source != "user" || v.Metadata.Description != "override" {
		t.Fatalf("%+v %v", v, err)
	}
	if err = Validate(v); err != nil {
		t.Fatal(err)
	}
}
func TestValidateUndeclaredVariable(t *testing.T) {
	s := Spec{APIVersion: "tasktree.dev/v2", Kind: "Template", Metadata: Metadata{Name: "bad"}, Task: Task{Title: "{{missing}}"}}
	if err := Validate(s); err == nil {
		t.Fatal("expected error")
	}
}

// A flow-mapping description with commas must be quoted, or YAML splits it
// into stray keys (found by the M42 schema test).
func TestInvestigationRefParameterDescription(t *testing.T) {
	s, err := New(t.TempDir()).Load("investigation")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Parameters {
		if p.Name == "ref" {
			if p.Description != "Branch, tag, or commit" {
				t.Fatalf("description = %q", p.Description)
			}
			return
		}
	}
	t.Fatal("ref parameter missing")
}
