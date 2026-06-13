// Package snapshot captures and restores the concrete working state of a
// tasktree workspace as a single portable tar.gz artifact.
package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
)

// Member is a single file entry in the outer snapshot tar.gz.
type Member struct {
	Name string
	Data []byte
	Mode int64
}

// Pack writes the given members as a gzip-compressed tar to w.
func Pack(w io.Writer, members []Member) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		mode := m.Mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     m.Name,
			Mode:     mode,
			Size:     int64(len(m.Data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header %q: %w", m.Name, err)
		}
		if _, err := tw.Write(m.Data); err != nil {
			return fmt.Errorf("write tar body %q: %w", m.Name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip: %w", err)
	}
	return nil
}

// Open reads a snapshot tar.gz fully into a map of member name to bytes.
func Open(r io.Reader) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read tar member %q: %w", hdr.Name, err)
		}
		out[hdr.Name] = data
	}
	return out, nil
}
