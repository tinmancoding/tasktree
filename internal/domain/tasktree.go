package domain

import "time"

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
