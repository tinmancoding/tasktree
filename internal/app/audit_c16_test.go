package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H2: a clone directory owned by an incomplete import must never be fixable.
func TestDoctorFixKeepsCloneOfIncompleteImport(t *testing.T) {
	a, root := importFixture(t)
	repo := filepath.Join(root, "repo")
	// Leave nothing at risk, so only the import journal protects the clone.
	importGit(t, repo, "checkout", "--", ".")
	importGit(t, repo, "clean", "-fdx")
	importGit(t, repo, "stash", "drop")
	before := treeDigest(t, root)
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "3")
	t.Setenv("TASKTREE_TEST_IMPORT_KILL", "1")
	if _, err := a.Import(context.Background(), root, ImportOptions{}); err == nil {
		t.Fatal("expected simulated interruption")
	}
	t.Setenv("TASKTREE_TEST_IMPORT_FAIL", "")
	t.Setenv("TASKTREE_TEST_IMPORT_KILL", "")
	entries, err := os.ReadDir(filepath.Join(a.Home.Root, "imports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("journal entries = %v %v", entries, err)
	}
	id := entries[0].Name()
	clone := a.Home.ClonePath(id)
	if _, err := os.Stat(clone); err != nil {
		t.Fatalf("clone missing before doctor: %v", err)
	}

	for _, fix := range []bool{false, true} {
		r, err := a.Doctor(context.Background(), fix)
		if err != nil {
			t.Fatal(err)
		}
		journal := false
		for _, f := range r.Findings {
			if f.Code == "orphan-clone" && f.Subject == id {
				t.Fatalf("fix=%v: import clone reported as orphan-clone: %+v", fix, f)
			}
			if f.Code == "import-journal" {
				journal = true
				if f.Fixable || f.Fixed || !strings.Contains(f.Message, "import --recover") {
					t.Fatalf("journal finding = %+v", f)
				}
			}
		}
		if !journal {
			t.Fatalf("fix=%v: no import-journal finding", fix)
		}
		if _, err := os.Stat(clone); err != nil {
			t.Fatalf("fix=%v: doctor removed the import clone: %v", fix, err)
		}
		if _, err := os.Stat(filepath.Join(a.Home.Root, "imports", id, "journal.json")); err != nil {
			t.Fatalf("fix=%v: journal gone: %v", fix, err)
		}
	}
	if _, err := a.RecoverImport(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if got := treeDigest(t, root); got != before {
		t.Fatalf("recovery changed fixture: %s != %s", got, before)
	}
}
