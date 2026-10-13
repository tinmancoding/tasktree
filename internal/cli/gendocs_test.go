package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestCLIReferenceUpToDate fails when docs/reference/cli no longer matches the
// command definitions.
func TestCLIReferenceUpToDate(t *testing.T) {
	committed := filepath.Join("..", "..", "docs", "reference", "cli")
	generated := t.TempDir()
	if err := GenerateCLIDocs(generated); err != nil {
		t.Fatal(err)
	}
	const hint = "CLI reference is out of date: run `go run ./cmd/tasktree gen-docs docs/reference/cli`"
	want, err := os.ReadDir(generated)
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadDir(committed)
	if err != nil {
		t.Fatalf("%v\n%s", err, hint)
	}
	names := map[string]bool{}
	for _, e := range want {
		names[e.Name()] = true
		a, err := os.ReadFile(filepath.Join(generated, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(committed, e.Name()))
		if err != nil {
			t.Errorf("%s is missing from docs/reference/cli\n%s", e.Name(), hint)
			continue
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from the generated page\n%s", e.Name(), hint)
		}
	}
	for _, e := range have {
		if !names[e.Name()] {
			t.Errorf("docs/reference/cli/%s has no matching command\n%s", e.Name(), hint)
		}
	}
}
