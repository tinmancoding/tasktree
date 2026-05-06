package materialize_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/materialize"
)

// buildTarGz creates an in-memory tar.gz with a single file at entryPath
// containing content.
func buildTarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range entries {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar write header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// buildZip creates an in-memory zip archive.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestArchiveExtractsTarGz(t *testing.T) {
	archive := buildTarGz(t, map[string]string{
		"file.txt":       "hello",
		"sub/nested.txt": "nested",
	})

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "extracted")
	spec := &domain.ArchiveSourceSpec{URL: srv.URL + "/archive.tar.gz"}

	if err := materialize.Archive(t.Context(), destPath, spec); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	for _, rel := range []string{"file.txt", filepath.Join("sub", "nested.txt")} {
		if _, err := os.Stat(filepath.Join(destPath, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestArchiveExtractsZip(t *testing.T) {
	archive := buildZip(t, map[string]string{
		"a.txt":   "aaa",
		"b/c.txt": "bbb",
	})

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "extracted")
	spec := &domain.ArchiveSourceSpec{URL: srv.URL + "/archive.zip"}

	if err := materialize.Archive(t.Context(), destPath, spec); err != nil {
		t.Fatalf("Archive zip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destPath, "a.txt")); err != nil {
		t.Fatalf("stat a.txt: %v", err)
	}
}

func TestArchiveStripComponents(t *testing.T) {
	// GitHub-style archive: all files under "repo-v1.0/"
	archive := buildTarGz(t, map[string]string{
		"repo-v1.0/README.md":   "readme",
		"repo-v1.0/src/main.go": "package main",
	})

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "extracted")
	spec := &domain.ArchiveSourceSpec{
		URL:             srv.URL + "/archive.tar.gz",
		StripComponents: 1,
	}

	if err := materialize.Archive(t.Context(), destPath, spec); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// After stripping "repo-v1.0/", README.md should be at destPath/README.md.
	if _, err := os.Stat(filepath.Join(destPath, "README.md")); err != nil {
		t.Fatalf("stat README.md after strip: %v", err)
	}
}

func TestArchiveUnknownFormatError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("garbage"))
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = orig }()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "extracted")
	// URL has no recognisable extension and no format field.
	spec := &domain.ArchiveSourceSpec{URL: srv.URL + "/archive"}

	err := materialize.Archive(t.Context(), destPath, spec)
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
	var fmtErr domain.UnknownArchiveFormatError
	if !isError[domain.UnknownArchiveFormatError](err, &fmtErr) {
		t.Fatalf("expected UnknownArchiveFormatError, got %T: %v", err, err)
	}
}
