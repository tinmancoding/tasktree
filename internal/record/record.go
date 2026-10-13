package record

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var idPattern = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{6}$`)
var logHeader = regexp.MustCompile(`(?m)^### ([^ ]+) (note|comment|decision|ref) ([^ @]+)@([^ ]+) \{#e-([0-9a-hjkmnp-tv-z]{6})\}[ \t]*$`)
var reservedH2 = regexp.MustCompile(`(?m)^## (Description|Log)$`)
var descriptionH2 = regexp.MustCompile(`(?m)^## Description$`)
var logH2 = regexp.MustCompile(`(?m)^## Log$`)
var entryHeaderLine = regexp.MustCompile(`^### ([^ ]+) (note|comment|decision|ref) ([^ @]+)@([^ ]+) \{#e-([0-9a-hjkmnp-tv-z]{6})\}[ \t]*$`)

var OpenStatuses = []string{"incoming", "next", "active", "blocked"}
var ClosedStatuses = []string{"done", "dropped"}

type Ref struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Progress struct {
	Closed int `json:"closed"`
	Total  int `json:"total"`
}

func (r Ref) MarshalYAML() (any, error) { return map[string]string{r.Type: r.Value}, nil }

type Entry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Author    string    `json:"author"`
	Machine   string    `json:"machine"`
	Body      string    `json:"body"`
}
type Task struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Parent      *string   `json:"parent"`
	Labels      []string  `json:"labels"`
	Refs        []Ref     `json:"refs"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Description string    `json:"description"`
	Log         []Entry   `json:"log"`
	Conflict    []string  `json:"conflict,omitempty"`
	// Progress is a derived display field and is never persisted in frontmatter.
	Progress *Progress `json:"progress,omitempty"`
}
type Document struct {
	Task     Task
	raw      []byte
	frontEnd int
}

func ValidID(s string) bool { return idPattern.MatchString(s) }
func NewID() (string, error) {
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	b := make([]byte, 6)
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(random[i])&31]
	}
	return string(b), nil
}
func ValidateStatus(s string) error {
	for _, v := range append(append([]string{}, OpenStatuses...), ClosedStatuses...) {
		if s == v {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q (expected incoming, next, active, blocked, done, or dropped)", s)
}
func IsOpen(s string) bool {
	for _, v := range OpenStatuses {
		if s == v {
			return true
		}
	}
	return false
}
func validateBody(kind, s string) error {
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if reservedH2.MatchString(line) {
			return fmt.Errorf("%s contains reserved heading %q", kind, line)
		}
		if entryHeaderLine.MatchString(line) {
			return fmt.Errorf("%s contains reserved log entry header", kind)
		}
	}
	return nil
}
func ValidateDescription(s string) error { return validateBody("description", s) }

// ValidateEntryBody validates text supplied to tasktree note before a hub write.
func ValidateEntryBody(s string) error { return validateBody("note body", s) }

func ValidateKind(s string) error {
	for _, v := range []string{"note", "comment", "decision", "ref"} {
		if s == v {
			return nil
		}
	}
	return fmt.Errorf("invalid note kind %q", s)
}

func Parse(data []byte) (*Document, error) {
	original := append([]byte(nil), data...)
	// Parse against LF while retaining the original bytes for lossless round trips.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, fmt.Errorf("record missing YAML frontmatter")
	}
	rel := bytes.Index(data[4:], []byte("\n---"))
	if rel < 0 {
		return nil, fmt.Errorf("unterminated YAML frontmatter")
	}
	end := 4 + rel + 4
	if end < len(data) && data[end] == '\n' {
		end++
	}
	fm := data[4 : 4+rel]
	var aux struct {
		ID        string              `yaml:"id"`
		Handle    string              `yaml:"handle"`
		Title     string              `yaml:"title"`
		Status    string              `yaml:"status"`
		Parent    *string             `yaml:"parent"`
		Labels    []string            `yaml:"labels"`
		Refs      []map[string]string `yaml:"refs"`
		CreatedAt flexibleTime        `yaml:"createdAt"`
		UpdatedAt flexibleTime        `yaml:"updatedAt"`
		Conflict  []string            `yaml:"conflict"`
	}
	if err := yaml.Unmarshal(fm, &aux); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if !ValidID(aux.ID) {
		return nil, fmt.Errorf("invalid task id %q", aux.ID)
	}
	if err := ValidateStatus(aux.Status); err != nil {
		return nil, err
	}
	t := Task{ID: aux.ID, Handle: aux.Handle, Title: aux.Title, Status: aux.Status, Parent: aux.Parent, Labels: aux.Labels, CreatedAt: aux.CreatedAt.Time.UTC(), UpdatedAt: aux.UpdatedAt.Time.UTC(), Conflict: aux.Conflict}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	for _, m := range aux.Refs {
		if len(m) != 1 {
			return nil, fmt.Errorf("each ref must have exactly one type")
		}
		for k, v := range m {
			if k != "jira" && k != "pr" && k != "url" {
				return nil, fmt.Errorf("invalid ref type %q", k)
			}
			t.Refs = append(t.Refs, Ref{k, v})
		}
	}
	if t.Refs == nil {
		t.Refs = []Ref{}
	}
	body := string(data[end:])
	descMatch := descriptionH2.FindStringIndex(body)
	logMatch := logH2.FindStringIndex(body)
	if descMatch == nil || logMatch == nil || logMatch[0] < descMatch[0] {
		return nil, fmt.Errorf("record must contain Description then Log")
	}
	descEnd := descMatch[1]
	logPos, logEnd := logMatch[0], logMatch[1]
	logTail := body[logEnd:]
	descTail := body[descEnd:logPos]
	t.Description = strings.Trim(descTail, "\n")
	matches := logHeader.FindAllStringSubmatchIndex(logTail, -1)
	seen := map[string]bool{}
	for i, m := range matches {
		ts, err := time.Parse(time.RFC3339, logTail[m[2]:m[3]])
		if err != nil {
			return nil, fmt.Errorf("invalid log timestamp: %w", err)
		}
		id := logTail[m[10]:m[11]]
		if seen[id] {
			return nil, fmt.Errorf("duplicate entry id %s", id)
		}
		seen[id] = true
		stop := len(logTail)
		if i+1 < len(matches) {
			stop = matches[i+1][0]
		}
		contentStart := m[1]
		if contentStart < len(logTail) && logTail[contentStart] == '\n' {
			contentStart++
		}
		bodyText := strings.TrimRight(logTail[contentStart:stop], "\n")
		t.Log = append(t.Log, Entry{ID: id, Timestamp: ts.UTC(), Kind: logTail[m[4]:m[5]], Author: logTail[m[6]:m[7]], Machine: logTail[m[8]:m[9]], Body: bodyText})
	}
	if t.Log == nil {
		t.Log = []Entry{}
	}
	return &Document{Task: t, raw: original, frontEnd: end}, nil
}

func New(t Task) (*Document, error) {
	if !ValidID(t.ID) {
		return nil, fmt.Errorf("invalid task id %q", t.ID)
	}
	if err := ValidateStatus(t.Status); err != nil {
		return nil, err
	}
	if err := ValidateDescription(t.Description); err != nil {
		return nil, err
	}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	if t.Refs == nil {
		t.Refs = []Ref{}
	}
	if t.Log == nil {
		t.Log = []Entry{}
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	type front struct {
		ID        string   `yaml:"id"`
		Handle    string   `yaml:"handle"`
		Title     string   `yaml:"title"`
		Status    string   `yaml:"status"`
		Parent    *string  `yaml:"parent"`
		Labels    []string `yaml:"labels,flow"`
		Refs      []Ref    `yaml:"refs"`
		CreatedAt string   `yaml:"createdAt"`
		UpdatedAt string   `yaml:"updatedAt"`
		Conflict  []string `yaml:"conflict,omitempty,flow"`
	}
	var encoded bytes.Buffer
	enc := yaml.NewEncoder(&encoded)
	enc.SetIndent(2)
	if err := enc.Encode(front{t.ID, t.Handle, t.Title, t.Status, t.Parent, t.Labels, t.Refs, t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339), t.Conflict}); err != nil {
		return nil, err
	}
	_ = enc.Close()
	b := encoded.Bytes()
	// yaml.v3 quotes timestamp-looking strings. Record timestamps are deliberately
	// emitted as plain RFC3339 scalars while the parser accepts both forms.
	b = bytes.Replace(b, []byte("createdAt: \""+t.CreatedAt.Format(time.RFC3339)+"\""), []byte("createdAt: "+t.CreatedAt.Format(time.RFC3339)), 1)
	b = bytes.Replace(b, []byte("updatedAt: \""+t.UpdatedAt.Format(time.RFC3339)+"\""), []byte("updatedAt: "+t.UpdatedAt.Format(time.RFC3339)), 1)
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(b)
	out.WriteString("---\n\n## Description\n\n")
	out.WriteString(t.Description)
	if t.Description != "" {
		out.WriteByte('\n')
	}
	out.WriteString("\n## Log\n")
	d := &Document{Task: t, raw: out.Bytes()}
	parsed, err := Parse(d.raw)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}
func (d *Document) Render() []byte { return append([]byte(nil), d.raw...) }
func (d *Document) SetDescription(description string, at time.Time) error {
	if err := ValidateDescription(description); err != nil {
		return err
	}
	crlf := bytes.Contains(d.raw, []byte("\r\n"))
	raw := bytes.ReplaceAll(d.raw, []byte("\r\n"), []byte("\n"))
	body := raw[d.frontEnd:]
	desc := descriptionH2.FindIndex(body)
	log := logH2.FindIndex(body)
	if desc == nil || log == nil || log[0] < desc[0] {
		return fmt.Errorf("record must contain Description then Log")
	}
	value := strings.ReplaceAll(description, "\r\n", "\n")
	candidate := append([]byte(nil), raw[:d.frontEnd+desc[1]]...)
	candidate = append(candidate, []byte("\n\n"+value)...)
	if value != "" {
		candidate = append(candidate, '\n')
	}
	candidate = append(candidate, '\n')
	candidate = append(candidate, raw[d.frontEnd+log[0]:]...)
	candidate = replaceFront(candidate, "updatedAt", at.UTC().Format(time.RFC3339))
	if crlf {
		candidate = bytes.ReplaceAll(candidate, []byte("\n"), []byte("\r\n"))
	}
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}

// SetFrontString updates one typed frontmatter scalar while preserving unknown
// frontmatter and all non-frontmatter content.
func (d *Document) SetFrontString(key, value string, at time.Time) error {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	candidate := replaceFront(d.raw, key, strings.TrimSpace(string(encoded)))
	candidate = replaceFront(candidate, "updatedAt", at.UTC().Format(time.RFC3339))
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}

func (d *Document) SetParent(parent *string, at time.Time) error {
	value := "null"
	if parent != nil {
		encoded, err := yaml.Marshal(*parent)
		if err != nil {
			return err
		}
		value = strings.TrimSpace(string(encoded))
	}
	candidate := replaceFront(d.raw, "parent", value)
	candidate = replaceFront(candidate, "updatedAt", at.UTC().Format(time.RFC3339))
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}

func (d *Document) SetRefs(refs []Ref, at time.Time) error {
	t := d.Task
	t.Refs, t.UpdatedAt = refs, at.UTC()
	return d.rewriteFront(t)
}

func (d *Document) SetLabels(labels []string, at time.Time) error {
	t := d.Task
	t.Labels, t.UpdatedAt = labels, at.UTC()
	return d.rewriteFront(t)
}

func (d *Document) rewriteFront(t Task) error {
	// Re-rendering the complete record would discard hand-edited sections. Only
	// replace the two collection fields and updatedAt in the original frontmatter.
	refs, err := yaml.Marshal(t.Refs)
	if err != nil {
		return err
	}
	labelsNode := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Tag: "!!seq"}
	for _, label := range t.Labels {
		labelsNode.Content = append(labelsNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: label})
	}
	labels, err := yaml.Marshal(labelsNode)
	if err != nil {
		return err
	}
	candidate := replaceFrontBlock(d.raw, "refs", strings.TrimRight(string(refs), "\n"))
	candidate = replaceFrontBlock(candidate, "labels", strings.TrimSpace(string(labels)))
	candidate = replaceFront(candidate, "updatedAt", t.UpdatedAt.Format(time.RFC3339))
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}

func replaceFrontBlock(raw []byte, key, value string) []byte {
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
	stop := start + 1
	if start < 0 {
		start, stop = end, end
	} else {
		for stop < end && (strings.HasPrefix(lines[stop], " ") || strings.HasPrefix(lines[stop], "\t") || strings.HasPrefix(lines[stop], "-")) {
			stop++
		}
	}
	keyText := key
	if strings.ContainsAny(key, " \t\r\n:\"'{}[],*&?#|>!%@`") {
		keyText = strconv.Quote(key)
	}
	text := keyText + ": " + value
	if strings.Contains(value, "\n") || strings.HasPrefix(value, "- ") {
		text = keyText + ":\n  " + strings.Join(strings.Split(value, "\n"), "\n  ")
	}
	replacement := strings.Split(text, "\n")
	result := make([]string, 0, len(lines)-stop+start+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	result = append(result, lines[stop:]...)
	return []byte(strings.Join(result, eol))
}

