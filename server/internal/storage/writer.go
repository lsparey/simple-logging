package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SegmentWriter appends log lines for one pod's container to daily segment
// files under <logsRoot>/<namespace>/<pod>/<container>/<date>.log. It rolls
// over to a new file whenever a written line's own timestamp crosses a UTC
// day boundary, so retention can delete a whole day's history as a single
// file removal instead of rewriting a shared file.
type SegmentWriter struct {
	mu sync.Mutex

	logsRoot                  string
	namespace, pod, container string

	f           *os.File
	segmentDate string // "YYYY-MM-DD" of the currently open segment, "" if none open yet

	hadExistingContent bool
}

// NewSegmentWriter opens a writer for namespace/pod/container. It does not
// create a segment file until the first write, since the segment depends on
// that write's timestamp.
func NewSegmentWriter(logsRoot, namespace, pod, container string) (*SegmentWriter, error) {
	segments, err := ListSegments(logsRoot, namespace, pod, container)
	if err != nil {
		return nil, fmt.Errorf("list segments for %s/%s/%s: %w", namespace, pod, container, err)
	}

	w := &SegmentWriter{
		logsRoot:           logsRoot,
		namespace:          namespace,
		pod:                pod,
		container:          container,
		hadExistingContent: len(segments) > 0,
	}

	if err := recordContainerSeen(logsRoot, namespace, pod, container, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("record pod meta: %w", err)
	}

	return w, nil
}

// Write appends line, stamped with lineTS, to the appropriate segment.
func (w *SegmentWriter) Write(lineTS time.Time, line string) error {
	_, _, _, err := w.WriteWithLocation(lineTS, line)
	return err
}

// WriteWithLocation appends line to the segment for lineTS's UTC date,
// rolling over from any previously open segment first. It returns the
// segment's date ("YYYY-MM-DD") and the line's byte offset and length within
// that segment file, which indexes store instead of a second copy of the line.
func (w *SegmentWriter) WriteWithLocation(lineTS time.Time, line string) (segment string, offset int64, length uint32, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	date := lineTS.UTC().Format(segmentDateForm)
	if w.f == nil || date != w.segmentDate {
		if err := w.rollToLocked(date); err != nil {
			return "", 0, 0, err
		}
	}

	off, err := w.f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", 0, 0, err
	}
	written, err := fmt.Fprintln(w.f, line)
	if err != nil {
		return "", 0, 0, err
	}
	if written == 0 {
		return "", 0, 0, io.ErrShortWrite
	}
	return w.segmentDate, off, uint32(written - 1), nil
}

// rollToLocked closes the currently open segment (if any) and opens the
// segment file for date, creating the container directory if needed. mu must
// be held by the caller.
func (w *SegmentWriter) rollToLocked(date string) error {
	if w.f != nil {
		if err := w.f.Close(); err != nil {
			return fmt.Errorf("close previous segment: %w", err)
		}
	}

	dir := ContainerDir(w.logsRoot, w.namespace, w.pod, w.container)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create container dir %q: %w", dir, err)
	}

	path := filepath.Join(dir, date+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open segment %q: %w", path, err)
	}

	w.f = f
	w.segmentDate = date
	w.hadExistingContent = true
	return nil
}

// HasContent reports whether this pod/container already had log segments on
// disk before this writer was constructed (or has since written any). Used to
// decide whether to write a restart separator and whether to skip replaying
// historical logs from the Kubernetes API.
func (w *SegmentWriter) HasContent() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hadExistingContent
}

// Close syncs and closes the currently open segment, if any.
func (w *SegmentWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("sync segment: %w", err)
	}
	return w.f.Close()
}
