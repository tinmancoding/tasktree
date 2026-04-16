package domain

import (
	"fmt"
	"regexp"
	"time"
)

// annotationKeyRe is the pattern a valid annotation key must match.
// Keys must start with a letter or digit and may contain letters, digits,
// dots, hyphens, and underscores. Dots allow simple namespacing (e.g. "jira.ticket").
var annotationKeyRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-]*$`)

// ValidateAnnotationKey returns an InvalidAnnotationKeyError when k is not a
// valid annotation key.
func ValidateAnnotationKey(k string) error {
	if k == "" {
		return InvalidAnnotationKeyError{Key: k, Reason: "key must not be empty"}
	}
	if len(k) > 128 {
		return InvalidAnnotationKeyError{Key: k, Reason: fmt.Sprintf("key length %d exceeds maximum of 128", len(k))}
	}
	if !annotationKeyRe.MatchString(k) {
		return InvalidAnnotationKeyError{Key: k, Reason: "key must match ^[a-zA-Z0-9][a-zA-Z0-9._-]*$"}
	}
	return nil
}

const (
	SpecFileName   = "Tasktree.yml"
	LegacyFileName = ".tasktree.toml"

	APIVersion   = "tasktree.dev/v1"
	KindTasktree = "Tasktree"
)

// TasktreeSpec is the user-authored declarative workspace file (Tasktree.yml).
type TasktreeSpec struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"` // must be "Tasktree"
	Metadata   SpecMetadata  `yaml:"metadata"`
	Spec       WorkspaceSpec `yaml:"spec"`
}

type SpecMetadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	CreatedAt   time.Time         `yaml:"createdAt,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type WorkspaceSpec struct {
	Sources []SourceSpec `yaml:"sources"`
}

type SourceSpec struct {
	Name string         `yaml:"name"`
	Type SourceType     `yaml:"type"`
	Path string         `yaml:"path,omitempty"`
	Git  *GitSourceSpec `yaml:"git,omitempty"`
	// future: HTTP, Archive, Static, Local
}

type SourceType string

const (
	SourceTypeGit     SourceType = "git"
	SourceTypeHTTP    SourceType = "http"
	SourceTypeArchive SourceType = "archive"
	SourceTypeStatic  SourceType = "static"
	SourceTypeLocal   SourceType = "local"
)

type GitSourceSpec struct {
	URL    string `yaml:"url"`
	Ref    string `yaml:"ref,omitempty"`
	Branch string `yaml:"branch,omitempty"`
}
