package store

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync"
)

const (
	blobContentGzip = 1 << iota
	blobSyntaxGzip
	blobFileGzip
	blobGzipMask = blobContentGzip | blobSyntaxGzip | blobFileGzip
)

var blobWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
	return w
}}

// Keep short and incompressible payloads verbatim. The explicit per-column
// flags distinguish legacy bytes (including gzip-looking source files) from
// compressed data without changing content hashes or cache formats.
func compressBlob(raw []byte, flag int) ([]byte, int, error) {
	if len(raw) < 256 {
		return raw, 0, nil
	}
	var buf bytes.Buffer
	w := blobWriters.Get().(*gzip.Writer)
	defer func() {
		w.Reset(io.Discard)
		blobWriters.Put(w)
	}()
	w.Reset(&buf)
	if _, err := w.Write(raw); err != nil {
		return nil, 0, err
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}
	if buf.Len() >= len(raw) {
		return raw, 0, nil
	}
	return buf.Bytes(), flag, nil
}

func decompressBlob(raw []byte, compression, flag int) ([]byte, error) {
	if compression & ^blobGzipMask != 0 {
		return nil, fmt.Errorf("unsupported blob compression flags: %d", compression)
	}
	if compression&flag == 0 {
		return raw, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open compressed blob: %w", err)
	}
	defer func() { _ = r.Close() }()
	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decode compressed blob: %w", err)
	}
	return decoded, nil
}
