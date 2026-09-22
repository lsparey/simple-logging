package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// fakeChecker implements ActiveChecker for tests.
type fakeChecker struct {
	active map[string]bool // key: "namespace/pod"
}

func (f *fakeChecker) IsActive(namespace, pod string) bool {
	if f.active == nil {
		return false
	}
	return f.active[namespace+"/"+pod]
}

func (f *fakeChecker) IsJsonLogging(_, _ string) bool { return false }

// noopDeploymentMapper is a no-op DeploymentMapper for tests that don't
// exercise deployment functionality.
type noopDeploymentMapper struct{}

func (noopDeploymentMapper) GetDeploymentName(_, _ string) (string, bool) { return "", false }
func (noopDeploymentMapper) ListKnownDeployments(_ string) []string       { return nil }

// writeLogFile writes lines into the segmented layout
// <dir>/<namespace>/<pod>/<container>/<segment>.log, deriving each line's
// container from its "[ns/pod/container]" tag (defaulting to "app" for
// fixtures that don't carry one, e.g. content-only ListPods/ListLogFiles
// tests) and its segment from its leading RFC3339 timestamp (defaulting to a
// fixed sentinel day when absent). A 3-day fixture with real per-day
// timestamps therefore lands in three real segment files, exactly like
// production output.
func writeLogFile(t *testing.T, dir, namespace, pod string, lines []string) {
	t.Helper()
	for _, line := range lines {
		container := "app"
		if start, end := strings.IndexByte(line, '['), strings.IndexByte(line, ']'); start >= 0 && end > start {
			if parts := strings.Split(line[start+1:end], "/"); len(parts) == 3 && parts[2] != "" {
				container = parts[2]
			}
		}
		segment := "1970-01-01"
		if idx := strings.IndexByte(line, ' '); idx > 0 {
			if ts, err := time.Parse(time.RFC3339, line[:idx]); err == nil {
				segment = ts.UTC().Format("2006-01-02")
			}
		}
		containerDir := filepath.Join(dir, namespace, pod, container)
		if err := os.MkdirAll(containerDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		f, err := os.OpenFile(filepath.Join(containerDir, segment+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("OpenFile: %v", err)
		}
		if _, err := fmt.Fprintln(f, line); err != nil {
			f.Close()
			t.Fatalf("write line: %v", err)
		}
		f.Close()
	}
}

// ── ListNamespaces ────────────────────────────────────────────────────────────

func TestListNamespaces(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	os.MkdirAll(filepath.Join(dir, "kube-system"), 0755)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.ListNamespaces(context.Background(), &pb.ListNamespacesRequest{})
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}

	want := map[string]bool{"default": true, "kube-system": true}
	for _, ns := range resp.Namespaces {
		delete(want, ns)
	}
	if len(want) > 0 {
		t.Errorf("missing namespaces: %v", want)
	}
}

// ── ListPods ──────────────────────────────────────────────────────────────────

func TestListPods(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod-a", []string{"line"})
	writeLogFile(t, dir, "default", "pod-b", []string{"line"})

	checker := &fakeChecker{active: map[string]bool{"default/pod-a": true}}
	svc := NewLogService(dir, checker, checker, noopDeploymentMapper{})
	resp, err := svc.ListPods(context.Background(), &pb.ListPodsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}

	if len(resp.Pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(resp.Pods))
	}

	byName := make(map[string]*pb.PodInfo)
	for _, p := range resp.Pods {
		byName[p.Name] = p
	}
	if !byName["pod-a"].Active {
		t.Error("expected pod-a to be active")
	}
	if byName["pod-b"].Active {
		t.Error("expected pod-b to be inactive")
	}
	if containers := byName["pod-a"].Containers; len(containers) != 1 || containers[0] != "app" {
		t.Errorf("expected pod-a containers = [app], got %v", containers)
	}
}

func TestListPods_UnknownNamespace(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.ListPods(context.Background(), &pb.ListPodsRequest{Namespace: "nonexistent"})
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(resp.Pods) != 0 {
		t.Errorf("expected empty pod list for unknown namespace, got %d", len(resp.Pods))
	}
}

