package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// minWriteBackoff and maxWriteBackoff bound how long a SegmentWriter waits
// before retrying after a write failure (e.g. a temporarily full or
// read-only volume). They are vars, not consts, so tests can shrink them.
var (
	minWriteBackoff = time.Second
	maxWriteBackoff = 30 * time.Second
)

// SegmentWriter appends log lines for one pod's container to daily segment
// files under <logsRoot>/<namespace>/<pod>/<container>/<date>.log. It rolls
// over to a new file whenever a written line's own timestamp crosses a UTC
// day boundary, so retention can delete a whole day's history as a single
// file removal instead of rewriting a shared file.
//
// Write failures are treated as transient: rather than propagating an error
// that would end the caller's stream goroutine, a failed write is dropped and
// the writer backs off (doubling from minWriteBackoff up to maxWriteBackoff)
// before it will attempt another write, so a persistently failing volume is
// not hammered once per log line.
type SegmentWriter struct {
	mu sync.Mutex

	logsRoot                  string
	namespace, pod, container string

	f           *os.File
	segmentDate string // "YYYY-MM-DD" of the currently open segment, "" if none open yet

	hadExistingContent bool

	unhealthyUntil time.Time
	writeBackoff   time.Duration
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

// Write appends line, stamped with lineTS, to the appropriate segment. It
// reports whether the write succeeded; a false return means the line was
// dropped (write failure, or still within the post-failure backoff window).
func (w *SegmentWriter) Write(lineTS time.Time, line string) bool {
	_, _, _, ok := w.WriteWithLocation(lineTS, line)
	return ok
}

// WriteWithLocation appends line to the segment for lineTS's UTC date,
// rolling over from any previously open segment first. It returns the
// segment's date ("YYYY-MM-DD") and the line's byte offset and length within
// that segment file, which indexes store instead of a second copy of the
// line, plus whether the write succeeded.
//
// A failed write drops the line and starts (or extends) a backoff window:
// calls made before the window elapses fail fast without touching the
// filesystem, so a persistently failing volume costs at most one real
// attempt per backoff period rather than one per line.
func (w *SegmentWriter) WriteWithLocation(lineTS time.Time, line string) (segment string, offset int64, length uint32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	if now.Before(w.unhealthyUntil) {
		return "", 0, 0, false
	}

	date := lineTS.UTC().Format(segmentDateForm)
	if w.f == nil || date != w.segmentDate {
		if err := w.rollToLocked(date); err != nil {
			w.markUnhealthyLocked(now)
			return "", 0, 0, false
		}
	}

	off, err := w.f.Seek(0, io.SeekEnd)
	var written int
	if err == nil {
		written, err = fmt.Fprintln(w.f, line)
		if err == nil && written == 0 {
			err = io.ErrShortWrite
		}
	}
	if err != nil {
		// Force a reopen on the next attempt — the fd may be in a bad state.
		_ = w.f.Close()
		w.f = nil
		w.markUnhealthyLocked(now)
		return "", 0, 0, false
	}

	w.writeBackoff = 0
	return w.segmentDate, off, uint32(written - 1), true
}

// markUnhealthyLocked starts or extends the write backoff window. mu must be
// held by the caller.
func (w *SegmentWriter) markUnhealthyLocked(now time.Time) {
	if w.writeBackoff < minWriteBackoff {
		w.writeBackoff = minWriteBackoff
	}
	w.unhealthyUntil = now.Add(w.writeBackoff)
	w.writeBackoff *= 2
	if w.writeBackoff > maxWriteBackoff {
		w.writeBackoff = maxWriteBackoff
	}
}

// rollToLocked closes the currently open segment (if any) and opens the
// segment file for date, creating the container directory if needed. mu must
// be held by the caller.
func (w *SegmentWriter) rollToLocked(date string) error {
	if w.f != nil {
		// Sync the finished segment so it's durable, not just the last one
		// Close syncs: the legacy migration deletes its source file once the
		// writer closes, relying on every segment it wrote being on disk.
		// This costs one fsync per container per day.
		//
		// Clear w.f before checking the errors: if opening the new segment
		// below then fails, w.f must not be left pointing at this now-closed
		// handle, or the next attempt would double-close it and fail every
		// time regardless of whether the real problem cleared.
		syncErr := w.f.Sync()
		closeErr := w.f.Close()
		w.f = nil
		if syncErr != nil {
			return fmt.Errorf("sync previous segment: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close previous segment: %w", closeErr)
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
