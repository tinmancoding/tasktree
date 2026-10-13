package product_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/product"
)

func TestNameIsTasktree(t *testing.T) {
	if product.Name != "tasktree" {
		t.Fatalf("Name = %q", product.Name)
	}
}

// User-facing text must go through product.Name (DN9); a stray hard-coded
// "tt2" in a message or hint is a regression.
func TestNoHardCodedLegacyBinaryName(t *testing.T) {
	legacy := regexp.MustCompile(`\btt2\b`)
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "product") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "//") {
				continue
			}
			code = strings.ReplaceAll(code, "tt2-", "")
			if legacy.MatchString(code) {
				t.Errorf("%s:%d hard-codes tt2: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