func TestListPods_ReportsAllContainers(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "multi-pod", []string{
		"2026-05-20T10:00:00Z [default/multi-pod/app] hello",
		"2026-05-20T10:00:01Z [default/multi-pod/sidecar] hello",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.ListPods(context.Background(), &pb.ListPodsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(resp.Pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(resp.Pods))
	}
	want := []string{"app", "sidecar"}
	got := resp.Pods[0].Containers
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("containers = %v, want %v", got, want)
	}
}

// ── ListLogFiles ─────────────────────────────────────────────────────────────

func TestListLogFiles(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod-a", []string{"alpha"})
	writeLogFile(t, dir, "monitoring", "pod-b", []string{"bravo", "charlie"})
	if err := os.WriteFile(filepath.Join(dir, "default", "index.json"), []byte("ignored"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	encodedKey := base64.RawURLEncoding.EncodeToString([]byte("companyUuid"))
	indexDir := filepath.Join(dir, ".indexes", "keys", encodedKey, "values", "ab")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	indexPath := filepath.Join(indexDir, "company-1.jsonl")
	indexEntry := `{"value":"company-1","line":"indexed"}` + "\n"
	if err := os.WriteFile(indexPath, []byte(indexEntry), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	secondIndexDir := filepath.Join(dir, ".indexes", "keys", encodedKey, "values", "cd")
	if err := os.MkdirAll(secondIndexDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secondIndexDir, "company-2.jsonl"), []byte(indexEntry), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	manifest := []byte(`{"keys":["companyUuid"],"formatVersion":3}`)
	if err := os.WriteFile(filepath.Join(dir, ".indexes", "indexes.json"), manifest, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.ListLogFiles(context.Background(), &pb.ListLogFilesRequest{})
	if err != nil {
		t.Fatalf("ListLogFiles: %v", err)
	}

	if len(resp.Files) != 4 {
		t.Fatalf("expected 4 summary rows, got %d", len(resp.Files))
	}

	// Log segments are grouped by (namespace, pod, container), so Subject —
	// not Name, which is now just a "N files" count — uniquely identifies a row.
	var summedSize int64
	byLogSubject := make(map[string]*pb.LogFileInfo)
	byIndexRow := make(map[string]*pb.LogFileInfo)
	for _, file := range resp.Files {
		summedSize += file.SizeBytes
		if file.Kind == "Log" {
			byLogSubject[file.Subject] = file
		} else {
			byIndexRow[file.Namespace+"/"+file.Name] = file
		}
	}
	if logFile := byLogSubject["default / pod-a (app)"]; logFile == nil {
		t.Error("missing default / pod-a (app) log summary")
	} else if logFile.Name != "1 file" {
		t.Errorf("log summary name = %q, want %q", logFile.Name, "1 file")
	}
	if byLogSubject["monitoring / pod-b (app)"] == nil {
		t.Error("missing monitoring / pod-b (app) log summary")
	}
	indexFile := byIndexRow[".indexes/2 files"]
	if indexFile == nil {
		t.Error("missing index summary")
	} else if indexFile.Kind != "Index" {
		t.Errorf("index summary kind = %q, want Index", indexFile.Kind)
	} else if indexFile.Subject != "companyUuid" {
		t.Errorf("index summary subject = %q, want %q", indexFile.Subject, "companyUuid")
	} else if indexFile.SizeBytes != int64(len(indexEntry)*2) {
		t.Errorf("index summary size = %d, want %d", indexFile.SizeBytes, len(indexEntry)*2)
	}
	metadataFile := byIndexRow[".indexes/indexes.json"]
	if metadataFile == nil {
		t.Error("missing indexes.json metadata file")
	} else if metadataFile.Subject != "Index metadata" {
		t.Errorf("metadata subject = %q, want %q", metadataFile.Subject, "Index metadata")
	}
	for _, file := range resp.Files {
		if file.ModifiedAtUnixMs <= 0 {
			t.Errorf("%s/%s has invalid modified time %d", file.Namespace, file.Name, file.ModifiedAtUnixMs)
		}
	}
	if resp.TotalSizeBytes != summedSize {
		t.Errorf("total size = %d, want %d", resp.TotalSizeBytes, summedSize)
	}
	if resp.TotalLogFileCount != 2 {
		t.Errorf("total log file count = %d, want 2", resp.TotalLogFileCount)
	}
	if resp.TotalIndexFileCount != 3 {
		t.Errorf("total index file count = %d, want 3", resp.TotalIndexFileCount)
	}
}

func TestListLogFiles_ReportsDiskUsage(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod-a", []string{"alpha"})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	svc.SetDiskWaterMarks(90, 80)

	resp, err := svc.ListLogFiles(context.Background(), &pb.ListLogFilesRequest{})
	if err != nil {
		t.Fatalf("ListLogFiles: %v", err)
	}
	if resp.DiskUsedPercent < 0 || resp.DiskUsedPercent > 100 {
		t.Errorf("DiskUsedPercent = %d, want a value in [0, 100]", resp.DiskUsedPercent)
	}
	if resp.DiskHighWaterPercent != 90 {
		t.Errorf("DiskHighWaterPercent = %d, want 90", resp.DiskHighWaterPercent)
	}
	if resp.DiskLowWaterPercent != 80 {
		t.Errorf("DiskLowWaterPercent = %d, want 80", resp.DiskLowWaterPercent)
	}
}

func TestListLogFiles_TruncatesToLargestFiles(t *testing.T) {
	dir := t.TempDir()
	const fileCount = maxListedLogFiles + 5
	for i := 0; i < fileCount; i++ {
		lines := make([]string, i+1)
		for j := range lines {
			lines[j] = "x"
		}
		writeLogFile(t, dir, "default", fmt.Sprintf("pod-%02d", i), lines)
	}

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.ListLogFiles(context.Background(), &pb.ListLogFilesRequest{})
	if err != nil {
		t.Fatalf("ListLogFiles: %v", err)
	}

	if len(resp.Files) != maxListedLogFiles {
		t.Fatalf("expected %d files, got %d", maxListedLogFiles, len(resp.Files))
	}
	if resp.TotalLogFileCount != int32(fileCount) {
		t.Errorf("total log file count = %d, want %d", resp.TotalLogFileCount, fileCount)
	}
	for i, file := range resp.Files {
		if i > 0 && file.SizeBytes > resp.Files[i-1].SizeBytes {
			t.Fatalf("files not sorted by size descending at index %d", i)
		}
	}
	// The smallest file (pod-00, 1 line) should have been dropped in favor of larger ones.
	for _, file := range resp.Files {
		if file.Subject == "default / pod-00 (app)" {
			t.Errorf("expected smallest pod pod-00 to be truncated from results")
		}
	}
}

func TestIndexFileGroupRecognizesV2Shards(t *testing.T) {
	encodedKey := base64.RawURLEncoding.EncodeToString([]byte("reqId"))
	got := indexFileGroup(filepath.Join("keys", encodedKey, "shards", "af.idx"))
	if got != "reqId" {
		t.Fatalf("indexFileGroup = %q, want reqId", got)
	}
}

// ── GetLogs ───────────────────────────────────────────────────────────────────

func TestGetLogs_Basic(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		"2026-05-20T10:00:00Z [default/pod/app] first",
		"2026-05-20T10:00:01Z [default/pod/app] second",
		"2026-05-20T10:00:02Z [default/pod/app] third",
	}
	writeLogFile(t, dir, "default", "pod", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default",
		Pod:       "pod",
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(resp.Lines))
	}
	if !strings.Contains(resp.Lines[0], "first") {
		t.Errorf("unexpected first line: %q", resp.Lines[0])
	}
}

func TestGetLogs_Pagination(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf("2026-05-20T10:00:%02dZ [default/pod/app] line %d", i, i))
	}
	writeLogFile(t, dir, "default", "pod", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	// First page of 2.
	resp1, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "pod", PageSize: 2,
	})
	if err != nil {
		t.Fatalf("GetLogs page1: %v", err)
	}
	if len(resp1.Lines) != 2 {
		t.Fatalf("expected 2 lines on page 1, got %d", len(resp1.Lines))
	}
	if resp1.NextPageToken == "" {
		t.Fatal("expected a next_page_token for page 1")
	}

	// Second page.
	resp2, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "pod", PageSize: 2, PageToken: resp1.NextPageToken,
	})
	if err != nil {
		t.Fatalf("GetLogs page2: %v", err)
	}
	if len(resp2.Lines) != 2 {
		t.Fatalf("expected 2 lines on page 2, got %d", len(resp2.Lines))
	}

	// Third (last) page.
	resp3, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "pod", PageSize: 2, PageToken: resp2.NextPageToken,
	})
	if err != nil {
		t.Fatalf("GetLogs page3: %v", err)
	}
	if len(resp3.Lines) != 1 {
		t.Fatalf("expected 1 line on last page, got %d", len(resp3.Lines))
	}
	if resp3.NextPageToken != "" {
		t.Error("expected no next_page_token on last page")
	}

	// Verify no duplicate or missing lines across all pages.
	allLines := append(append(resp1.Lines, resp2.Lines...), resp3.Lines...)
	if len(allLines) != 5 {
		t.Errorf("expected 5 total lines, got %d", len(allLines))
	}
}

