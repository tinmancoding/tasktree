package materialize

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// Archive downloads a remote archive (tar.gz, tar.bz2, or zip), optionally
// verifies its SHA-256 digest, and extracts it into destPath with optional
// path-component stripping. tar.xz is not yet supported.
func Archive(ctx context.Context, destPath string, spec *domain.ArchiveSourceSpec) error {
	// Download to a temp file.
	tmp, err := os.CreateTemp("", "tasktree-archive-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := downloadToFile(ctx, spec.URL, spec.SHA256, tmpPath); err != nil {
		return err
	}

	format := spec.Format
	if format == "" {
		format = detectArchiveFormat(spec.URL)
	}
	if format == "" {
		return domain.UnknownArchiveFormatError{URL: spec.URL}
	}

	if err := os.MkdirAll(destPath, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	if err := extractArchive(tmpPath, format, destPath, spec.StripComponents); err != nil {
		_ = os.RemoveAll(destPath)
		return err
	}
	return nil
}

// detectArchiveFormat infers the archive format from the URL file extension.
func detectArchiveFormat(url string) string {
	lower := strings.ToLower(url)
	// Strip query string if present.
	if idx := strings.IndexByte(lower, '?'); idx >= 0 {
		lower = lower[:idx]
	}
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
		return "tar.bz2"
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	}
	return ""
}

// extractArchive dispatches to the format-specific extractor.
func extractArchive(archivePath, format, destPath string, stripComponents int) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	switch format {
	case "tar.gz":
		gr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gzip reader: %w", err)
		}
		defer gr.Close()
		return extractTar(tar.NewReader(gr), destPath, stripComponents)

	case "tar.bz2":
		return extractTar(tar.NewReader(bzip2.NewReader(f)), destPath, stripComponents)

	case "tar.xz":
		return fmt.Errorf("tar.xz format is not yet supported; use tar.gz or zip")

	case "zip":
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		return extractZip(f, fi.Size(), destPath, stripComponents)

	default:
		return domain.UnknownArchiveFormatError{URL: archivePath}
	}
}

// extractTar extracts entries from a tar.Reader into destPath, stripping the
// first stripComponents path components from each entry name.
func extractTar(tr *tar.Reader, destPath string, stripComponents int) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		entryPath, skip, err := resolveEntryPath(hdr.Name, destPath, stripComponents)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(entryPath, os.FileMode(hdr.Mode)|0o700); err != nil {
				return fmt.Errorf("create directory %s: %w", entryPath, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
				return fmt.Errorf("create parent directory: %w", err)
			}
			if err := writeEntry(tr, entryPath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
				return fmt.Errorf("create parent directory: %w", err)
			}
			if err := os.Symlink(hdr.Linkname, entryPath); err != nil {
				return fmt.Errorf("create symlink %s: %w", entryPath, err)
			}
		}
	}
	return nil
}

// extractZip extracts a zip archive from r (size bytes) into destPath,
// stripping the first stripComponents path components from each entry name.
func extractZip(r io.ReaderAt, size int64, destPath string, stripComponents int) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	for _, f := range zr.File {
		entryPath, skip, err := resolveEntryPath(f.Name, destPath, stripComponents)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(entryPath, f.Mode()|0o700); err != nil {
				return fmt.Errorf("create directory %s: %w", entryPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		writeErr := writeEntry(rc, entryPath, f.Mode())
		_ = rc.Close()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// resolveEntryPath strips path components from name and joins it under
// destPath, rejecting any path that would escape destPath (path traversal).
// Returns (resolvedPath, skip, error): skip is true when name has fewer
// components than stripComponents (the entry should be omitted).
func resolveEntryPath(name, destPath string, stripComponents int) (string, bool, error) {
	// Normalise: remove trailing slash so directories and files are handled
	// the same way when counting components.
	name = strings.TrimSuffix(filepath.ToSlash(name), "/")
	parts := strings.SplitN(name, "/", -1)

	// Filter empty segments produced by leading slashes.
	var clean []string
	for _, p := range parts {
		if p != "" && p != "." {
			clean = append(clean, p)
		}
	}

	if len(clean) <= stripComponents {
		return "", true, nil // entry has fewer components than requested — skip
	}
	clean = clean[stripComponents:]

	// Reject path traversal.
	for _, p := range clean {
		if p == ".." {
			return "", true, fmt.Errorf("archive entry %q contains path traversal", name)
		}
	}

	resolved := filepath.Join(append([]string{destPath}, clean...)...)

	// Double-check the resolved path is still under destPath.
	absRoot, err := filepath.Abs(destPath)
	if err != nil {
		return "", true, err
	}
	absEntry, err := filepath.Abs(resolved)
	if err != nil {
		return "", true, err
	}
	if !strings.HasPrefix(absEntry, absRoot+string(filepath.Separator)) && absEntry != absRoot {
		return "", true, fmt.Errorf("archive entry %q escapes destination directory", name)
	}

	return resolved, false, nil
}

// writeEntry writes from r to path with the given permissions.
func writeEntry(r io.Reader, path string, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	return out.Sync()
}
