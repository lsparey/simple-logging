package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRetentionManager_DeletesOldFiles(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	os.MkdirAll(nsDir, 0755)

	// Write a file then backdate its mtime to 31 days ago.
	oldPath := filepath.Join(nsDir, "old-pod.log")
	os.WriteFile(oldPath, []byte("old log data"), 0644)
	past := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(oldPath, past, past)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("expected old log file to be deleted after sweep")
	}
}

func TestRetentionManager_KeepsNewFiles(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	os.MkdirAll(nsDir, 0755)

	newPath := filepath.Join(nsDir, "new-pod.log")
	os.WriteFile(newPath, []byte("recent log data"), 0644)
	// mtime is now — well within 30 days.

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected recent log file to be kept after sweep: %v", err)
	}
}

func TestRetentionManager_RemovesEmptyNamespaceDirs(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	os.MkdirAll(nsDir, 0755)

	// Only one file in the namespace — backdate it.
	logPath := filepath.Join(nsDir, "pod.log")
	os.WriteFile(logPath, []byte("data"), 0644)
	past := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(logPath, past, past)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(nsDir); !os.IsNotExist(err) {
		t.Error("expected empty namespace directory to be removed after sweep")
	}
}

func TestRetentionManager_KeepsNonEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	nsDir := filepath.Join(dir, "ns")
	os.MkdirAll(nsDir, 0755)

	// Old file (will be deleted) and a new file (will be kept).
	oldPath := filepath.Join(nsDir, "old.log")
	os.WriteFile(oldPath, []byte("old"), 0644)
	past := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(oldPath, past, past)

	newPath := filepath.Join(nsDir, "new.log")
	os.WriteFile(newPath, []byte("new"), 0644)

	rm := NewRetentionManager(dir, 30, time.Hour, zap.NewNop())
	rm.sweep()

	if _, err := os.Stat(nsDir); err != nil {
		t.Errorf("expected namespace dir to remain when non-empty: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected new log file to remain: %v", err)
	}
}