func (d *Document) SetStatus(status string, at time.Time) error {
	if err := ValidateStatus(status); err != nil {
		return err
	}
	candidate := replaceFront(d.raw, "status", status)
	candidate = replaceFront(candidate, "updatedAt", at.UTC().Format(time.RFC3339))
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}
func frontKeyLine(lines []string, end int, key string) int {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &node); err == nil && len(node.Content) > 0 {
		mapping := node.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				return mapping.Content[i].Line
			}
		}
	}
	return -1
}
func replaceFront(raw []byte, key, value string) []byte { return replaceFrontBlock(raw, key, value) }
func (d *Document) Append(e Entry) error {
	if !ValidID(e.ID) {
		return fmt.Errorf("invalid entry id")
	}
	if err := ValidateKind(e.Kind); err != nil {
		return err
	}
	if err := ValidateEntryBody(e.Body); err != nil {
		return err
	}
	for _, x := range d.Task.Log {
		if x.ID == e.ID {
			return fmt.Errorf("duplicate entry id %s", e.ID)
		}
	}
	e.Timestamp = e.Timestamp.UTC()
	eol := "\n"
	if bytes.Contains(d.raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	separator := eol + eol
	if bytes.HasSuffix(d.raw, []byte(eol+eol)) {
		separator = ""
	} else if bytes.HasSuffix(d.raw, []byte(eol)) {
		separator = eol
	}
	candidate := append([]byte(nil), d.raw...)
	body := strings.ReplaceAll(e.Body, "\r\n", "\n")
	candidate = append(candidate, []byte(separator+"### "+e.Timestamp.Format(time.RFC3339)+" "+e.Kind+" "+e.Author+"@"+e.Machine+" {#e-"+e.ID+"}"+eol+strings.ReplaceAll(body, "\n", eol)+eol)...)
	candidate = replaceFront(candidate, "updatedAt", e.Timestamp.Format(time.RFC3339))
	parsed, err := Parse(candidate)
	if err != nil {
		return fmt.Errorf("rendered record is invalid: %w", err)
	}
	d.raw, d.Task, d.frontEnd = candidate, parsed.Task, parsed.frontEnd
	return nil
}

type flexibleTime struct{ time.Time }

func (t *flexibleTime) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := time.Parse(time.RFC3339, node.Value)
	if err != nil {
		return fmt.Errorf("invalid RFC3339 timestamp %q: %w", node.Value, err)
	}
	t.Time = parsed
	return nil
}

func (t Task) MarshalJSON() ([]byte, error) { type plain Task; return json.Marshal(plain(t)) }
