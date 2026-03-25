package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/gitx"
)

type Manager struct {
	rootPath string
	git      gitx.Client
}

func NewManager(rootPath string, git gitx.Client) Manager {
	return Manager{rootPath: rootPath, git: git}
}

func DefaultRoot() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(userCacheDir, "tasktree", "repos"), nil
}

func (m Manager) Root() string {
	return m.rootPath
}

func (m Manager) PathForURL(repoURL string) string {
	return PathForURL(m.rootPath, repoURL)
}

func (m Manager) Ensure(ctx context.Context, repoURL string) (string, error) {
	if err := os.MkdirAll(m.rootPath, 0o755); err != nil {
		return "", fmt.Errorf("create cache root: %w", err)
	}
	cachePath := m.PathForURL(repoURL)
	exists, err := fsx.Exists(cachePath)
	if err != nil {
		return "", fmt.Errorf("check cache path: %w", err)
	}
	if !exists {
		if err := m.git.CloneBare(ctx, repoURL, cachePath); err != nil {
			return "", err
		}
		return cachePath, nil
	}
	if err := m.git.FetchAllPrune(ctx, cachePath); err != nil {
		return "", err
	}
	return cachePath, nil
}
