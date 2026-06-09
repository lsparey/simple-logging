package indexes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestGetLogsOrdersEntriesAcrossPodsByTimestamp(t *testing.T) {
	root := t.TempDir()
	writePodLog(t, root, "default", "api", []string{
		`2026-06-05T08:00:00Z [default/api/app] {"companyUuid":"co-1","msg":"api zero"}`,
		`2026-06-05T08:00:02Z [default/api/app] {"companyUuid":"co-1","msg":"api two"}`,
		`2026-06-05T08:00:04Z [default/api/app] {"companyUuid":"co-1","msg":"api four"}`,
	})
	writePodLog(t, root, "default", "worker", []string{
		`2026-06-05T08:00:01Z [default/worker/app] {"companyUuid":"co-1","msg":"worker one"}`,
		`2026-06-05T08:00:03Z [default/worker/app] {"companyUuid":"co-1","msg":"worker three"}`,
		`2026-06-05T08:00:05Z [default/worker/app] {"companyUuid":"co-1","msg":"worker five"}`,
	})

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	page1, next, prev, err := m.GetLogs("companyUuid", "co-1", 2, "", false)
	if err != nil {
		t.Fatalf("GetLogs page1: %v", err)
	}
	if prev != "" || next == "" {
		t.Fatalf("unexpected page1 tokens next=%q prev=%q", next, prev)
	}
	assertMessages(t, page1, "api zero", "worker one")

	page2, next, prev, err := m.GetLogs("companyUuid", "co-1", 2, next, false)
	if err != nil {
		t.Fatalf("GetLogs page2: %v", err)
	}
	if prev == "" || next == "" {
		t.Fatalf("unexpected page2 tokens next=%q prev=%q", next, prev)
	}
	assertMessages(t, page2, "api two", "worker three")

	lastPage, next, prev, err := m.GetLogs("companyUuid", "co-1", 2, "", true)
	if err != nil {
		t.Fatalf("GetLogs last page: %v", err)
	}
	if next != "" || prev == "" {
		t.Fatalf("unexpected last-page tokens next=%q prev=%q", next, prev)
	}
	assertMessages(t, lastPage, "api four", "worker five")
}

func assertMessages(t *testing.T, lines []string, messages ...string) {
	t.Helper()
	if len(lines) != len(messages) {
		t.Fatalf("got %d lines, want %d: %#v", len(lines), len(messages), lines)
	}
	for i, message := range messages {
		if !strings.Contains(lines[i], message) {
			t.Errorf("line %d = %q, want message %q", i, lines[i], message)
		}
	}
}

func TestListValuesReturnsNewestValuesFirst(t *testing.T) {
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

	values, next, prev, err := m.ListValues("companyUuid", 50, "")
	if err != nil {
		t.Fatalf("ListValues: %v", err)
	}
	if next != "" || prev != "" {
		t.Fatalf("unexpected pagination tokens next=%q prev=%q", next, prev)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d: %#v", len(values), values)
	}
	want := []ValueInfo{
		{Value: "co-2", Count: 3, LastUpdated: mustParseTime(t, "2026-06-05T08:00:05Z")},
		{Value: "co-3", Count: 1, LastUpdated: mustParseTime(t, "2026-06-05T08:00:03Z")},
		{Value: "co-1", Count: 2, LastUpdated: mustParseTime(t, "2026-06-05T08:00:02Z")},
	}
	for i := range want {
		if values[i].Value != want[i].Value ||
			values[i].Count != want[i].Count ||
			!values[i].LastUpdated.Equal(want[i].LastUpdated) {
			t.Fatalf("value %d got %#v want %#v", i, values[i], want[i])
		}
	}
}

func TestListValuesPaginatesNewestFirst(t *testing.T) {
	root := t.TempDir()
	lines := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf(
			`2026-06-05T08:00:0%dZ [default/api/app] {"companyUuid":"co-%d"}`,
			i,
			i,
		))
	}
	writePodLog(t, root, "default", "api", lines)

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	page1, next, prev, err := m.ListValues("companyUuid", 2, "")
	if err != nil {
		t.Fatalf("ListValues page1: %v", err)
	}
	if next == "" || prev != "" {
		t.Fatalf("unexpected page1 tokens next=%q prev=%q", next, prev)
	}
	assertValues(t, page1, "co-4", "co-3")

	page2, next2, prev2, err := m.ListValues("companyUuid", 2, next)
	if err != nil {
		t.Fatalf("ListValues page2: %v", err)
	}
	if next2 == "" || prev2 == "" {
		t.Fatalf("unexpected page2 tokens next=%q prev=%q", next2, prev2)
	}
	assertValues(t, page2, "co-2", "co-1")

	page1Again, _, _, err := m.ListValues("companyUuid", 2, prev2)
	if err != nil {
		t.Fatalf("ListValues previous page: %v", err)
	}
	assertValues(t, page1Again, "co-4", "co-3")
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func assertValues(t *testing.T, values []ValueInfo, expected ...string) {
	t.Helper()
	if len(values) != len(expected) {
		t.Fatalf("got %d values, want %d: %#v", len(values), len(expected), values)
	}
	for i, value := range expected {
		if values[i].Value != value {
			t.Errorf("value %d = %q, want %q", i, values[i].Value, value)
		}
	}
}

func TestDeleteRemovesManifestKeyAndIndexFiles(t *testing.T) {
	root := t.TempDir()
	writePodLog(t, root, "default", "api", []string{
		`2026-06-05T08:00:00Z [default/api/app] {"companyUuid":"co-1","msg":"one"}`,
	})

	m := NewManager(root)
	if err := m.Create("companyUuid"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if values, _, _, err := m.ListValues("companyUuid", 50, ""); err != nil || len(values) != 1 {
		t.Fatalf("ListValues before delete got values=%#v err=%v", values, err)
	}

	if err := m.Delete("companyUuid"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if keys := m.List(); len(keys) != 0 {
		t.Fatalf("expected no keys after delete, got %#v", keys)
	}
	if _, err := os.Stat(m.keyRoot("companyUuid")); !os.IsNotExist(err) {
		t.Fatalf("expected index dir removed, stat err=%v", err)
	}

	reloaded := NewManager(root)
	if keys := reloaded.List(); len(keys) != 0 {
		t.Fatalf("expected manifest without deleted key, got %#v", keys)
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

	values, _, _, err := m.ListValues("message", 50, "")
	if err != nil {
		t.Fatalf("ListValues: %v", err)
	}
	if len(values) != 1 || values[0].Value != longMessage || values[0].Count != 1 {
		t.Fatalf("unexpected long values: %#v", values)
	}
}
