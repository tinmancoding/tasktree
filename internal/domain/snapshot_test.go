package domain

import (
	"errors"
	"testing"
)

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		m       SnapshotManifest
		wantErr any
	}{
		{
			name: "valid",
			m: SnapshotManifest{Version: SnapshotManifestVersion, Sources: []SnapshotSourceEntry{
				{Name: "api", Type: SourceTypeGit, Git: &GitSubSnapshot{RemoteURL: "u", BaseSHA: "a", HeadSHA: "b"}},
				{Name: "docs", Type: SourceTypeHTTP},
			}},
		},
		{
			name:    "bad version",
			m:       SnapshotManifest{Version: 99},
			wantErr: UnsupportedSnapshotVersionError{},
		},
		{
			name: "duplicate name",
			m: SnapshotManifest{Version: SnapshotManifestVersion, Sources: []SnapshotSourceEntry{
				{Name: "api", Type: SourceTypeHTTP},
				{Name: "api", Type: SourceTypeHTTP},
			}},
			wantErr: IncompleteSnapshotManifestError{},
		},
		{
			name: "incomplete git",
			m: SnapshotManifest{Version: SnapshotManifestVersion, Sources: []SnapshotSourceEntry{
				{Name: "api", Type: SourceTypeGit, Git: &GitSubSnapshot{RemoteURL: "u"}},
			}},
			wantErr: IncompleteSnapshotManifestError{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(tt.m)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			switch tt.wantErr.(type) {
			case UnsupportedSnapshotVersionError:
				var target UnsupportedSnapshotVersionError
				if !errors.As(err, &target) {
					t.Fatalf("want UnsupportedSnapshotVersionError, got %v", err)
				}
			case IncompleteSnapshotManifestError:
				var target IncompleteSnapshotManifestError
				if !errors.As(err, &target) {
					t.Fatalf("want IncompleteSnapshotManifestError, got %v", err)
				}
			}
		})
	}
}
