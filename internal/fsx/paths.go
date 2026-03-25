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
		metadataPath := filepath.Join(current, domain.MetadataFileName)
		info, statErr := os.Stat(metadataPath)
		if statErr == nil && !info.IsDir() {
			return current, nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
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
