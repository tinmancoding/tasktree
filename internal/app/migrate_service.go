package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// legacyTasktreeFile is the old .tasktree.toml format, kept here only for
// migration purposes. It is not exposed as a domain type.
type legacyTasktreeFile struct {
	Version   int              `toml:"version"`
	Name      string           `toml:"name"`
	CreatedAt time.Time        `toml:"created_at"`
	Repos     []legacyRepoSpec `toml:"repos"`
}

type legacyRepoSpec struct {
	Name        string `toml:"name"`
	Path        string `toml:"path"`
	URL         string `toml:"url"`
	Checkout    string `toml:"checkout"`
	ResolvedRef string `toml:"resolved_ref"`
	Commit      string `toml:"commit"`
	Branch      string `toml:"branch,omitempty"`
}

// MigrateResult describes the outcome of the migration.
type MigrateResult struct {
	Root       string
	Name       string
	Sources    []MigratedSource
	NewPath    string
	BackupPath string
}

// MigratedSource describes a single source entry that was migrated.
type MigratedSource struct {
	Name   string
	URL    string
	Ref    string
	Branch string
}

// MigrateService converts a legacy .tasktree.toml to Tasktree.yml.
type MigrateService struct {
	specStore metadata.Store
}

// NewMigrateService constructs the service.
func NewMigrateService(specStore metadata.Store) MigrateService {
	return MigrateService{specStore: specStore}
}

// Run reads .tasktree.toml, produces Tasktree.yml (discarding resolved state),
// renames .tasktree.toml to .tasktree.toml.bak.
func (s MigrateService) Run(root string) (MigrateResult, error) {
	legacyPath := filepath.Join(root, domain.LegacyFileName)
	contents, err := os.ReadFile(legacyPath)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("read legacy metadata: %w", err)
	}

	var legacy legacyTasktreeFile
	if err := toml.Unmarshal(contents, &legacy); err != nil {
		return MigrateResult{}, fmt.Errorf("parse legacy metadata: %w", err)
	}

	// Build spec from legacy repos — intent only, no resolved state.
	sources := make([]domain.SourceSpec, 0, len(legacy.Repos))
	migratedSources := make([]MigratedSource, 0, len(legacy.Repos))
	for _, repo := range legacy.Repos {
		sourcePath := repo.Path
		if sourcePath == repo.Name {
			sourcePath = "" // omit path if it matches name (will default)
		}

		// Carry over the user's declared intent: checkout (the branch/tag/ref they requested).
		// Discard resolved_ref and commit.
		var ref, branch string
		if repo.Branch != "" {
			branch = repo.Branch
		}
		if repo.Checkout != "" && repo.Checkout != repo.Branch {
			ref = repo.Checkout
		}

		source := domain.SourceSpec{
			Name: repo.Name,
			Type: domain.SourceTypeGit,
			Path: sourcePath,
			Git: &domain.GitSourceSpec{
				URL:    repo.URL,
				Ref:    ref,
				Branch: branch,
			},
		}
		sources = append(sources, source)
		migratedSources = append(migratedSources, MigratedSource{
			Name:   repo.Name,
			URL:    repo.URL,
			Ref:    ref,
			Branch: branch,
		})
	}

	spec := domain.TasktreeSpec{
		APIVersion: domain.APIVersion,
		Kind:       domain.KindTasktree,
		Metadata: domain.SpecMetadata{
			Name:      legacy.Name,
			CreatedAt: legacy.CreatedAt,
		},
		Spec: domain.WorkspaceSpec{
			Sources: sources,
		},
	}

	if err := s.specStore.Save(root, spec); err != nil {
		return MigrateResult{}, fmt.Errorf("write Tasktree.yml: %w", err)
	}

	backupPath := legacyPath + ".bak"
	if err := os.Rename(legacyPath, backupPath); err != nil {
		return MigrateResult{}, fmt.Errorf("rename legacy metadata: %w", err)
	}

	return MigrateResult{
		Root:       root,
		Name:       legacy.Name,
		Sources:    migratedSources,
		NewPath:    s.specStore.Path(root),
		BackupPath: backupPath,
	}, nil
}
