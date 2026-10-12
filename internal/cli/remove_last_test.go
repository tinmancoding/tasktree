package cli_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Removing the last submodule leaves an empty .gitmodules; info must still work.
func TestInfoAfterRemovingLastSubmodule(t *testing.T) {
	f := newCloseFixture(t)
	_, _, root := createAndOpen(t, "Remove last", filepath.Join(f.tasks, "remove-last"))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "add", f.upstream, "--name", "api"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "remove", "api"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "info"); err != nil {
		t.Fatalf("info after removing last submodule: %v", err)
	}
	if _, _, err := run(t, "info", "--json"); err != nil {
		t.Fatalf("info --json after removing last submodule: %v", err)
	}
}
