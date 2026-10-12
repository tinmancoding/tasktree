package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfoEditEditorAndStdin(t *testing.T) {
	setupRecords(t)
	newRecord(t, "Description", "description")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'edited by fake editor' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	t.Setenv("VISUAL", "")
	if _, _, err := run(t, "info", "edit", "-t", "description"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, "description").Description; got != "edited by fake editor" {
		t.Fatalf("editor description=%q", got)
	}
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'edited by fake editor\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "info", "edit", "-t", "description")
	if err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("newline-only editor save output=%q err=%v", out, err)
	}
	input := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(input, []byte("from stdin"), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = in
	t.Cleanup(func() { os.Stdin = old; _ = in.Close() })
	if _, _, err := run(t, "info", "edit", "--stdin", "-t", "description"); err != nil {
		t.Fatal(err)
	}
	if got := taskInfo(t, "description").Description; got != "from stdin" {
		t.Fatalf("stdin description=%q", got)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	out, _, err = run(t, "info", "edit", "--stdin", "-t", "description")
	if err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("unchanged output=%q err=%v", out, err)
	}
}
