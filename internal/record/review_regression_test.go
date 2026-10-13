package record

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestReviewOneSidedBytes(t *testing.T) {
	base := mergeDoc(t).Render()
	for _, changed := range [][]byte{[]byte("malformed hand edit\r\n"), append(append([]byte(nil), base...), []byte("hand-edited log preamble\r\n")...)} {
		for _, remoteChanged := range []bool{false, true} {
			l, r := changed, base
			if remoteChanged {
				l, r = base, changed
			}
			got, err := Merge(base, l, r, "a", "A", "remote")
			if err != nil || !bytes.Equal(got, changed) {
				t.Fatalf("one-sided bytes changed: %v\n%s", err, got)
			}
		}
	}
}
func TestReviewConflictFlagsThreeWay(t *testing.T) {
	b := mergeDoc(t)
	b.setConflicts([]string{"title", "status"})
	l, _ := Parse(b.Render())
	r, _ := Parse(b.Render())
	l.setConflicts([]string{"status", "local"})
	r.setConflicts([]string{"title", "remote"})
	got, err := Merge(b.Render(), l.Render(), r.Render(), "a", "A", "remote")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Task.Conflict, []string{"local", "remote"}) {
		t.Fatalf("flags=%v", d.Task.Conflict)
	}
}
func TestReviewUnknownConflictAudit(t *testing.T) {
	b := replaceFront(mergeDoc(t).Render(), "custom", "base")
	l := replaceFront(b, "custom", "local")
	r := replaceFront(b, "custom", "remote")
	got, err := Merge(b, l, r, "a", "A", "remote-source")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Task.Conflict, []string{"custom"}) || len(d.Task.Log) != 1 || !strings.Contains(d.Task.Log[0].Body, "remote-source") {
		t.Fatalf("missing unknown audit: %s", got)
	}
}
func TestReviewConflictIdentityValidHeader(t *testing.T) {
	b := mergeDoc(t)
	l, _ := Parse(b.Render())
	r, _ := Parse(b.Render())
	l.SetStatus("active", b.Task.UpdatedAt)
	r.SetStatus("blocked", b.Task.UpdatedAt)
	for _, identity := range [][2]string{{"bad name", "host"}, {"a@b", "host"}, {"a", "bad\nhost"}, {"", ""}} {
		got, err := Merge(b.Render(), l.Render(), r.Render(), identity[0], identity[1], "remote")
		if err != nil {
			t.Fatal(err)
		}
		d, err := Parse(got)
		if err != nil || len(d.Task.Log) != 1 {
			t.Fatalf("invalid generated header %v: %v\n%s", identity, err, got)
		}
		if !rawLogHeader.Match(got) {
			t.Fatal("comment does not match entry regex")
		}
	}
}
