package metadata

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
)

type Store struct{}

func NewStore() Store {
	return Store{}
}

func (s Store) Path(root string) string {
	return filepath.Join(root, domain.MetadataFileName)
}

func (s Store) Load(root string) (domain.TasktreeFile, error) {
	var file domain.TasktreeFile
	contents, err := os.ReadFile(s.Path(root))
	if err != nil {
		return file, fmt.Errorf("read metadata: %w", err)
	}
	if err := toml.Unmarshal(contents, &file); err != nil {
		return file, fmt.Errorf("parse metadata: %w", err)
	}
	return file, nil
}

func (s Store) Save(root string, file domain.TasktreeFile) error {
	contents, err := toml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	return fsx.AtomicWriteFile(s.Path(root), contents, 0o644)
}
