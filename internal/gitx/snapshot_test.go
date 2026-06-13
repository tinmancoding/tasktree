package gitx

import (
	"reflect"
	"testing"
)

func TestParseStatusZ(t *testing.T) {
	// modified (worktree), staged-add, untracked, deleted, rename.
	// rename emits two NUL fields: "R  new" then "old".
	data := " M modified.txt\x00" +
		"A  staged.txt\x00" +
		"?? untracked.txt\x00" +
		" D deleted.txt\x00" +
		"R  new.txt\x00old.txt\x00"

	got := parseStatusZ(data)
	want := []StatusEntry{
		{X: ' ', Y: 'M', Path: "modified.txt"},
		{X: 'A', Y: ' ', Path: "staged.txt"},
		{X: '?', Y: '?', Path: "untracked.txt", Untracked: true},
		{X: ' ', Y: 'D', Path: "deleted.txt"},
		{X: 'R', Y: ' ', Path: "new.txt", OrigPath: "old.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseStatusZ mismatch:\n got=%+v\nwant=%+v", got, want)
	}

	// Staged classification.
	if !want[1].Staged() {
		t.Error("staged.txt should be staged")
	}
	if want[0].Staged() {
		t.Error("modified.txt (worktree-only) should not be staged")
	}
	if want[2].Staged() {
		t.Error("untracked should not be staged")
	}
}
