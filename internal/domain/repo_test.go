package domain_test

import (
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
)

func TestDeriveRepoName(t *testing.T) {
	tests := map[string]string{
		"git@github.com:myorg/api.git":        "api",
		"https://github.com/myorg/web.git":    "web",
		"ssh://git@github.com/myorg/core.git": "core",
	}

	for input, want := range tests {
		got, err := domain.DeriveRepoName(input)
		if err != nil {
			t.Fatalf("derive repo name for %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("derive repo name for %q = %q, want %q", input, got, want)
		}
	}
}

func TestValidateRepoNameRejectsUnsafeValues(t *testing.T) {
	for _, input := range []string{"", ".", "..", "a/b", `a\\b`} {
		if err := domain.ValidateRepoName(input); err == nil {
			t.Fatalf("expected invalid name error for %q", input)
		}
	}
}
