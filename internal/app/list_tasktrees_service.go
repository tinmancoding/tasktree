package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/registry"
)

// TasktreeStatus indicates whether a registered tasktree path is accessible.
type TasktreeStatus string

const (
	// TasktreeStatusOK means the path exists and contains a valid .tasktree.toml.
	TasktreeStatusOK TasktreeStatus = "ok"
	// TasktreeStatusMissing means the path no longer exists on disk.
	TasktreeStatusMissing TasktreeStatus = "missing"
	// TasktreeStatusInvalid means the path exists but contains no .tasktree.toml.
	TasktreeStatusInvalid TasktreeStatus = "invalid"
)

// TasktreeListEntry is a single row returned by ListTasktreesService.
type TasktreeListEntry struct {
	registry.TasktreeEntry
	Status TasktreeStatus
}

// ListTasktreesService lists all tasktrees known to the global registry.
type ListTasktreesService struct {
	registry *registry.Store
}

// NewListTasktreesService constructs the service.
func NewListTasktreesService(reg *registry.Store) ListTasktreesService {
	return ListTasktreesService{registry: reg}
}

// Run loads the registry and validates each entry against the filesystem.
// It never modifies the registry — stale entries are reported but not pruned.
func (s ListTasktreesService) Run() ([]TasktreeListEntry, error) {
	f, err := s.registry.Load()
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	entries := make([]TasktreeListEntry, 0, len(f.Tasktrees))
	for _, te := range f.Tasktrees {
		entry := TasktreeListEntry{TasktreeEntry: te}

		if _, err := os.Stat(te.Path); os.IsNotExist(err) {
			entry.Status = TasktreeStatusMissing
		} else if _, err := os.Stat(filepath.Join(te.Path, domain.MetadataFileName)); os.IsNotExist(err) {
			entry.Status = TasktreeStatusInvalid
		} else {
			entry.Status = TasktreeStatusOK
		}

		entries = append(entries, entry)
	}
	return entries, nil
}
