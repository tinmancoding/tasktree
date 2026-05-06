package materialize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// downloadToFile fetches url into destPath, verifying the optional SHA-256
// digest. destPath is written atomically via a sibling temp file.
func downloadToFile(ctx context.Context, url, expectedSHA256, destPath string) error {
	if !strings.HasPrefix(url, "https://") {
		return domain.InvalidHTTPSSchemeError{URL: url}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %d %s", url, resp.StatusCode, resp.Status)
	}

	return writeWithOptionalVerify(resp.Body, url, expectedSHA256, destPath)
}

// downloadToFileWithHeaders is like downloadToFile but injects extra headers.
func downloadToFileWithHeaders(ctx context.Context, url string, headers map[string]string, expectedSHA256, destPath string) error {
	if !strings.HasPrefix(url, "https://") {
		return domain.InvalidHTTPSSchemeError{URL: url}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %d %s", url, resp.StatusCode, resp.Status)
	}

	return writeWithOptionalVerify(resp.Body, url, expectedSHA256, destPath)
}

// writeWithOptionalVerify streams r into destPath via an atomic temp-file
// write, computing a SHA-256 digest along the way. If expectedSHA256 is
// non-empty and the digest does not match, the temp file is removed and a
// SHA256MismatchError is returned.
func writeWithOptionalVerify(r io.Reader, url, expectedSHA256, destPath string) error {
	tmp, err := os.CreateTemp(destPath+"_", ".tasktree-dl-*.tmp")
	if err != nil {
		// Fallback: use system temp dir if the parent does not exist yet.
		tmp, err = os.CreateTemp("", ".tasktree-dl-*.tmp")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	w := io.MultiWriter(tmp, h)
	if _, err := io.Copy(w, r); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if expectedSHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != strings.ToLower(expectedSHA256) {
			return domain.SHA256MismatchError{URL: url, Expected: strings.ToLower(expectedSHA256), Got: got}
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
