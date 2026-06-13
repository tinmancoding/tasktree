package domain

import (
	"time"
)

// SnapshotManifestVersion is the current snapshot manifest schema version.
// Restore rejects manifests with an unknown/newer major version.
const SnapshotManifestVersion = 1

// SnapshotManifestName is the manifest file name inside the snapshot tarball.
const SnapshotManifestName = "snapshot.yaml"

// SnapshotManifest is the top-level descriptor of a workspace snapshot. It is
// the one artifact where resolved SHAs are allowed to live.
type SnapshotManifest struct {
	Version   int                   `yaml:"version"`
	CreatedAt time.Time             `yaml:"createdAt"`
	Tasktree  string                `yaml:"tasktree"`
	Sources   []SnapshotSourceEntry `yaml:"sources"`
}

// SnapshotSourceEntry records one source's contribution to the snapshot.
// Non-git source types are inventory-only (Git is nil) and reproduced from the
// embedded spec on restore.
type SnapshotSourceEntry struct {
	Name string          `yaml:"name"`
	Type SourceType      `yaml:"type"`
	Git  *GitSubSnapshot `yaml:"git,omitempty"`
}

// GitSubSnapshot captures the concrete state of a git source.
type GitSubSnapshot struct {
	RemoteURL      string `yaml:"remoteURL"`
	Branch         string `yaml:"branch,omitempty"`
	Detached       bool   `yaml:"detached,omitempty"`
	BaseSHA        string `yaml:"baseSHA"`
	HeadSHA        string `yaml:"headSHA"`
	Bundle         string `yaml:"bundle,omitempty"` // path within the tar; empty if no local commits
	Dirty          string `yaml:"dirty,omitempty"`  // path within the tar; empty if clean
	IncludeIgnored bool   `yaml:"includeIgnored,omitempty"`
}

// DirtyManifestName is the side-manifest member inside each dirty tar.
const DirtyManifestName = ".tasktree-dirty.yaml"

// DirtyManifest is stored inside a dirty/<source>.tar archive. It records the
// metadata that the captured file content alone cannot represent.
type DirtyManifest struct {
	// Deleted lists tracked paths that were removed in the working tree (and
	// rename sources). Restore deletes these after unpacking content.
	Deleted []string `yaml:"deleted,omitempty"`
	// Staged lists paths that were staged in the index at capture time. Restore
	// re-runs `git add` on them as a best-effort index-fidelity hint.
	Staged []string `yaml:"staged,omitempty"`
}

// ValidateManifest checks structural invariants of a loaded snapshot manifest.
func ValidateManifest(m SnapshotManifest) error {
	if m.Version != SnapshotManifestVersion {
		return UnsupportedSnapshotVersionError{Found: m.Version, Max: SnapshotManifestVersion}
	}
	seen := make(map[string]struct{}, len(m.Sources))
	for _, s := range m.Sources {
		if s.Name == "" {
			return IncompleteSnapshotManifestError{Reason: "source entry has empty name"}
		}
		if _, dup := seen[s.Name]; dup {
			return IncompleteSnapshotManifestError{Reason: "duplicate source name " + s.Name}
		}
		seen[s.Name] = struct{}{}
		if s.Git != nil {
			if s.Git.RemoteURL == "" || s.Git.BaseSHA == "" || s.Git.HeadSHA == "" {
				return IncompleteSnapshotManifestError{Reason: "git source " + s.Name + " missing remoteURL/baseSHA/headSHA"}
			}
		}
	}
	return nil
}