func TestGetLogs_TimeRangeFilter(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		"2026-05-20T09:00:00Z [default/pod/app] before range",
		"2026-05-20T10:00:00Z [default/pod/app] in range",
		"2026-05-20T11:00:00Z [default/pod/app] after range",
	}
	writeLogFile(t, dir, "default", "pod", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	start := time.Date(2026, 5, 20, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)

	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default",
		Pod:       "pod",
		StartTime: start.Unix(),
		EndTime:   end.Unix(),
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Lines) != 1 {
		t.Fatalf("expected 1 line in time range, got %d: %v", len(resp.Lines), resp.Lines)
	}
	if !strings.Contains(resp.Lines[0], "in range") {
		t.Errorf("unexpected line: %q", resp.Lines[0])
	}
}

func TestGetLogs_InvalidPageToken(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod", []string{"line"})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	_, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "pod", PageToken: "notvalidbase64!!!",
	})
	if err == nil {
		t.Error("expected error for invalid page_token")
	}
}

func TestGetLogs_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	_, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "nonexistent",
	})
	if err == nil {
		t.Error("expected not-found error for unknown pod")
	}
}

func TestGetLogs_DefaultPageSize(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 250; i++ {
		lines = append(lines, fmt.Sprintf("2026-05-20T10:00:00Z [default/pod/app] line %d", i))
	}
	writeLogFile(t, dir, "default", "pod", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default", Pod: "pod",
		// PageSize intentionally zero — should default to 200.
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Lines) != defaultPageSize {
		t.Errorf("expected %d lines (default page size), got %d", defaultPageSize, len(resp.Lines))
	}
	if resp.NextPageToken == "" {
		t.Error("expected next_page_token when more lines remain")
	}
}

