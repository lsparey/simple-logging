package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/internal/metrics"
)

func TestAllSegmentsByAge_SortsOldestFirstAcrossNamespacesPodsContainers(t *testing.T) {
	dir := t.TempDir()
	writeSegment(t, dir, "ns-a", "pod-1", "app", "2026-05-18")
	writeSegment(t, dir, "ns-b", "pod-2", "sidecar", "2026-05-16")
	writeSegment(t, dir, "ns-a", "pod-1", "app", "2026-05-20")
	writeSegment(t, dir, "ns-b", "pod-2", "sidecar", "2026-05-17")

	segments, err := allSegmentsByAge(dir)
	if err != nil {
		t.Fatalf("allSegmentsByAge: %v", err)
	}
	if len(segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segments))
	}
	want := []string{"2026-05-16", "2026-05-17", "2026-05-18", "2026-05-20"}
	for i, w := range want {
		if got := segments[i].date.Format("2006-01-02"); got != w {
			t.Errorf("segments[%d] date = %q, want %q", i, got, w)
		}
	}
}

func TestAllSegmentsByAge_EmptyLogsRoot(t *testing.T) {
	dir := t.TempDir()
	segments, err := allSegmentsByAge(filepath.Join(dir, "does-not-exist"))
	if err != nil {
		t.Fatalf("allSegmentsByAge: %v", err)
	}
	if len(segments) != 0 {
		t.Errorf("expected no segments, got %d", len(segments))
	}
}

func TestDiskGuard_DeletesOldestSegmentsUntilBelowLowWaterMark(t *testing.T) {
	dir := t.TempDir()
	// Oldest to newest: day1, day2, day3, day4.
	writeSegment(t, dir, "ns", "pod", "app", "2026-05-17")
	writeSegment(t, dir, "ns", "pod", "app", "2026-05-18")
	writeSegment(t, dir, "ns", "pod", "app", "2026-05-19")
	writeSegment(t, dir, "ns", "pod", "app", "2026-05-20")

	// Simulate usage starting at 95%, dropping 10 points per deleted segment:
	// 95 (>= high 90, keep going) -> 85 (< low 80? no, 85 >= 80, keep going)
	// -> 75 (< 80, stop). Two segments should be deleted.
	usage := []int{95, 85, 75}
	call := 0
	m := metrics.New(nil)
	g := NewDiskGuard(dir, 90, 80, time.Hour, zap.NewNop())
	g.SetMetrics(m)
	g.usedPercent = func(string) (int, error) {
		v := usage[call]
		if call < len(usage)-1 {
			call++
		}
		return v, nil
	}

	g.check()

	if got := m.Snapshot().DiskGuardSegmentsDeleted; got != 2 {
		t.Errorf("DiskGuardSegmentsDeleted: got %d, want 2", got)
	}

	segments, err := ListSegments(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	want := []string{"2026-05-19", "2026-05-20"}
	if len(segments) != len(want) {
		t.Fatalf("remaining segments = %v, want %v", segments, want)
	}
	for i := range want {
		if segments[i] != want[i] {
			t.Errorf("remaining segments = %v, want %v", segments, want)
			break
		}
	}
}

func TestDiskGuard_NoOpBelowHighWaterMark(t *testing.T) {
	dir := t.TempDir()
	path := writeSegment(t, dir, "ns", "pod", "app", "2026-05-20")

	g := NewDiskGuard(dir, 90, 80, time.Hour, zap.NewNop())
	g.usedPercent = func(string) (int, error) { return 50, nil }

	g.check()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected segment to be kept when usage is below the high water mark: %v", err)
	}
}

func TestDiskGuard_StopsWhenNoSegmentsLeftToDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	g := NewDiskGuard(dir, 90, 80, time.Hour, zap.NewNop())
	g.usedPercent = func(string) (int, error) { return 99, nil }

	// Must not panic or hang when there is nothing to delete.
	g.check()
}

func TestDiskGuard_CleansUpEmptyDirsAfterDeletingLastSegment(t *testing.T) {
	dir := t.TempDir()
	writeSegment(t, dir, "ns", "pod", "app", "2026-05-20")
	if err := WritePodMeta(dir, PodMeta{Namespace: "ns", Pod: "pod", Containers: []string{"app"}}); err != nil {
		t.Fatalf("WritePodMeta: %v", err)
	}

	g := NewDiskGuard(dir, 90, 10, time.Hour, zap.NewNop())
	calls := 0
	g.usedPercent = func(string) (int, error) {
		calls++
		if calls == 1 {
			return 95, nil
		}
		return 5, nil // below low water mark after the one deletion
	}

	g.check()

	if _, err := os.Stat(filepath.Join(dir, "ns")); !os.IsNotExist(err) {
		t.Error("expected empty namespace directory to be removed after deleting the last segment")
	}
}

func TestDiskUsedPercent_ReturnsSaneValue(t *testing.T) {
	dir := t.TempDir()
	percent, err := DiskUsedPercent(dir)
	if err != nil {
		t.Fatalf("DiskUsedPercent: %v", err)
	}
	if percent < 0 || percent > 100 {
		t.Errorf("DiskUsedPercent = %d, want a value in [0, 100]", percent)
	}
}

func TestDiskGuard_SkipsSegmentsAWriterHasOpen(t *testing.T) {
	dir := t.TempDir()
	oldest := writeSegment(t, dir, "ns", "pod", "app", "2026-05-17")
	next := writeSegment(t, dir, "ns", "pod", "other", "2026-05-18")

	// A writer with the oldest segment open, as for a quiet container whose
	// last line was that day.
	w, err := NewSegmentWriter(dir, "ns", "pod", "app")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if !w.Write(time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC), "line") {
		t.Fatal("write failed")
	}

	compacted := false
	g := NewDiskGuard(dir, 90, 80, time.Hour, zap.NewNop())
	g.SetIndexCompactor(func() error { compacted = true; return nil })
	usage := []int{95, 70}
	call := 0
	g.usedPercent = func(string) (int, error) {
		v := usage[min(call, len(usage)-1)]
		call++
		return v, nil
	}
	g.check()

	if _, err := os.Stat(oldest); err != nil {
		t.Error("the open segment was deleted")
	}
	if _, err := os.Stat(next); !os.IsNotExist(err) {
		t.Error("expected the oldest closed segment to be deleted instead")
	}
	if !compacted {
		t.Error("expected indexes to be compacted after deleting a segment")
	}
}
