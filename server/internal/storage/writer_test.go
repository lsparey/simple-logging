package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSegmentWriter_RecordsPodMeta(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "mynamespace", "my-pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	meta, err := ReadPodMeta(dir, "mynamespace", "my-pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if len(meta.Containers) != 1 || meta.Containers[0] != "app" {
		t.Errorf("expected containers = [app], got %v", meta.Containers)
	}
	if meta.FirstSeen.IsZero() || meta.LastSeen.IsZero() {
		t.Error("expected FirstSeen/LastSeen to be set")
	}
}

func TestSegmentWriter_Write(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	ts := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	if ok := w.Write(ts, "hello world"); !ok {
		t.Fatalf("Write: write failed")
	}

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Errorf("unexpected file content: %q", string(content))
	}
}

func TestSegmentWriter_RollsOverAtUTCDayBoundary(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	day1 := time.Date(2026, 5, 20, 23, 59, 59, 0, time.UTC)
	day2 := time.Date(2026, 5, 21, 0, 0, 1, 0, time.UTC)

	if ok := w.Write(day1, "last line of day 1"); !ok {
		t.Fatalf("Write day1: write failed")
	}
	if ok := w.Write(day2, "first line of day 2"); !ok {
		t.Fatalf("Write day2: write failed")
	}

	content1, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile day1: %v", err)
	}
	if string(content1) != "last line of day 1\n" {
		t.Errorf("day1 segment content = %q", string(content1))
	}

	content2, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-21.log"))
	if err != nil {
		t.Fatalf("ReadFile day2: %v", err)
	}
	if string(content2) != "first line of day 2\n" {
		t.Errorf("day2 segment content = %q", string(content2))
	}

	segments, err := ListSegments(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if want := []string{"2026-05-20", "2026-05-21"}; len(segments) != 2 || segments[0] != want[0] || segments[1] != want[1] {
		t.Errorf("ListSegments = %v, want %v", segments, want)
	}
}

func TestSegmentWriter_OutOfOrderTimestampReturnsToEarlierSegment(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	day1 := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	if ok := w.Write(day1, "day1 first"); !ok {
		t.Fatalf("Write: write failed")
	}
	if ok := w.Write(day2, "day2 only"); !ok {
		t.Fatalf("Write: write failed")
	}
	// A late-arriving line for day1 (e.g. reconnect replay) should append to
	// day1's segment rather than staying stuck on day2's.
	if ok := w.Write(day1, "day1 second"); !ok {
		t.Fatalf("Write: write failed")
	}

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "day1 first\nday1 second\n" {
		t.Errorf("day1 segment content = %q", string(content))
	}
}

func TestSegmentWriter_WriteWithLocation(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	ts := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	seg1, off1, len1, ok := w.WriteWithLocation(ts, "first")
	if !ok {
		t.Fatalf("WriteWithLocation(first): write failed")
	}
	seg2, off2, len2, ok := w.WriteWithLocation(ts, "second")
	if !ok {
		t.Fatalf("WriteWithLocation(second): write failed")
	}
	if seg1 != "2026-05-20" || seg2 != "2026-05-20" {
		t.Fatalf("segments = (%q, %q), want both 2026-05-20", seg1, seg2)
	}
	if off1 != 0 || len1 != 5 || off2 != 6 || len2 != 6 {
		t.Fatalf("locations = (%d,%d), (%d,%d)", off1, len1, off2, len2)
	}
}

func TestSegmentWriter_HasContent(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	if w.HasContent() {
		t.Error("expected HasContent false for a brand new container")
	}
	if ok := w.Write(time.Now().UTC(), "data"); !ok {
		t.Fatalf("Write: write failed")
	}
	if !w.HasContent() {
		t.Error("expected HasContent true after writing data")
	}
}

func TestSegmentWriter_HasContent_TrueOnReopenWithExistingSegments(t *testing.T) {
	dir := t.TempDir()
	w1, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	if ok := w1.Write(time.Now().UTC(), "data"); !ok {
		t.Fatalf("Write: write failed")
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w2, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter (reopen): %v", err)
	}
	defer w2.Close()
	if !w2.HasContent() {
		t.Error("expected HasContent true when segments already exist on disk")
	}
}

func TestSegmentWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	ts := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if ok := w.Write(ts, "concurrent"); !ok {
				t.Errorf("Write: write failed")
			}
		}()
	}
	wg.Wait()

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
}

// TestSegmentWriter_SequentialWritesPreserveOrder verifies that lines written
// one after another appear in the file in write order, even when all messages
// carry the same timestamp (as happens during rapid app startup).
func TestSegmentWriter_SequentialWritesPreserveOrder(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	ts := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	const n = 10
	for i := 0; i < n; i++ {
		line := fmt.Sprintf("%s startup message %d", ts.Format(time.RFC3339), i)
		if ok := w.Write(ts, line); !ok {
			t.Fatalf("Write(%d): write failed", i)
		}
	}

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(got) != n {
		t.Fatalf("expected %d lines in file, got %d", n, len(got))
	}
	for i, line := range got {
		want := fmt.Sprintf("%s startup message %d", ts.Format(time.RFC3339), i)
		if line != want {
			t.Errorf("line[%d]: got %q, want %q", i, line, want)
		}
	}
}

// TestSegmentWriter_BacksOffAfterWriteFailure verifies that a failed write is
// dropped rather than retried on every subsequent call, and that the writer
// recovers automatically once the backoff window elapses.
func TestSegmentWriter_BacksOffAfterWriteFailure(t *testing.T) {
	origMin, origMax := minWriteBackoff, maxWriteBackoff
	minWriteBackoff = 20 * time.Millisecond
	maxWriteBackoff = 20 * time.Millisecond
	t.Cleanup(func() { minWriteBackoff, maxWriteBackoff = origMin, origMax })

	dir := t.TempDir()
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("NewSegmentWriter: %v", err)
	}
	defer w.Close()

	day1 := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	if ok := w.Write(day1, "day1 line"); !ok {
		t.Fatal("expected first write to succeed")
	}

	// Block day2's segment path with a directory in its place, so opening it
	// as a file fails regardless of the test process's privileges (unlike a
	// permission-based block, which root would bypass).
	containerDir := ContainerDir(dir, "ns", "pod", "app")
	blockedPath := filepath.Join(containerDir, "2026-05-21.log")
	if err := os.Mkdir(blockedPath, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	day2 := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	if ok := w.Write(day2, "should fail"); ok {
		t.Fatal("expected write to fail while the segment path is blocked")
	}

	// Unblock immediately — the very next write should still fail because it
	// fast-fails during the backoff window rather than retrying every call.
	if err := os.Remove(blockedPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ok := w.Write(day2, "still backing off"); ok {
		t.Fatal("expected write to still fail immediately during the backoff window")
	}

	time.Sleep(40 * time.Millisecond) // let the (shrunk) backoff elapse

	if ok := w.Write(day2, "recovered"); !ok {
		t.Fatal("expected write to succeed once the backoff elapsed and the path was clear")
	}

	content, err := os.ReadFile(blockedPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "recovered\n" {
		t.Errorf("segment content = %q, want only the recovered line (dropped writes must not be retried)", content)
	}
}