// ── StreamLogs ────────────────────────────────────────────────────────────────

// fakeStreamLogsServer captures sent lines and honours a cancellable context.
type fakeStreamLogsServer struct {
	ctx    context.Context
	lines  []string
	sendCh chan string
}

func newFakeStreamLogsServer(ctx context.Context) *fakeStreamLogsServer {
	return &fakeStreamLogsServer{ctx: ctx, sendCh: make(chan string, 64)}
}

func (f *fakeStreamLogsServer) Send(resp *pb.StreamLogsResponse) error {
	select {
	case <-f.ctx.Done():
		return f.ctx.Err()
	case f.sendCh <- resp.Line:
		return nil
	}
}
func (f *fakeStreamLogsServer) Context() context.Context     { return f.ctx }
func (f *fakeStreamLogsServer) SetHeader(metadata.MD) error  { return nil }
func (f *fakeStreamLogsServer) SendHeader(metadata.MD) error { return nil }
func (f *fakeStreamLogsServer) SetTrailer(metadata.MD)       {}
func (f *fakeStreamLogsServer) SendMsg(any) error            { return nil }
func (f *fakeStreamLogsServer) RecvMsg(any) error            { return nil }

func TestStreamLogs_OnlyNewLines(t *testing.T) {
	dir := t.TempDir()
	// Write existing content — StreamLogs must NOT replay these.
	writeLogFile(t, dir, "default", "mypod", []string{
		"2026-05-20T10:00:00Z [default/mypod/app] existing line",
	})

	logPath := filepath.Join(dir, "default", "mypod", "app", "2026-05-20.log")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newFakeStreamLogsServer(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamLogs(&pb.StreamLogsRequest{Namespace: "default", Pod: "mypod"}, stream)
	}()

	// Give the goroutine time to open and seek to EOF.
	time.Sleep(50 * time.Millisecond)

	// Append two new lines.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	fmt.Fprintln(f, "2026-05-20T10:00:01Z [default/mypod/app] new line 1")
	fmt.Fprintln(f, "2026-05-20T10:00:02Z [default/mypod/app] new line 2")
	f.Close()

	// Collect two lines then cancel.
	var received []string
	deadline := time.After(2 * time.Second)
	for len(received) < 2 {
		select {
		case line := <-stream.sendCh:
			received = append(received, line)
		case <-deadline:
			t.Fatalf("timed out waiting for streamed lines; got: %v", received)
		}
	}
	cancel()
	<-errCh

	if len(received) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(received), received)
	}
	if !strings.Contains(received[0], "new line 1") {
		t.Errorf("unexpected first line: %q", received[0])
	}
	if !strings.Contains(received[1], "new line 2") {
		t.Errorf("unexpected second line: %q", received[1])
	}
}

