package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FileWriter appends log lines to a single pod's log file.
// It is safe for concurrent use; all writes are serialised by a mutex.
type FileWriter struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// NewFileWriter opens (or creates) the log file at
// <logsRoot>/<namespace>/<pod>.log, creating the parent directory if needed.
func NewFileWriter(logsRoot, namespace, pod string) (*FileWriter, error) {
	dir := filepath.Join(logsRoot, namespace)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir %q: %w", dir, err)
	}

	path := filepath.Join(dir, pod+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	return &FileWriter{path: path, f: f}, nil
}

// Write appends line followed by a newline to the log file.
func (w *FileWriter) Write(line string) error {
	_, _, err := w.WriteWithLocation(line)
	return err
}

// WriteWithLocation appends a line and returns its byte offset and length in
// the canonical log file. Indexes store this compact location instead of a
// second copy of the complete log line.
func (w *FileWriter) WriteWithLocation(line string) (int64, uint32, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	offset, err := w.f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0, err
	}
	written, err := fmt.Fprintln(w.f, line)
	if err != nil {
		return 0, 0, err
	}
	if written == 0 {
		return 0, 0, io.ErrShortWrite
	}
	return offset, uint32(written - 1), nil
}

// HasContent reports whether the log file already contains data on disk.
// Used to decide whether to write a restart separator and whether to skip
// replaying historical logs from the Kubernetes API.
func (w *FileWriter) HasContent() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.f.Stat()
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// Close syncs and closes the underlying file.
func (w *FileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("sync log file %q: %w", w.path, err)
	}
	return w.f.Close()
}
