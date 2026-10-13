package record

import (
	"bytes"
	"strings"
	"testing"
)

func TestMergeQuotedKeysAndArbitraryLabelValues(t *testing.T) {
	base := mergeDoc(t).Render()
	base = bytes.Replace(base, []byte("title: Task"), []byte("\"title\": Task"), 1)
	base = append([]byte(nil), base...)
	base = replaceFrontBlock(base, "custom:key", "old")
	local := replaceFrontBlock(base, "custom:key", "local")
	remote := replaceFrontBlock(base, "custom:key", "old")
	r, e := Parse(remote)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SetFrontString("title", "Remote title\nsecond line", r.Task.UpdatedAt); e != nil {
		t.Fatal(e)
	}
	if e = r.SetLabels([]string{"with, comma", "with: colon"}, r.Task.UpdatedAt); e != nil {
		t.Fatal(e)
	}
	merged, e := Merge(base, local, r.Render(), "", "", "remote commit")
	if e != nil {
		t.Fatal(e)
	}
	d, e := Parse(merged)
	if e != nil {
		t.Fatal(e)
	}
	if d.Task.Title != "Remote title\nsecond line" || len(d.Task.Labels) != 2 || frontValues(d)["custom:key"] != "local" {
		t.Fatalf("task=%+v front=%+v", d.Task, frontValues(d))
	}
	if e = d.SetFrontString("title", "One line again", d.Task.UpdatedAt); e != nil {
		t.Fatal(e)
	}
	if d.Task.Title != "One line again" || strings.Contains(string(d.Render()), "second line") {
		t.Fatal("old multiline scalar survived")
	}
}

func TestMergeRemoteConflictFlagAndNoConfiguredAuthor(t *testing.T) {
	base := mergeDoc(t)
	local, _ := Parse(base.Render())
	remote, _ := Parse(base.Render())
	if e := remote.setConflicts([]string{"description"}); e != nil {
		t.Fatal(e)
	}
	if e := local.Append(Entry{ID: "111111", Timestamp: local.Task.UpdatedAt, Kind: "note", Author: "a", Machine: "A", Body: "local note"}); e != nil {
		t.Fatal(e)
	}
	merged, e := Merge(base.Render(), local.Render(), remote.Render(), "", "", "remote")
	if e != nil {
		t.Fatal(e)
	}
	d, e := Parse(merged)
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Task.Conflict) != 1 {
		t.Fatal("remote conflict flag lost")
	}
	local.SetStatus("active", local.Task.UpdatedAt)
	remote.SetStatus("blocked", remote.Task.UpdatedAt)
	merged, e = Merge(base.Render(), local.Render(), remote.Render(), "", "", "remote")
	if e != nil {
		t.Fatal(e)
	}
	d, e = Parse(merged)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, entry := range d.Task.Log {
		if entry.Kind == "comment" && entry.Author == "tasktree" && entry.Machine == "localhost" {
			found = true
		}
	}
	if !found {
		t.Fatal("fallback author conflict comment was not parsed")
	}
}
