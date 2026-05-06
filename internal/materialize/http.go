package materialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// HTTP downloads a single file from spec.URL and writes it to destPath.
// If spec.SHA256 is set, the download is verified before the file is placed
// at destPath. The HTTPS scheme is required; HTTP is rejected.
func HTTP(ctx context.Context, destPath string, spec *domain.HTTPSourceSpec) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	return downloadToFileWithHeaders(ctx, spec.URL, spec.Headers, spec.SHA256, destPath)
}
