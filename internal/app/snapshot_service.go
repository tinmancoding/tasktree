package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/snapshot"
)

// SnapshotOptions configures a snapshot run.
type SnapshotOptions struct {
	IncludeIgnored bool
	// Output receives the snapshot tar.gz stream.
	Output io.Writer
}

// SnapshotSourceResult summarizes one source's contribution to the snapshot.
type SnapshotSourceResult struct {
	Name      string
	Type      domain.SourceType
	HasBundle bool
	HasDirty  bool
}

// SnapshotResult holds the outcome of a snapshot run.
type SnapshotResult struct {
	Root     string
	Tasktree string
	Sources  []SnapshotSourceResult
}

// SnapshotService captures a workspace's concrete state into a single tar.gz.
type SnapshotService struct {
	store metadata.Store
	git   gitx.Client
}

func NewSnapshotService(store metadata.Store, git gitx.Client) SnapshotService {
	return SnapshotService{store: store, git: git}
}

func (s SnapshotService) Run(ctx context.Context, start string, opts SnapshotOptions) (SnapshotResult, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return SnapshotResult{}, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return SnapshotResult{}, err
	}

	// Embed the spec file verbatim so restore is self-contained.
	specBytes, err := os.ReadFile(s.store.Path(root))
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("read spec: %w", err)
	}

	manifest := domain.SnapshotManifest{
		Version:   domain.SnapshotManifestVersion,
		CreatedAt: time.Now().UTC(),
		Tasktree:  spec.Metadata.Name,
	}
	members := []snapshot.Member{
		{Name: domain.SpecFileName, Data: specBytes},
	}
	result := SnapshotResult{Root: root, Tasktree: spec.Metadata.Name}

	for _, source := range spec.Spec.Sources {
		relPath := source.Path
		if relPath == "" {
			relPath = source.Name
		}
		srcPath := filepath.Join(root, relPath)
		exists, err := fsx.Exists(srcPath)
		if err != nil {
			return SnapshotResult{}, err
		}
		if !exists {
			return SnapshotResult{}, domain.SourceNotMaterializedError{Name: source.Name, Path: relPath}
		}

		entry := domain.SnapshotSourceEntry{Name: source.Name, Type: source.Type}
		srcResult := SnapshotSourceResult{Name: source.Name, Type: source.Type}

		if source.Type == domain.SourceTypeGit {
			capture, err := snapshot.Git(ctx, srcPath, source.Name, opts.IncludeIgnored, s.git)
			if err != nil {
				return SnapshotResult{}, fmt.Errorf("source %q: %w", source.Name, err)
			}
			git := &domain.GitSubSnapshot{
				RemoteURL:      capture.RemoteURL,
				Branch:         capture.Branch,
				Detached:       capture.Detached,
				BaseSHA:        capture.BaseSHA,
				HeadSHA:        capture.HeadSHA,
				IncludeIgnored: opts.IncludeIgnored,
			}
			if capture.Bundle != nil {
				name := "bundles/" + source.Name + ".bundle"
				git.Bundle = name
				members = append(members, snapshot.Member{Name: name, Data: capture.Bundle})
				srcResult.HasBundle = true
			}
			if capture.Dirty != nil {
				name := "dirty/" + source.Name + ".tar"
				git.Dirty = name
				members = append(members, snapshot.Member{Name: name, Data: capture.Dirty})
				srcResult.HasDirty = true
			}
			entry.Git = git
		}

		manifest.Sources = append(manifest.Sources, entry)
		result.Sources = append(result.Sources, srcResult)
	}

	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("encode manifest: %w", err)
	}
	// Manifest first for readability when listing the archive.
	members = append([]snapshot.Member{{Name: domain.SnapshotManifestName, Data: manifestBytes}}, members...)

	if err := snapshot.Pack(opts.Output, members); err != nil {
		return SnapshotResult{}, err
	}
	return result, nil
}
