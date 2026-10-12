package record_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tinmancoding/tasktree/v2/internal/record"
)

const fixture = `---
id: k7x2mq
handle: hand-edited
title: A title
x-extra: keep me
status: next
parent: null
labels: [one]
refs:
  - url: https://example.com
createdAt: 2026-10-12T09:00:00Z
updatedAt: 2026-10-12T10:03:00Z
---

A hand-written preamble.

## Description

Keep this prose exactly.

## Investigation

An extra hand-edited section.

## Log

Loose text before entries.

### 2026-10-12T10:03:00Z note laci@mbp {#e-01jab3}
First body.
`

func TestLosslessRoundTrip(t *testing.T) {
	d, err := record.Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Render(); !bytes.Equal(got, []byte(fixture)) {
		t.Fatal("round trip changed bytes")
	}
}
func TestMutationsPreserveUnknownContent(t *testing.T) {
	d, err := record.Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetStatus("active", time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(record.Entry{ID: "01jab9", Timestamp: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), Kind: "decision", Author: "pi", Machine: "mbp", Body: "Decided."}); err != nil {
		t.Fatal(err)
	}
	got := string(d.Render())
	for _, want := range []string{"x-extra: keep me", "A hand-written preamble.", "## Investigation\n\nAn extra hand-edited section.", "status: active", "### 2026-10-12T12:00:00Z decision pi@mbp {#e-01jab9}\nDecided."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}
func TestTimestampCompatibilityAndNewFormatting(t *testing.T) {
	quoted := strings.Replace(fixture, "createdAt: 2026-10-12T09:00:00Z", "createdAt: \"2026-10-12T09:00:00Z\"", 1)
	d, err := record.Parse([]byte(quoted))
	if err != nil || d.Task.CreatedAt.IsZero() {
		t.Fatalf("quoted timestamp: %v %+v", err, d)
	}
	n, err := record.New(d.Task)
	if err != nil {
		t.Fatal(err)
	}
	got := string(n.Render())
	if strings.Contains(got, "createdAt: \"") || !strings.Contains(got, "refs:\n  - url:") {
		t.Fatalf("unexpected new formatting:\n%s", got)
	}
}

func TestDescriptionReservedHeadingRejected(t *testing.T) {
	if err := record.ValidateDescription("text\n## Log\nno"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLogBodyAllowsOrdinaryHeadingsAndRoundTripsCRLF(t *testing.T) {
	crlf := []byte(strings.ReplaceAll(fixture, "\n", "\r\n"))
	d, err := record.Parse(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Render(), crlf) {
		t.Fatal("CRLF round trip changed bytes")
	}
	if err := d.Append(record.Entry{ID: "01jabc", Timestamp: time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC), Kind: "note", Author: "pi", Machine: "mbp", Body: "## Findings\n### Detail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := record.Parse(d.Render()); err != nil {
		t.Fatalf("render no longer parses: %v", err)
	}
}

func TestAppendRejectsRecordBoundaryLines(t *testing.T) {
	d, err := record.Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"## Log", "### 2026-10-12T10:03:00Z note laci@mbp {#e-01jab4}"} {
		if err := d.Append(record.Entry{ID: "01jab8", Timestamp: time.Now(), Kind: "note", Author: "a", Machine: "m", Body: body}); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestReplaceFrontDoesNotReplaceBodyAndInsertsMissingKey(t *testing.T) {
	raw := strings.Replace(fixture, "updatedAt: 2026-10-12T10:03:00Z\n", "", 1) + "\nupdatedAt: prose\n"
	d, err := record.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetStatus("active", time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got := string(d.Render())
	if !strings.Contains(got, "updatedAt: 2026-10-12T14:00:00Z\n---") {
		t.Fatal("missing frontmatter key was not inserted")
	}
	if !strings.Contains(got, "updatedAt: prose") {
		t.Fatal("body key was replaced")
	}
}
func TestIDFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := record.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if !record.ValidID(id) {
			t.Fatalf("invalid %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate %q", id)
		}
		seen[id] = true
	}
}
