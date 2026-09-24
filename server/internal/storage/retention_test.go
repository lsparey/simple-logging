package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/internal/metrics"
)

func writeSegment(t *testing.T, root, namespace, pod, container, date string) string {
	t.Helper()
	dir := filepath.Join(root, namespace, pod, container)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, date+".log")
	if err := os.WriteFile(path, []byte("log data\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRetentionManager_DeletesOldSegments(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
	oldPath := writeSegment(t, dir, "ns", "pod", "app", oldDate)

	m := metrics.New(nil)
	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.SetMetrics(m)
	rm.sweep()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("expected old segment to be deleted after sweep")
	}
	if got := m.Snapshot().RetentionSegmentsDeleted; got != 1 {
		t.Errorf("RetentionSegmentsDeleted: got %d, want 1", got)
	}
}

func TestRetentionManager_KeepsRecentSegments(t *testing.T) {
	dir := t.TempDir()
	recentDate := time.Now().UTC().Format("2006-01-02")
	newPath := writeSegment(t, dir, "ns", "pod", "app", recentDate)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected recent segment to be kept after sweep: %v", err)
	}
}

func TestRetentionManager_RemovesEmptyContainerPodAndNamespaceDirs(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
	writeSegment(t, dir, "ns", "pod", "app", oldDate)
	if err := WritePodMeta(dir, PodMeta{Namespace: "ns", Pod: "pod", Containers: []string{"app"}}); err != nil {
		t.Fatalf("WritePodMeta: %v", err)
	}

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(filepath.Join(dir, "ns")); !os.IsNotExist(err) {
		t.Error("expected empty namespace directory to be removed after sweep")
	}
}

func TestRetentionManager_KeepsNonEmptyDirsWhenOtherSegmentsRemain(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
	recentDate := time.Now().UTC().Format("2006-01-02")
	writeSegment(t, dir, "ns", "pod", "app", oldDate)
	newPath := writeSegment(t, dir, "ns", "pod", "app", recentDate)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(filepath.Join(dir, "ns")); err != nil {
		t.Errorf("expected namespace dir to remain when non-empty: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected recent segment to remain: %v", err)
	}
}

func TestRetentionManager_KeepsOtherPodsInSameNamespace(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
	writeSegment(t, dir, "ns", "old-pod", "app", oldDate)
	recentDate := time.Now().UTC().Format("2006-01-02")
	newPath := writeSegment(t, dir, "ns", "new-pod", "app", recentDate)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(filepath.Join(dir, "ns", "old-pod")); !os.IsNotExist(err) {
		t.Error("expected expired pod directory to be removed")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected other pod's recent segment to remain: %v", err)
	}
}

func TestRetentionManager_CompactsIndexesAfterDeletingSegments(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
	writeSegment(t, dir, "ns", "pod", "app", oldDate)

	compactCalls := 0
	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.SetIndexCompactor(func() error {
		compactCalls++
		return nil
	})
	rm.sweep()

	if compactCalls != 1 {
		t.Fatalf("index compactor called %d times, want 1", compactCalls)
	}
}

func TestRetentionManager_DoesNotCompactWhenNothingDeleted(t *testing.T) {
	dir := t.TempDir()
	recentDate := time.Now().UTC().Format("2006-01-02")
	writeSegment(t, dir, "ns", "pod", "app", recentDate)

	compactCalls := 0
	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.SetIndexCompactor(func() error {
		compactCalls++
		return nil
	})
	rm.sweep()

	if compactCalls != 0 {
		t.Fatalf("index compactor called %d times, want 0", compactCalls)
	}
}

// TestRetentionManager_SweepsLegacyFilesByMtime covers the MIGRATE_LEGACY=false
// escape hatch: a stray v0.11 <ns>/<pod>.log file has no segment date to key
// off, so it is swept by mtime exactly as it was before Phase 1.
func TestRetentionManager_SweepsLegacyFilesByMtime(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	oldPath := filepath.Join(nsDir, "legacy-pod.log")
	if err := os.WriteFile(oldPath, []byte("old log data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	past := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("expected old legacy log file to be deleted after sweep")
	}
}

func TestRetentionManager_KeepsRecentLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	newPath := filepath.Join(nsDir, "legacy-pod.log")
	if err := os.WriteFile(newPath, []byte("recent log data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected recent legacy log file to be kept after sweep: %v", err)
	}
}

func TestNextDailySweep(t *testing.T) {
	for _, tc := range []struct{ now, want string }{
		{"2026-09-24T12:00:00Z", "2026-09-25T00:05:00Z"},
		{"2026-09-24T00:04:59Z", "2026-09-24T00:05:00Z"},
		{"2026-09-24T00:05:00Z", "2026-09-25T00:05:00Z"},
		{"2026-12-31T23:59:00Z", "2027-01-01T00:05:00Z"},
		// Non-UTC input is still scheduled against UTC midnight.
		{"2026-09-24T01:00:00+02:00", "2026-09-24T00:05:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := nextDailySweep(now); !got.Equal(want) {
			t.Errorf("nextDailySweep(%s) = %s, want %s", tc.now, got.UTC().Format(time.RFC3339), tc.want)
		}
	}
}

func TestRetentionManager_KeepsMetaOfActivePodsWithNoSegmentsLeft(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02")
	writeSegment(t, dir, "ns", "quiet", "app", oldDate)
	writeSegment(t, dir, "ns", "gone", "app", oldDate)
	for _, pod := range []string{"quiet", "gone"} {
		if err := RecordOwner(dir, "ns", pod, "Deployment", "web", ""); err != nil {
			t.Fatal(err)
		}
	}

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.SetActiveChecker(func(_, pod string) bool { return pod == "quiet" })
	rm.sweep()

	if meta, err := ReadPodMeta(dir, "ns", "quiet"); err != nil || meta.OwnerName != "web" {
		t.Errorf("active pod's meta.json should survive with its owner, got %+v, %v", meta, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ns", "gone")); !os.IsNotExist(err) {
		t.Error("an inactive pod with no segments left should be removed entirely")
	}
}
