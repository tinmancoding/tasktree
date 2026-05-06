package materialize

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// Local materializes a local source by creating a symlink or recursive copy
// at destPath pointing to (or containing the contents of) spec.SourcePath.
// root is the tasktree root directory, used to resolve relative sourcePaths.
func Local(root, destPath string, spec *domain.LocalSourceSpec) error {
	srcPath := spec.SourcePath
	if !filepath.IsAbs(srcPath) {
		srcPath = filepath.Join(root, srcPath)
	}

	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return domain.LocalSourceNotFoundError{Path: spec.SourcePath}
		}
		return err
	}

	if spec.Copy {
		return copyPath(srcPath, destPath)
	}
	return os.Symlink(srcPath, destPath)
}

// copyPath copies src to dst. If src is a directory it is copied recursively;
// if src is a regular file it is copied directly.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(dstPath, info.Mode())
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, dstPath, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}
	return out.Sync()
}
