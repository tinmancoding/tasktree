package metadata

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
)

type Store struct{}

func NewStore() Store {
	return Store{}
}

func (s Store) Path(root string) string {
	return filepath.Join(root, domain.SpecFileName)
}

func (s Store) Load(root string) (domain.TasktreeSpec, error) {
	var spec domain.TasktreeSpec
	contents, err := os.ReadFile(s.Path(root))
	if err != nil {
		return spec, fmt.Errorf("read metadata: %w", err)
	}
	if err := yaml.Unmarshal(contents, &spec); err != nil {
		return spec, fmt.Errorf("parse metadata: %w", err)
	}
	return spec, nil
}

func (s Store) Save(root string, spec domain.TasktreeSpec) error {
	contents, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	return fsx.AtomicWriteFile(s.Path(root), contents, 0o644)
}
