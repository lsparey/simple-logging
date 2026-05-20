package storage

import (
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
