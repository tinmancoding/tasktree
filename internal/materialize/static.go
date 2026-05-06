package materialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
)

// Static writes inline content from a StaticSourceSpec to destPath.
func Static(destPath string, spec *domain.StaticSourceSpec) error {
	mode, err := parseMode(spec.Mode)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	return fsx.AtomicWriteFile(destPath, []byte(spec.Content), mode)
}

// parseMode parses an octal string (e.g. "0644") into an os.FileMode.
// An empty string returns the default mode 0644.
func parseMode(s string) (os.FileMode, error) {
	if s == "" {
		return 0o644, nil
	}
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode %q: must be an octal string like \"0644\"", s)
	}
	return os.FileMode(n), nil
}
