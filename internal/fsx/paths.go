package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinmancoding/tasktree/internal/domain"
)

func ResolveTasktreeRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		// Check for new Tasktree.yml first.
		specPath := filepath.Join(current, domain.SpecFileName)
		info, statErr := os.Stat(specPath)
		if statErr == nil && !info.IsDir() {
			return current, nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}

		// Check for legacy .tasktree.toml and surface a migration error.
		legacyPath := filepath.Join(current, domain.LegacyFileName)
		legacyInfo, legacyStatErr := os.Stat(legacyPath)
		if legacyStatErr == nil && !legacyInfo.IsDir() {
			return "", domain.LegacyMetadataError{Path: legacyPath}
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", domain.NotInTasktreeError{Start: start}
		}
		current = parent
	}
}

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func IsWithin(root, target string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return false, nil
	}
	return true, nil
}
