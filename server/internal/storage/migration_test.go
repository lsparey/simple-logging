package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func writeLegacyPodFile(t *testing.T, root, namespace, pod string, lines []string) {
	t.Helper()
	dir := filepath.Join(root, namespace)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, pod+".log"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		fmt.Fprintln(f, line)
	}
}

func TestMigrateLegacyLayout_ConvertsSingleDaySegment(t *testing.T) {
	dir := t.TempDir()
	writeLegacyPodFile(t, dir, "default", "api", []string{
		"2026-05-20T10:00:00Z [default/api/app] first",
		"2026-05-20T10:00:01Z [default/api/app] second",
	})

	if err := MigrateLegacyLayout(dir, zap.NewNop()); err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "default", "api.log")); !os.IsNotExist(err) {
		t.Error("expected legacy file to be removed after migration")
	}

	content, err := os.ReadFile(filepath.Join(dir, "default", "api", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile migrated segment: %v", err)
	}
	want := "2026-05-20T10:00:00Z [default/api/app] first\n2026-05-20T10:00:01Z [default/api/app] second\n"
	if string(content) != want {
		t.Errorf("migrated segment content = %q, want %q", string(content), want)
	}

	meta, err := ReadPodMeta(dir, "default", "api")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if len(meta.Containers) != 1 || meta.Containers[0] != "app" {
		t.Errorf("expected containers = [app], got %v", meta.Containers)
	}
}

func TestMigrateLegacyLayout_SplitsMultiDayFileIntoSegments(t *testing.T) {
	dir := t.TempDir()
	writeLegacyPodFile(t, dir, "default", "api", []string{
		"2026-05-20T23:59:59Z [default/api/app] last of day 1",
		"2026-05-21T00:00:01Z [default/api/app] first of day 2",
	})

	if err := MigrateLegacyLayout(dir, zap.NewNop()); err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}

	segments, err := ListSegments(dir, "default", "api", "app")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if want := []string{"2026-05-20", "2026-05-21"}; len(segments) != 2 || segments[0] != want[0] || segments[1] != want[1] {
		t.Fatalf("segments = %v, want %v", segments, want)
	}
}

func TestMigrateLegacyLayout_HandlesRestartSeparatorLines(t *testing.T) {
	dir := t.TempDir()
	writeLegacyPodFile(t, dir, "default", "api", []string{
		"2026-05-20T10:00:00Z [default/api/app] before restart",
		"--- pod restarted at 2026-05-20T10:00:01Z ---",
		"2026-05-20T10:00:02Z [default/api/app] after restart",
	})

	if err := MigrateLegacyLayout(dir, zap.NewNop()); err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}

	// The separator line has no parseable timestamp of its own, so it falls
	// back to migration-time "now" and lands in a different segment than the
	// two real, timestamped lines either side of it — count across all of the
	// container's segments rather than assuming a single file.
	segments, err := ListSegments(dir, "default", "api", "app")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	var totalLines int
	for _, segment := range segments {
		content, err := os.ReadFile(filepath.Join(dir, "default", "api", "app", segment+".log"))
		if err != nil {
			t.Fatalf("ReadFile segment %s: %v", segment, err)
		}
		for _, b := range content {
			if b == '\n' {
				totalLines++
			}
		}
	}
	if totalLines != 3 {
		t.Errorf("expected all 3 lines (including the separator) to migrate, got %d across segments %v", totalLines, segments)
	}
}

func TestMigrateLegacyLayout_NoOpWithoutLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "default", "api", "app"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "default", "api", "app", "2026-05-20.log"), []byte("already migrated\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := MigrateLegacyLayout(dir, zap.NewNop()); err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "default", "api", "app", "2026-05-20.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "already migrated\n" {
		t.Errorf("expected already-segmented content untouched, got %q", string(content))
	}
}

func TestMigrateLegacyLayout_MissingLogsRootIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := MigrateLegacyLayout(filepath.Join(dir, "does-not-exist"), zap.NewNop()); err != nil {
		t.Fatalf("MigrateLegacyLayout: %v", err)
	}
}
