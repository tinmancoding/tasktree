package domain

import "time"

const (
	MetadataFileName = ".tasktree.toml"
	MetadataVersion  = 1
)

type TasktreeFile struct {
	Version   int        `toml:"version"`
	Name      string     `toml:"name"`
	CreatedAt time.Time  `toml:"created_at"`
	Repos     []RepoSpec `toml:"repos"`
}

type RepoSpec struct {
	Name        string `toml:"name"`
	Path        string `toml:"path"`
	URL         string `toml:"url"`
	Checkout    string `toml:"checkout"`
	ResolvedRef string `toml:"resolved_ref"`
	Commit      string `toml:"commit"`
	Branch      string `toml:"branch,omitempty"`
}
