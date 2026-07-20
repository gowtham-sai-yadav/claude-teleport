// Package bundle reads and writes the portable .tgz archive that carries a
// user's Claude Code data between machines. Layout inside the archive:
//
//	manifest.json                       (always first, so it reads fast)
//	config/claude.json                  (sanitised ~/.claude.json)
//	config/settings.json
//	config/settings.local.json
//	config/history.jsonl
//	plans/...
//	plugins/installed_plugins.json
//	plugins/known_marketplaces.json
//	projects/<encodedFolder>/...        (transcripts, sidecars, memory)
package bundle

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"time"
)

type Writer struct {
	c  io.Closer // underlying file to close, or nil for an in-memory/stream writer
	gz *gzip.Writer
	tw *tar.Writer
}

// NewWriter writes a bundle to any destination: a file, an in-memory buffer, or
// a network stream. Close flushes the gzip and tar framing but leaves the
// destination open (Create wires up closing the file it opened).
func NewWriter(w io.Writer) *Writer {
	gz := gzip.NewWriter(w)
	return &Writer{gz: gz, tw: tar.NewWriter(gz)}
}

func Create(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := NewWriter(f)
	w.c = f
	return w, nil
}

func (w *Writer) AddBytes(name string, data []byte) error {
	h := &tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
		ModTime:  time.Now(),
	}
	if err := w.tw.WriteHeader(h); err != nil {
		return err
	}
	_, err := w.tw.Write(data)
	return err
}

func (w *Writer) AddFile(name, src string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	h := &tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     info.Size(),
		Typeflag: tar.TypeReg,
		ModTime:  info.ModTime(),
	}
	if err := w.tw.WriteHeader(h); err != nil {
		return err
	}
	_, err = io.Copy(w.tw, f)
	return err
}

func (w *Writer) Close() error {
	if err := w.tw.Close(); err != nil {
		return err
	}
	if err := w.gz.Close(); err != nil {
		return err
	}
	if w.c != nil {
		return w.c.Close()
	}
	return nil
}

// ErrStop lets a ForEach callback end iteration early without it being treated
// as a failure.
var ErrStop = errors.New("stop")

func ForEach(path string, fn func(*tar.Header, io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if err := fn(h, tr); err != nil {
			return err
		}
	}
	return nil
}

// ReadManifest returns the raw manifest.json bytes. Because the manifest is
// written first, this stops after the first entry.
func ReadManifest(path string) ([]byte, error) {
	var out []byte
	err := ForEach(path, func(h *tar.Header, r io.Reader) error {
		if h.Name == "manifest.json" {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			out = b
			return ErrStop
		}
		return nil
	})
	if errors.Is(err, ErrStop) {
		err = nil
	}
	return out, err
}
