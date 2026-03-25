package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

func PathForURL(root, repoURL string) string {
	sum := sha256.Sum256([]byte(repoURL))
	return filepath.Join(root, hex.EncodeToString(sum[:]))
}
