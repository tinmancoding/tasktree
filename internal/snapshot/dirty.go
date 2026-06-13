package snapshot

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
)

// BuildDirtyTar builds an uncompressed tar of the working-tree content of every
// changed/untracked path, plus a DirtyManifest member recording deletions and
// the staged-path re-stage hint. It returns (nil, false, nil) when the working
// tree is clean (no status entries).
func BuildDirtyTar(srcPath string, entries []gitx.StatusEntry) ([]byte, bool, error) {
	if len(entries) == 0 {
		return nil, false, nil
	}

	captureSet := make(map[string]struct{})
	deletedSet := make(map[string]struct{})
	stagedSet := make(map[string]struct{})

	consider := func(rel string) error {
		if rel == "" {
			return nil
		}
		abs := filepath.Join(srcPath, rel)
		if _, err := os.Lstat(abs); err != nil {
			if os.IsNotExist(err) {
				deletedSet[rel] = struct{}{}
				return nil
			}
			return err
		}
		captureSet[rel] = struct{}{}
		return nil
	}

	for _, e := range entries {
		if e.Staged() {
			stagedSet[e.Path] = struct{}{}
		}
		if err := consider(e.Path); err != nil {
			return nil, false, err
		}
		if err := consider(e.OrigPath); err != nil {
			return nil, false, err
		}
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, rel := range sortedKeys(captureSet) {
		if err := addPathToTar(tw, srcPath, rel); err != nil {
			return nil, false, err
		}
	}

	dm := domain.DirtyManifest{
		Deleted: sortedKeys(deletedSet),
		Staged:  sortedKeys(stagedSet),
	}
	dmBytes, err := yaml.Marshal(dm)
	if err != nil {
		return nil, false, fmt.Errorf("encode dirty manifest: %w", err)
	}
	if err := writeTarReg(tw, domain.DirtyManifestName, dmBytes, 0o644); err != nil {
		return nil, false, err
	}

	if err := tw.Close(); err != nil {
		return nil, false, fmt.Errorf("close dirty tar: %w", err)
	}
	return buf.Bytes(), true, nil
}

// addPathToTar adds a file, symlink, or directory (walked recursively) at rel
// (relative to srcPath) to the tar, preserving file mode and symlink targets.
func addPathToTar(tw *tar.Writer, srcPath, rel string) error {
	abs := filepath.Join(srcPath, rel)
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(abs)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: rel, Typeflag: tar.TypeSymlink, Linkname: target, Mode: int64(info.Mode().Perm())}
		return tw.WriteHeader(hdr)
	case info.IsDir():
		return filepath.Walk(abs, func(p string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if fi.IsDir() {
				return nil
			}
			childRel, err := filepath.Rel(srcPath, p)
			if err != nil {
				return err
			}
			return addPathToTar(tw, srcPath, childRel)
		})
	default:
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		return writeTarReg(tw, rel, data, int64(info.Mode().Perm()))
	}
}

func writeTarReg(tw *tar.Writer, name string, data []byte, mode int64) error {
	hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write dirty header %q: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write dirty body %q: %w", name, err)
	}
	return nil
}

// UnpackDirtyTar restores dirty content into targetSrc, then applies the
// deletions and re-stage hint via the supplied git client.
func UnpackDirtyTar(srcPath string, dirtyTar []byte) (domain.DirtyManifest, error) {
	var dm domain.DirtyManifest
	tr := tar.NewReader(bytes.NewReader(dirtyTar))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return dm, fmt.Errorf("read dirty tar: %w", err)
		}
		if hdr.Name == domain.DirtyManifestName {
			data, err := io.ReadAll(tr)
			if err != nil {
				return dm, fmt.Errorf("read dirty manifest: %w", err)
			}
			if err := yaml.Unmarshal(data, &dm); err != nil {
				return dm, fmt.Errorf("parse dirty manifest: %w", err)
			}
			continue
		}
		abs := filepath.Join(srcPath, filepath.FromSlash(hdr.Name))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return dm, err
		}
		switch hdr.Typeflag {
		case tar.TypeSymlink:
			_ = os.Remove(abs)
			if err := os.Symlink(hdr.Linkname, abs); err != nil {
				return dm, fmt.Errorf("restore symlink %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return dm, fmt.Errorf("read dirty member %q: %w", hdr.Name, err)
			}
			if err := os.WriteFile(abs, data, os.FileMode(hdr.Mode)); err != nil {
				return dm, fmt.Errorf("restore file %q: %w", hdr.Name, err)
			}
		}
	}
	return dm, nil
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
