package cli_test

import (
	"os"
	"testing"
)

func TestM11NotFoundExitsThree(t *testing.T) {
	f := newCloseFixture(t)
	handle, id, root := createAndOpen(t, "Not found", f.tasks+"/nf")
	_ = id
	if _, _, err := run(t, "add", f.upstream, "--name", "app", "-t", handle); err != nil {
		t.Fatal(err)
	}
	other := newRecord(t, "Defined only", "defined-only")
	_ = other
	expectCode(t, 3, `repository "nope" not found`, "remove", "nope", "-t", handle)
	expectCode(t, 3, `repository "nope" not found`, "mode", "nope", "work", "-t", handle)
	expectCode(t, 3, `repository "nope" not found`, "push", "nope", "-t", handle)
	// A task without a checkout edits its definition instead.
	expectCode(t, 3, `repository "nope" not found`, "remove", "nope", "-t", "defined-only")
	expectCode(t, 3, `repository "nope" not found`, "mode", "nope", "work", "-t", "defined-only")
	expectCode(t, 3, `template "missing-template" not found`, "new", "x", "--template", "missing-template")
	expectCode(t, 3, `template "missing-template" not found`, "template", "show", "missing-template")
	expectCode(t, 3, "ref jira:ABC-1 not found on "+handle, "ref", "rm", "jira", "ABC-1", "-t", handle)
	_ = root
}

func TestM12UsageErrorsExitTwo(t *testing.T) {
	f := newCloseFixture(t)
	handle, _, _ := createAndOpen(t, "Usage", f.tasks+"/usage")
	expectCode(t, 2, "", "frobnicate")
	for _, group := range []string{"ref", "label", "template", "config"} {
		expectCode(t, 2, "unknown command", group, "frobnicate")
	}
	expectCode(t, 2, "invalid status", "new", "bad status", "--status", "bogus")
	expectCode(t, 2, "invalid status", "list", "--status", "bogus")
	expectCode(t, 2, "invalid note kind", "note", "text", "--kind", "bogus", "-t", handle)
	expectCode(t, 2, "invalid mode", "add", f.upstream, "--mode", "bogus", "-t", handle)
	expectCode(t, 2, "unknown config key", "config", "get", "nokey")
	expectCode(t, 2, "unknown config key", "config", "set", "nokey", "v")
}

func TestM13NoCheckoutExitsFour(t *testing.T) {
	setupRecords(t)
	handle := "far-away"
	newRecord(t, "Far away", handle)
	want := handle + " has no checkout on this machine; run tasktree open " + handle
	for _, args := range [][]string{
		{"commit", "-t", handle},
		{"push", "-t", handle},
		{"bootstrap", "-t", handle},
		{"move", os.TempDir() + "/never", "-t", handle},
		{"root", "-t", handle},
		{"resolve", "-t", handle},
		{"sync", "-t", handle},
	} {
		expectCode(t, 4, want, args...)
	}
}
