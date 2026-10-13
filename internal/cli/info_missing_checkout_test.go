package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// info -t must still show the record when the recorded checkout folder is gone.
func TestInfoWithMissingCheckout(t *testing.T) {
	f := newCloseFixture(t)
	handle, _, root := createAndOpen(t, "Gone", filepath.Join(f.tasks, "gone"))
	if err := os.Chdir(f.base); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := run(t, "info", "-t", handle)
	if err != nil {
		t.Fatalf("info with missing checkout: %v %s", err, errOut)
	}
	if !strings.Contains(out, "Handle: "+handle) || !strings.Contains(errOut, "is missing") {
		t.Fatalf("out=%q err=%q", out, errOut)
	}
	if _, _, err := run(t, "info", "-t", handle, "--json"); err != nil {
		t.Fatalf("info --json with missing checkout: %v", err)
	}
}
