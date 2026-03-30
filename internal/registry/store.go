package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/tinmancoding/tasktree/internal/fsx"
)

const registryVersion = 1

// RegistryFile is the top-level structure of the global registry file.
type RegistryFile struct {
	Version   int             `toml:"version"`
	Tasktrees []TasktreeEntry `toml:"tasktrees"`
}

// TasktreeEntry is a single registered tasktree in the global registry.
type TasktreeEntry struct {
	Path    string    `toml:"path"`
	Name    string    `toml:"name"`
	AddedAt time.Time `toml:"added_at"`
}

// Store manages the global tasktree registry file.
type Store struct {
	path string
}

// NewStore returns a Store using the default OS-appropriate registry path.
func NewStore() (*Store, error) {
	p, err := defaultRegistryPath()
	if err != nil {
		return nil, fmt.Errorf("resolve registry path: %w", err)
	}
	return &Store{path: p}, nil
}

// NewStoreAt returns a Store using the given path. Intended for tests.
func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

// Load reads and parses the registry file. Returns an empty registry if the
// file does not exist yet.
func (s *Store) Load() (RegistryFile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return RegistryFile{Version: registryVersion}, nil
	}
	if err != nil {
		return RegistryFile{}, fmt.Errorf("read registry: %w", err)
	}
	var f RegistryFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return RegistryFile{}, fmt.Errorf("parse registry at %s: %w", s.path, err)
	}
	return f, nil
}

// Save atomically writes the registry file to disk, creating parent directories
// as needed.
func (s *Store) Save(f RegistryFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create registry directory: %w", err)
	}
	data, err := toml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := fsx.AtomicWriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// Register adds or updates the entry for the given absolute path.
// If an entry with the same path already exists its name is updated in-place;
// AddedAt is preserved.
func (s *Store) Register(path, name string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}
	for i, entry := range f.Tasktrees {
		if entry.Path == path {
			f.Tasktrees[i].Name = name
			return s.Save(f)
		}
	}
	f.Tasktrees = append(f.Tasktrees, TasktreeEntry{
		Path:    path,
		Name:    name,
		AddedAt: time.Now().UTC(),
	})
	return s.Save(f)
}

// Deregister removes the entry with the given absolute path. It is a no-op if
// the path is not registered.
func (s *Store) Deregister(path string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}
	filtered := f.Tasktrees[:0]
	for _, entry := range f.Tasktrees {
		if entry.Path != path {
			filtered = append(filtered, entry)
		}
	}
	f.Tasktrees = filtered
	return s.Save(f)
}

// defaultRegistryPath returns ~/.local/state/tasktree/registry.toml.
func defaultRegistryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "tasktree", "registry.toml"), nil
}
