package indexes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePodLog(t *testing.T, root, namespace, pod string, lines []string) {
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

func TestCreateBackfillsExistingLogs(t *testing.T) {
	root := t.TempDir()
	writePodLog(t, root, "default", "api", []string{
		`2026-06-05T08:00:00Z [default/api/app] {"companyUuid":"co-1","msg":"one"}`,
		`2026-06-05T08:00:01Z [default/api/app] {"companyUuid":"co-2","msg":"two"}`,
		`2026-06-05T08:00:02Z [default/api/app] {"companyUuid":"co-1","msg":"three"}`,
		`2026-06-05T08:00:03Z [default/api/app] plain text`,
	})

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	lines, next, prev, err := m.GetLogs("companyUuid", "co-1", 200, "", false)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if next != "" || prev != "" {
		t.Fatalf("unexpected pagination tokens next=%q prev=%q", next, prev)
	}
}

func TestObserveLineAppendsToExistingIndex(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root)
	if err := m.Create("userUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m.ObserveLine("default", "api", `2026-06-05T08:00:00Z [default/api/app] {"userUuid":"u-1","msg":"hello"}`)
	m.ObserveLine("default", "api", `2026-06-05T08:00:01Z [default/api/app] {"userUuid":"u-2","msg":"skip"}`)

	lines, _, _, err := m.GetLogs("userUuid", "u-1", 200, "", false)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
}

func TestGetLogsPaginates(t *testing.T) {
	root := t.TempDir()
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf(`2026-06-05T08:00:0%dZ [default/api/app] {"companyUuid":"co-1","msg":"%d"}`, i, i))
	}
	writePodLog(t, root, "default", "api", lines)

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	page1, next, prev, err := m.GetLogs("companyUuid", "co-1", 2, "", false)
	if err != nil {
		t.Fatalf("GetLogs page1: %v", err)
	}
	if len(page1) != 2 || next == "" || prev != "" {
		t.Fatalf("unexpected page1 len=%d next=%q prev=%q", len(page1), next, prev)
	}

	page2, next2, prev2, err := m.GetLogs("companyUuid", "co-1", 2, next, false)
	if err != nil {
		t.Fatalf("GetLogs page2: %v", err)
	}
	if len(page2) != 2 || next2 == "" || prev2 == "" {
		t.Fatalf("unexpected page2 len=%d next=%q prev=%q", len(page2), next2, prev2)
	}
}

func TestListValuesReturnsCountsDescending(t *testing.T) {
	root := t.TempDir()
	writePodLog(t, root, "default", "api", []string{
		`2026-06-05T08:00:00Z [default/api/app] {"companyUuid":"co-1","msg":"one"}`,
		`2026-06-05T08:00:01Z [default/api/app] {"companyUuid":"co-2","msg":"two"}`,
		`2026-06-05T08:00:02Z [default/api/app] {"companyUuid":"co-1","msg":"three"}`,
		`2026-06-05T08:00:03Z [default/api/app] {"companyUuid":"co-3","msg":"four"}`,
		`2026-06-05T08:00:04Z [default/api/app] {"companyUuid":"co-2","msg":"five"}`,
		`2026-06-05T08:00:05Z [default/api/app] {"companyUuid":"co-2","msg":"six"}`,
	})

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	values, err := m.ListValues("companyUuid")
	if err != nil {
		t.Fatalf("ListValues: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d: %#v", len(values), values)
	}
	want := []ValueInfo{
		{Value: "co-2", Count: 3},
		{Value: "co-1", Count: 2},
		{Value: "co-3", Count: 1},
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("value %d got %#v want %#v", i, values[i], want[i])
		}
	}
}

func TestCreateBackfillsLongJSONLines(t *testing.T) {
	root := t.TempDir()
	longMessage := strings.Repeat("x", 128*1024)
	line := fmt.Sprintf(
		`2026-06-05T08:00:00Z [default/api/app] {"message":"%s","companyUuid":"co-1"}`,
		longMessage,
	)
	writePodLog(t, root, "default", "api", []string{line})

	m := NewManager(root)
	if err := m.Create("message"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	lines, _, _, err := m.GetLogs("message", longMessage, 200, "", false)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 long line, got %d", len(lines))
	}
	if lines[0] != line {
		t.Fatalf("long line changed during indexing")
	}

	values, err := m.ListValues("message")
	if err != nil {
		t.Fatalf("ListValues: %v", err)
	}
	if len(values) != 1 || values[0].Value != longMessage || values[0].Count != 1 {
		t.Fatalf("unexpected long values: %#v", values)
	}
}