func TestStreamLogs_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	ctx := context.Background()
	stream := newFakeStreamLogsServer(ctx)
	err := svc.StreamLogs(&pb.StreamLogsRequest{Namespace: "default", Pod: "ghost"}, stream)
	if err == nil {
		t.Fatal("expected error for missing log file")
	}
}

func TestStreamLogs_MissingArgs(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	ctx := context.Background()
	stream := newFakeStreamLogsServer(ctx)
	err := svc.StreamLogs(&pb.StreamLogsRequest{}, stream)
	if err == nil {
		t.Fatal("expected error for empty namespace/pod")
	}
}

// TestGetLogs_SameTimestampPreservesOrder verifies that a burst of log lines
// sharing an identical RFC3339 timestamp (e.g. rapid startup messages) is
// returned in file-insertion order and not reordered.
func TestGetLogs_SameTimestampPreservesOrder(t *testing.T) {
	dir := t.TempDir()

	const ts = "2026-05-20T10:00:00Z"
	const n = 10
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s [default/pod/app] startup message %d", ts, i)
	}
	writeLogFile(t, dir, "default", "pod", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default",
		Pod:       "pod",
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}

	if len(resp.Lines) != n {
		t.Fatalf("expected %d lines, got %d", n, len(resp.Lines))
	}
	for i, got := range resp.Lines {
		if got != lines[i] {
			t.Errorf("line[%d]: got %q, want %q", i, got, lines[i])
		}
	}
}

// TestStreamLogs_SameTimestampPreservesOrder verifies that a burst of new log
// lines sharing an identical RFC3339 timestamp is streamed in file-write order.
func TestStreamLogs_SameTimestampPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "default", "pod", "app", "2026-05-20.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := os.Create(logPath); err != nil {
		t.Fatalf("Create: %v", err)
	}

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newFakeStreamLogsServer(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamLogs(&pb.StreamLogsRequest{Namespace: "default", Pod: "pod"}, stream)
	}()

	// Give the goroutine time to seek to EOF.
	time.Sleep(50 * time.Millisecond)

	const ts = "2026-05-20T10:00:00Z"
	const n = 5
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(f, "%s [default/pod/app] startup message %d\n", ts, i)
	}
	f.Close()

	var received []string
	deadline := time.After(2 * time.Second)
	for len(received) < n {
		select {
		case line := <-stream.sendCh:
			received = append(received, line)
		case <-deadline:
			t.Fatalf("timed out waiting for streamed lines; got %d/%d: %v", len(received), n, received)
		}
	}
	cancel()
	<-errCh

	for i, got := range received {
		want := fmt.Sprintf("%s [default/pod/app] startup message %d", ts, i)
		if got != want {
			t.Errorf("received[%d]: got %q, want %q", i, got, want)
		}
	}
}
