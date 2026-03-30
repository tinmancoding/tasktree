package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/registry"
)

// PruneResult describes the outcome of a single registry entry inspection.
type PruneResult struct {
	Name   string
	Path   string
	Status TasktreeStatus // only Missing and Invalid entries are included
}

// PruneService removes stale entries from the global tasktree registry.
// An entry is stale when its path no longer exists on disk (Missing) or when
// the path exists but has no .tasktree.toml (Invalid).
type PruneService struct {
	registry *registry.Store
}

// NewPruneService constructs the service.
func NewPruneService(reg *registry.Store) PruneService {
	return PruneService{registry: reg}
}

// Run inspects every registered tasktree and collects stale entries.
// When dryRun is true no changes are written; the returned slice still
// describes what would have been removed.
func (s PruneService) Run(dryRun bool) ([]PruneResult, error) {
	f, err := s.registry.Load()
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	var stale []PruneResult
	for _, te := range f.Tasktrees {
		var status TasktreeStatus
		if _, err := os.Stat(te.Path); os.IsNotExist(err) {
			status = TasktreeStatusMissing
		} else if _, err := os.Stat(filepath.Join(te.Path, domain.MetadataFileName)); os.IsNotExist(err) {
			status = TasktreeStatusInvalid
		} else {
			continue
		}
		stale = append(stale, PruneResult{Name: te.Name, Path: te.Path, Status: status})
	}

	if dryRun || len(stale) == 0 {
		return stale, nil
	}

	for _, entry := range stale {
		if err := s.registry.Deregister(entry.Path); err != nil {
			return stale, fmt.Errorf("deregister %s: %w", entry.Path, err)
		}
	}
	return stale, nil
}
