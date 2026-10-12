package fsx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type NotInTasktreeError struct{ Start string }

func (e NotInTasktreeError) Error() string { return fmt.Sprintf("not in a tasktree from %s", e.Start) }
func ResolveTasktreeRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		marker := filepath.Join(current, ".tasktree", "config.yml")
		info, statErr := os.Stat(marker)
		if statErr == nil && !info.IsDir() {
			return current, nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", NotInTasktreeError{start}
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
