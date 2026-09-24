package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckWritable_AllWritable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default", "pod", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "default", "pod", "app", "2026-09-24.log"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckWritable(root); err != nil {
		t.Errorf("CheckWritable: %v", err)
	}
}

func TestCheckWritable_ReportsUnwritableEntry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write regardless of permissions")
	}
	root := t.TempDir()
	segment := filepath.Join(root, "default", "pod", "app", "2026-09-24.log")
	if err := os.MkdirAll(filepath.Dir(segment), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(segment, []byte("x\n"), 0o444); err != nil {
		t.Fatal(err)
	}

	err := CheckWritable(root)
	if err == nil || !strings.Contains(err.Error(), segment) {
		t.Errorf("CheckWritable = %v, want an error naming %s", err, segment)
	}
}
