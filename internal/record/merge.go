package record

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClearConflict acknowledges a field without changing any other frontmatter.
func (d *Document) ClearConflict(field string) error {
	fields := []string{}
	for _, f := range d.Task.Conflict {
		if f != field {
			fields = append(fields, f)
		}
	}
	return d.setConflicts(fields)
}
func (d *Document) setConflicts(fields []string) error {
	var candidate []byte
	if len(fields) == 0 {
		candidate = removeFront(d.raw, "conflict")
	} else {
		b, err := yaml.Marshal(fields)
		if err != nil {
			return err
		}
		candidate = replaceFrontBlock(d.raw, "conflict", strings.TrimSpace(string(b)))
	}
	p, err := Parse(candidate)
	if err != nil {
		return err
	}
	d.raw, d.Task, d.frontEnd = candidate, p.Task, p.frontEnd
	return nil
}
func removeFront(raw []byte, key string) []byte {
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(string(raw), eol)
	end := len(lines)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	start := frontKeyLine(lines, end, key)
	if start < 0 {
		return raw
	}
	stop := start + 1
	for stop < end && (strings.HasPrefix(lines[stop], " ") || strings.HasPrefix(lines[stop], "\t") || strings.HasPrefix(lines[stop], "-")) {
		stop++
	}
	return []byte(strings.Join(append(lines[:start], lines[stop:]...), eol))
}
func frontValues(d *Document) map[string]any {
	if d == nil {
		return map[string]any{}
	}
	raw := bytes.ReplaceAll(d.raw, []byte("\r\n"), []byte("\n"))
	end := bytes.Index(raw[4:], []byte("\n---"))
	m := map[string]any{}
	_ = yaml.Unmarshal(raw[4:4+end], &m)
	return m
}

