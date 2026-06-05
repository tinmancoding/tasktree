package app

import (
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/internal/domain"
)

func TestRenderTemplateRendersBootstrap(t *testing.T) {
	tmpl := domain.TemplateSpec{
		Template: domain.TasktreeTemplate{
			Metadata: domain.SpecMetadata{Name: "{{ticket}}"},
			Spec: domain.WorkspaceSpec{
				Bootstrap: []domain.BootstrapStep{
					{
						Name:    "gen-{{ticket}}",
						Run:     "./gen.sh --ticket {{ticket}}",
						Workdir: "{{ticket}}",
						Env:     map[string]string{"TICKET": "{{ticket}}"},
					},
				},
			},
		},
	}
	values := map[string]string{"ticket": "BUG-1"}

	spec := renderTemplate(tmpl, values, "ws", time.Now())

	if len(spec.Spec.Bootstrap) != 1 {
		t.Fatalf("expected 1 bootstrap step, got %d", len(spec.Spec.Bootstrap))
	}
	b := spec.Spec.Bootstrap[0]
	if b.Name != "gen-BUG-1" {
		t.Fatalf("name not rendered: %q", b.Name)
	}
	if b.Run != "./gen.sh --ticket BUG-1" {
		t.Fatalf("run not rendered: %q", b.Run)
	}
	if b.Workdir != "BUG-1" {
		t.Fatalf("workdir not rendered: %q", b.Workdir)
	}
	if b.Env["TICKET"] != "BUG-1" {
		t.Fatalf("env not rendered: %q", b.Env["TICKET"])
	}
}

func TestExtractAllRefsIncludesBootstrap(t *testing.T) {
	tmpl := domain.TemplateSpec{
		Template: domain.TasktreeTemplate{
			Spec: domain.WorkspaceSpec{
				Bootstrap: []domain.BootstrapStep{
					{Name: "deps", Run: "echo {{token}}", Env: map[string]string{"{{envkey}}": "{{envval}}"}},
				},
			},
		},
	}
	refs := extractAllRefs(tmpl)
	got := map[string]bool{}
	for _, r := range refs {
		got[r.Name] = true
	}
	for _, want := range []string{"token", "envkey", "envval"} {
		if !got[want] {
			t.Fatalf("expected ref %q to be collected, got %v", want, got)
		}
	}
}
