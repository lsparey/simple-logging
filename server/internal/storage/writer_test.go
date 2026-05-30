package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewFileWriter_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFileWriter(dir, "mynamespace", "my-pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(filepath.Join(dir, "mynamespace")); err != nil {
		t.Errorf("expected namespace directory to be created: %v", err)
	}
}

func TestFileWriter_Write(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	defer w.Close()

	if err := w.Write("hello world"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Errorf("unexpected file content: %q", string(content))
	}
}

func TestFileWriter_Appends(t *testing.T) {
	dir := t.TempDir()

	w, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	w.Write("line1")
	w.Close()

	w2, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter (reopen): %v", err)
	}
	defer w2.Close()
	w2.Write("line2")

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "line1\nline2\n" {
		t.Errorf("unexpected file content: %q", string(content))
	}
}

func TestFileWriter_HasContent(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	defer w.Close()

	if w.HasContent() {
		t.Error("expected HasContent false for a new empty file")
	}
	w.Write("data")
	if !w.HasContent() {
		t.Error("expected HasContent true after writing data")
	}
}

func TestFileWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	defer w.Close()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := w.Write("concurrent"); err != nil {
				t.Errorf("Write: %v", err)
			}
		}()
	}
	wg.Wait()

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
}

// TestFileWriter_SequentialWritesPreserveOrder verifies that lines written one
// after another appear in the file in write order, even when all messages carry
// the same timestamp (as happens during rapid app startup).
func TestFileWriter_SequentialWritesPreserveOrder(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFileWriter(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	defer w.Close()

	const ts = "2026-05-20T10:00:00Z"
	const n = 10
	for i := 0; i < n; i++ {
		line := fmt.Sprintf("%s startup message %d", ts, i)
		if err := w.Write(line); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}

	content, err := os.ReadFile(filepath.Join(dir, "ns", "pod.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(got) != n {
		t.Fatalf("expected %d lines in file, got %d", n, len(got))
	}
	for i, line := range got {
		want := fmt.Sprintf("%s startup message %d", ts, i)
		if line != want {
			t.Errorf("line[%d]: got %q, want %q", i, line, want)
		}
	}
}
