package record

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var idPattern = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{6}$`)
var logHeader = regexp.MustCompile(`(?m)^### ([^ ]+) (note|comment|decision|ref) ([^ @]+)@([^ ]+) \{#e-([0-9a-hjkmnp-tv-z]{6})\}[ \t]*$`)
var reservedH2 = regexp.MustCompile(`(?m)^## (Description|Log)[ \t]*$`)
var descriptionH2 = regexp.MustCompile(`(?m)^## Description[ \t]*$`)
var logH2 = regexp.MustCompile(`(?m)^## Log[ \t]*$`)
var anyH2 = regexp.MustCompile(`(?m)^## [^\n]+$`)

var OpenStatuses = []string{"incoming", "next", "active", "blocked"}
var ClosedStatuses = []string{"done", "dropped"}

type Ref struct {
	Type  string `json:"type"`
	Value string `json:"value"`
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
func ValidateDescription(s string) error {
	if reservedH2.MatchString(s) {
		return fmt.Errorf("description contains reserved H2 heading")
	}
	return nil
}
func ValidateKind(s string) error {
	for _, v := range []string{"note", "comment", "decision", "ref"} {
		if s == v {
			return nil
		}
	}
	return fmt.Errorf("invalid note kind %q", s)
}

func Parse(data []byte) (*Document, error) {
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
		ID, Handle, Title, Status string
		Parent                    *string
		Labels                    []string
		Refs                      []map[string]string
		CreatedAt, UpdatedAt      time.Time
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
	t := Task{ID: aux.ID, Handle: aux.Handle, Title: aux.Title, Status: aux.Status, Parent: aux.Parent, Labels: aux.Labels, CreatedAt: aux.CreatedAt.UTC(), UpdatedAt: aux.UpdatedAt.UTC()}
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
	if anyH2.MatchString(logTail) {
		return nil, fmt.Errorf("Log must be the last H2 section")
	}
	descTail := body[descEnd:logPos]
	if next := anyH2.FindStringIndex(descTail); next != nil {
		descTail = descTail[:next[0]]
	}
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
	return &Document{Task: t, raw: append([]byte(nil), data...), frontEnd: end}, nil
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
	}
	b, err := yaml.Marshal(front{t.ID, t.Handle, t.Title, t.Status, t.Parent, t.Labels, t.Refs, t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339)})
	if err != nil {
		return nil, err
	}
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
func (d *Document) SetStatus(status string, at time.Time) error {
	if err := ValidateStatus(status); err != nil {
		return err
	}
	d.raw = replaceFront(d.raw, "status", status)
	d.raw = replaceFront(d.raw, "updatedAt", at.UTC().Format(time.RFC3339))
	d.Task.Status = status
	d.Task.UpdatedAt = at.UTC()
	return nil
}
func replaceFront(raw []byte, key, value string) []byte {
	lines := bytes.Split(raw, []byte("\n"))
	prefix := []byte(key + ":")
	for i, l := range lines {
		if bytes.HasPrefix(l, prefix) {
			lines[i] = []byte(key + ": " + value)
			break
		}
	}
	return bytes.Join(lines, []byte("\n"))
}
func (d *Document) Append(e Entry) error {
	if !ValidID(e.ID) {
		return fmt.Errorf("invalid entry id")
	}
	if err := ValidateKind(e.Kind); err != nil {
		return err
	}
	for _, x := range d.Task.Log {
		if x.ID == e.ID {
			return fmt.Errorf("duplicate entry id %s", e.ID)
		}
	}
	e.Timestamp = e.Timestamp.UTC()
	separator := "\n\n"
	if bytes.HasSuffix(d.raw, []byte("\n\n")) {
		separator = ""
	} else if bytes.HasSuffix(d.raw, []byte("\n")) {
		separator = "\n"
	}
	d.raw = append(d.raw, []byte(separator+"### "+e.Timestamp.Format(time.RFC3339)+" "+e.Kind+" "+e.Author+"@"+e.Machine+" {#e-"+e.ID+"}\n"+e.Body+"\n")...)
	d.Task.Log = append(d.Task.Log, e)
	d.Task.UpdatedAt = e.Timestamp
	d.raw = replaceFront(d.raw, "updatedAt", e.Timestamp.Format(time.RFC3339))
	return nil
}
func (t Task) MarshalJSON() ([]byte, error) { type plain Task; return json.Marshal(plain(t)) }
