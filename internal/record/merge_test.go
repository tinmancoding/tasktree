package record

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mergeDoc(t *testing.T) *Document {
	t.Helper()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, e := New(Task{ID: "abc123", Handle: "task", Title: "Task", Status: "incoming", CreatedAt: at, UpdatedAt: at, Labels: []string{"base", "remove"}, Refs: []Ref{{"jira", "KEEP"}, {"url", "remove"}}})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestSemanticMergeMatrix(t *testing.T) {
	base := mergeDoc(t)
	local, _ := Parse(base.Render())
	remote, _ := Parse(base.Render())
	at := base.Task.UpdatedAt
	local.SetLabels([]string{"base", "local"}, at.Add(time.Minute))
	remote.SetLabels([]string{"remove", "remote"}, at.Add(2*time.Minute))
	local.SetRefs([]Ref{{"jira", "KEEP"}, {"url", "local"}}, at)
	remote.SetRefs([]Ref{{"url", "remove"}, {"pr", "org/repo#1"}}, at)
	remote.SetStatus("next", at)
	local.Append(Entry{ID: "111111", Timestamp: at.Add(2 * time.Minute), Kind: "note", Author: "a", Machine: "A", Body: "local  \nbytes\t"})
	remote.Append(Entry{ID: "222222", Timestamp: at.Add(time.Minute), Kind: "note", Author: "b", Machine: "B", Body: "remote"})
	localBytes := entryChunks(local.Render())["111111"]
	merged, e := Merge(base.Render(), local.Render(), remote.Render(), "a", "A", "B")
	if e != nil {
		t.Fatal(e)
	}
	d, e := Parse(merged)
	if e != nil {
		t.Fatal(e)
	}
	if d.Task.Status != "next" || !reflect.DeepEqual(d.Task.Labels, []string{"local", "remote"}) || !reflect.DeepEqual(d.Task.Refs, []Ref{{"url", "local"}, {"pr", "org/repo#1"}}) {
		t.Fatalf("task=%+v", d.Task)
	}
	if len(d.Task.Log) != 2 || d.Task.Log[0].ID != "222222" {
		t.Fatalf("log=%+v", d.Task.Log)
	}
	if !bytes.Equal(localBytes, entryChunks(merged)["111111"]) {
		t.Fatal("local entry bytes changed")
	}
	if !d.Task.UpdatedAt.Equal(at.Add(2 * time.Minute)) {
		t.Fatalf("updatedAt=%s", d.Task.UpdatedAt)
	}
	again, e := Merge(base.Render(), local.Render(), remote.Render(), "a", "A", "B")
	if e != nil || !bytes.Equal(merged, again) {
		t.Fatal("merge is not pure")
	}
}
func TestSemanticConflictsAndAcknowledgement(t *testing.T) {
	base := mergeDoc(t)
	local, _ := Parse(base.Render())
	remote, _ := Parse(base.Render())
	at := base.Task.UpdatedAt
	local.SetStatus("active", at)
	remote.SetStatus("blocked", at)
	local.SetDescription("local description", at)
	remote.SetDescription("remote description\n````\ntext", at)
	merged, e := Merge(base.Render(), local.Render(), remote.Render(), "a", "A", "B")
	if e != nil {
		t.Fatal(e)
	}
	d, e := Parse(merged)
	if e != nil {
		t.Fatal(e)
	}
	if d.Task.Status != "active" || d.Task.Description != "local description" || !reflect.DeepEqual(d.Task.Conflict, []string{"status", "description"}) || len(d.Task.Log) != 2 {
		t.Fatalf("task=%+v", d.Task)
	}
	if !strings.Contains(string(merged), "Sync conflict on status: remote value (B) was: blocked") || !strings.Contains(string(merged), "`````\nremote description") {
		t.Fatalf("merge=%s", merged)
	}
	if e = d.ClearConflict("description"); e != nil {
		t.Fatal(e)
	}
	if e = d.SetDescription("new description", at); e != nil {
		t.Fatal(e)
	}
	if e = d.ClearConflict("status"); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(d.Render()), "conflict:") || d.Task.Description != "new description" {
		t.Fatalf("ack=%s", d.Render())
	}
	if e := ValidateEntryBody(escapeBody("## Log\n### 2026-01-01T00:00:00Z note a@A {#e-abc123}")); e != nil {
		t.Fatal(e)
	}
}
func TestSemanticUnknownFrontmatterAndCRLF(t *testing.T) {
	base := mergeDoc(t).Render()
	base = replaceFrontBlock(base, "custom", "value: base\nother: preserved")
	local := replaceFrontBlock(base, "custom", "value: local\nother: preserved")
	remote := replaceFrontBlock(base, "custom", "value: remote\nother: preserved")
	l, _ := Parse(local)
	r, _ := Parse(remote)
	at := l.Task.UpdatedAt
	l.Append(Entry{ID: "111111", Timestamp: at, Kind: "note", Author: "a", Machine: "A", Body: "original"})
	local = bytes.ReplaceAll(l.Render(), []byte("\n"), []byte("\r\n"))
	r.Append(Entry{ID: "222222", Timestamp: at.Add(time.Minute), Kind: "note", Author: "b", Machine: "B", Body: "remote"})
	result, e := Merge(base, local, r.Render(), "a", "A", "B")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(entryChunks(local)["111111"], entryChunks(result)["111111"]) {
		t.Fatal("CRLF local chunk changed")
	}
	d, e := Parse(result)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(frontValues(d)["custom"], frontValues(l)["custom"]) {
		t.Fatal("unknown conflict did not keep local")
	}
	if len(d.Task.Conflict) != 0 {
		t.Fatal("unknown key should not be a flagged scalar conflict")
	}
	remoteOnly := replaceFront(base, "extra", "remote")
	result, e = Merge(base, base, remoteOnly, "a", "A", "B")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(result), "extra: remote") {
		t.Fatal("unknown remote key lost")
	}
}
func TestSemanticFilePresence(t *testing.T) {
	b := mergeDoc(t).Render()
	modified := replaceFront(b, "title", "Modified")
	for _, x := range []struct{ base, local, remote, want []byte }{{nil, b, nil, b}, {nil, nil, b, b}, {b, nil, b, nil}, {b, b, nil, nil}, {b, nil, modified, modified}, {b, modified, nil, modified}} {
		got, e := Merge(x.base, x.local, x.remote, "a", "A", "B")
		if e != nil || !bytes.Equal(got, x.want) {
			t.Fatalf("presence got=%s want=%s err=%v", got, x.want, e)
		}
	}
}