// Merge performs S4's semantic three-way merge. Missing files are nil. Log
// chunks from the local document are copied verbatim, including line endings
// and spacing. Remote entries that interleave are inserted in timestamp/id
// order, rather than appended out of order. This is also the tie-break for
// equal timestamps and makes subsequent fast-forwards byte-identical.
func Merge(base, local, remote []byte, author, machine, remoteSource string) ([]byte, error) {
	if author == "" {
		author = "tasktree"
	}
	if machine == "" {
		machine = "localhost"
	}
	if bytes.Equal(local, remote) {
		return append([]byte(nil), local...), nil
	}
	if local == nil {
		if bytes.Equal(base, remote) {
			return nil, nil
		}
		return append([]byte(nil), remote...), nil
	}
	if remote == nil {
		if bytes.Equal(base, local) {
			return nil, nil
		}
		return append([]byte(nil), local...), nil
	}
	l, err := Parse(local)
	if err != nil {
		return nil, err
	}
	r, err := Parse(remote)
	if err != nil {
		return nil, err
	}
	var b *Document
	if base != nil {
		b, err = Parse(base)
		if err != nil {
			return nil, err
		}
	}
	at := l.Task.UpdatedAt
	if r.Task.UpdatedAt.After(at) {
		at = r.Task.UpdatedAt
	}
	bm, lm, rm := frontValues(b), frontValues(l), frontValues(r)
	fields := map[string]bool{}
	for k := range bm {
		fields[k] = true
	}
	for k := range lm {
		fields[k] = true
	}
	for k := range rm {
		fields[k] = true
	}
	keys := []string{}
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	conflicts := append([]string(nil), l.Task.Conflict...)
	baseConflicts := []string(nil)
	if b != nil {
		baseConflicts = b.Task.Conflict
	}
	if reflect.DeepEqual(l.Task.Conflict, baseConflicts) {
		conflicts = append([]string(nil), r.Task.Conflict...)
	}
	occupied := map[string]Entry{}
	for _, e := range r.Task.Log {
		occupied[e.ID] = e
	}
	for _, e := range l.Task.Log {
		occupied[e.ID] = e
	}
	var comments []Entry
	conflict := func(field string, value any) {
		found := false
		for _, f := range conflicts {
			if f == field {
				found = true
			}
		}
		if !found {
			conflicts = append(conflicts, field)
		}
		text := fmt.Sprint(value)
		if field == "description" {
			text = escapeBody(text)
			fence := "```"
			for strings.Contains(text, fence) {
				fence += "`"
			}
			text = "\n" + fence + "\n" + text + "\n" + fence
		}
		body := fmt.Sprintf("Sync conflict on %s: remote value (%s) was: %s", field, remoteSource, text)
		localValue := fmt.Sprint(lm[field])
		if field == "description" {
			localValue = l.Task.Description
		}
		seed := l.Task.ID + "\x00" + field + "\x00" + body + "\x00" + localValue + "\x00" + at.String()
		const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
		id := make([]byte, 6)
		for nonce := 0; ; nonce++ {
			sum := sha256.Sum256([]byte(seed + fmt.Sprint(nonce)))
			for i := range id {
				id[i] = alphabet[sum[i]&31]
			}
			old, exists := occupied[string(id)]
			if !exists || (old.Kind == "comment" && old.Body == escapeBody(body)) {
				break
			}
		}
		e := Entry{ID: string(id), Timestamp: at, Kind: "comment", Author: author, Machine: machine, Body: escapeBody(body)}
		occupied[e.ID] = e
		comments = append(comments, e)
	}
	for _, k := range keys {
		if k == "id" || k == "createdAt" || k == "updatedAt" || k == "labels" || k == "refs" || k == "conflict" {
			continue
		}
		bv, lv, rv := bm[k], lm[k], rm[k]
		value := lv
		if reflect.DeepEqual(lv, bv) {
			value = rv
		} else if !reflect.DeepEqual(rv, bv) && !reflect.DeepEqual(lv, rv) {
			if k == "title" || k == "handle" || k == "status" || k == "parent" {
				conflict(k, rv)
			}
		}
		if reflect.DeepEqual(lv, value) {
			continue
		}
		if value == nil {
			l.raw = removeFront(l.raw, k)
		} else {
			enc, e := yaml.Marshal(value)
			if e != nil {
				return nil, e
			}
			l.raw = replaceFrontBlock(l.raw, k, strings.TrimSpace(string(enc)))
		}
	}
	l, err = Parse(l.raw)
	if err != nil {
		return nil, err
	}
	descBase := ""
	if b != nil {
		descBase = b.Task.Description
	}
	if l.Task.Description == descBase {
		if err = l.SetDescription(r.Task.Description, at); err != nil {
			return nil, err
		}
	} else if r.Task.Description != descBase && r.Task.Description != l.Task.Description {
		conflict("description", r.Task.Description)
	}
	labelsBase := []string{}
	refsBase := []Ref{}
	if b != nil {
		labelsBase = b.Task.Labels
		refsBase = b.Task.Refs
	}
	labels := mergeSet(labelsBase, l.Task.Labels, r.Task.Labels)
	refs := mergeSet(refsBase, l.Task.Refs, r.Task.Refs)
	// Mutators validate and preserve unknown frontmatter and the complete Log.
	if err = l.SetLabels(labels, at); err != nil {
		return nil, err
	}
	if err = l.SetRefs(refs, at); err != nil {
		return nil, err
	}
	if err = l.setConflicts(conflicts); err != nil {
		return nil, err
	}
	chunks := entryChunks(local)
	remoteChunks := entryChunks(remote)
	entries := append([]Entry(nil), l.Task.Log...)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.ID] = true
	}
	for _, e := range r.Task.Log {
		if !seen[e.ID] {
			entries = append(entries, e)
			chunks[e.ID] = remoteChunks[e.ID]
			seen[e.ID] = true
		}
	}
	for _, e := range comments {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		entries = append(entries, e)
		chunks[e.ID] = []byte("### " + e.Timestamp.Format("2006-01-02T15:04:05Z07:00") + " " + e.Kind + " " + e.Author + "@" + e.Machine + " {#e-" + e.ID + "}\n" + e.Body + "\n\n")
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	raw := l.raw
	headers := rawLogHeader.FindAllIndex(raw, -1)
	start := len(raw)
	if len(headers) > 0 {
		start = headers[0][0]
	}
	raw = append([]byte(nil), raw[:start]...)
	for _, e := range entries {
		if len(raw) > 0 && raw[len(raw)-1] != '\n' {
			raw = append(raw, '\n')
		}
		raw = append(raw, chunks[e.ID]...)
	}
	raw = replaceFront(raw, "updatedAt", at.Format("2006-01-02T15:04:05Z07:00"))
	if _, err = Parse(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func mergeSet[T comparable](base, local, remote []T) []T {
	b, l, r := map[T]bool{}, map[T]bool{}, map[T]bool{}
	for _, v := range base {
		b[v] = true
	}
	for _, v := range local {
		l[v] = true
	}
	for _, v := range remote {
		r[v] = true
	}
	out := []T{}
	seen := map[T]bool{}
	for _, v := range append(append([]T(nil), local...), remote...) {
		if !seen[v] && ((!b[v] && (l[v] || r[v])) || (b[v] && l[v] && r[v])) {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}

var rawLogHeader = regexp.MustCompile(strings.TrimSuffix(logHeader.String(), "$") + "\\r?$")

func entryChunks(raw []byte) map[string][]byte {
	ms := rawLogHeader.FindAllSubmatchIndex(raw, -1)
	chunks := map[string][]byte{}
	for i, m := range ms {
		stop := len(raw)
		if i+1 < len(ms) {
			stop = ms[i+1][0]
		}
		chunks[string(raw[m[10]:m[11]])] = append([]byte(nil), raw[m[0]:stop]...)
	}
	return chunks
}
func escapeBody(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if reservedH2.MatchString(line) || entryHeaderLine.MatchString(line) {
			lines[i] = "\\" + line
		}
	}
	return strings.Join(lines, "\n")
}
