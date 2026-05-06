package materialize_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/materialize"
)

func TestHTTPDownloadsFile(t *testing.T) {
	content := []byte("hello from http")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	// Swap the default transport to trust the test server's TLS cert.
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.txt")
	spec := &domain.HTTPSourceSpec{URL: srv.URL + "/file.txt"}

	if err := materialize.HTTP(t.Context(), destPath, spec); err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content = %q, want %q", string(got), string(content))
	}
}

func TestHTTPVerifiesSHA256(t *testing.T) {
	content := []byte("checksum-me")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.txt")

	// sha256 of "checksum-me" (echo -n "checksum-me" | shasum -a 256):
	correctHash := "ea5943363841d1932d98a4ce4c38365bda0d33701dcfc00422363ddf6d150001"
	spec := &domain.HTTPSourceSpec{URL: srv.URL + "/file.txt", SHA256: correctHash}

	if err := materialize.HTTP(t.Context(), destPath, spec); err != nil {
		t.Fatalf("HTTP with correct sha256: %v", err)
	}

	// Now try with a wrong hash.
	_ = os.Remove(destPath)
	spec.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	err := materialize.HTTP(t.Context(), destPath, spec)
	if err == nil {
		t.Fatal("expected SHA256 mismatch error, got nil")
	}
	var mismatch domain.SHA256MismatchError
	if !isError[domain.SHA256MismatchError](err, &mismatch) {
		t.Fatalf("expected SHA256MismatchError, got %T: %v", err, err)
	}
	// File should not exist after mismatch.
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatal("expected destPath to not exist after sha256 mismatch")
	}
}

func TestHTTPForwardsHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.txt")
	spec := &domain.HTTPSourceSpec{
		URL:     srv.URL + "/file.txt",
		Headers: map[string]string{"Authorization": "Bearer token123"},
	}

	if err := materialize.HTTP(t.Context(), destPath, spec); err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	if gotAuth != "Bearer token123" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer token123")
	}
}

func TestHTTPRejectsHTTPScheme(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.txt")
	spec := &domain.HTTPSourceSpec{URL: "http://example.com/file.txt"}

	err := materialize.HTTP(t.Context(), destPath, spec)
	if err == nil {
		t.Fatal("expected error for http:// URL, got nil")
	}
	var schemeErr domain.InvalidHTTPSSchemeError
	if !isError[domain.InvalidHTTPSSchemeError](err, &schemeErr) {
		t.Fatalf("expected InvalidHTTPSSchemeError, got %T: %v", err, err)
	}
}
