package api

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

func TestSearchLogs_RequiresQuery(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	_, err := search(t, svc, &pb.SearchLogsRequest{})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestSearchLogs_RejectsInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	_, err := search(t, svc, &pb.SearchLogsRequest{Query: "([", Regex: true})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestSearchLogs_SubstringMatchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] something ERROR happened",
		"2026-05-20T10:00:01Z [default/web-app-abc/app] all good",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	stream, err := search(t, svc, &pb.SearchLogsRequest{Namespace: "default", Query: "error"})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}

	lines := stream.lines()
	if len(lines) != 1 || lines[0] != "2026-05-20T10:00:00Z [default/web-app-abc/app] something ERROR happened" {
		t.Errorf("lines = %v, want the single ERROR line", lines)
	}
	if stream.truncated() {
		t.Error("did not expect truncation")
	}
}

func TestSearchLogs_RegexMatch(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] request id=42 status=200",
		"2026-05-20T10:00:01Z [default/web-app-abc/app] request id=43 status=500",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	req := &pb.SearchLogsRequest{Namespace: "default", Query: `status=(4\d\d|5\d\d)`, Regex: true}
	stream, err := search(t, svc, req)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}

	lines := stream.lines()
	if len(lines) != 1 {
		t.Fatalf("lines = %v, want 1 match", lines)
	}
}

func TestSearchLogs_NamespaceScoping(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] needle in default",
	})
	writeLogFile(t, dir, "other", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [other/web-app-abc/app] needle in other",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})

	// Scoped to one namespace.
	stream, err := search(t, svc, &pb.SearchLogsRequest{Namespace: "default", Query: "needle"})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if lines := stream.lines(); len(lines) != 1 || lines[0] != "2026-05-20T10:00:00Z [default/web-app-abc/app] needle in default" {
		t.Errorf("scoped search lines = %v", lines)
	}

	// Unscoped: searches every namespace.
	streamAll, err := search(t, svc, &pb.SearchLogsRequest{Query: "needle"})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if lines := streamAll.lines(); len(lines) != 2 {
		t.Errorf("unscoped search lines = %v, want 2", lines)
	}
}

func TestSearchLogs_WorkloadScoping(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{
		"2026-05-20T10:00:00Z [default/cache-0/app] needle in cache",
	})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] needle in web-app",
	})
	recordOwner(t, dir, "default", "web-app-abc", "Deployment", "web-app", "")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	req := &pb.SearchLogsRequest{Namespace: "default", WorkloadKind: "StatefulSet", WorkloadName: "cache", Query: "needle"}
	stream, err := search(t, svc, req)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if lines := stream.lines(); len(lines) != 1 || lines[0] != "2026-05-20T10:00:00Z [default/cache-0/app] needle in cache" {
		t.Errorf("workload-scoped search lines = %v", lines)
	}
}

func TestSearchLogs_PodAndContainerScoping(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] needle in app",
	})
	writeLogFile(t, dir, "default", "other-pod-abc", []string{
		"2026-05-20T10:00:00Z [default/other-pod-abc/app] needle in other pod",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	req := &pb.SearchLogsRequest{Namespace: "default", Pod: "web-app-abc", Query: "needle"}
	stream, err := search(t, svc, req)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if lines := stream.lines(); len(lines) != 1 || lines[0] != "2026-05-20T10:00:00Z [default/web-app-abc/app] needle in app" {
		t.Errorf("pod-scoped search lines = %v", lines)
	}
}

func TestSearchLogs_TimeRangePrunesSegments(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-18T10:00:00Z [default/web-app-abc/app] needle on day 1",
		"2026-05-19T10:00:00Z [default/web-app-abc/app] needle on day 2",
		"2026-05-20T10:00:00Z [default/web-app-abc/app] needle on day 3",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	start := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 19, 23, 59, 59, 0, time.UTC)
	req := &pb.SearchLogsRequest{
		Namespace:       "default",
		Query:           "needle",
		StartTimeUnixMs: start.UnixMilli(),
		EndTimeUnixMs:   end.UnixMilli(),
	}
	stream, err := search(t, svc, req)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if lines := stream.lines(); len(lines) != 1 || lines[0] != "2026-05-19T10:00:00Z [default/web-app-abc/app] needle on day 2" {
		t.Errorf("time-scoped search lines = %v", lines)
	}
}

func TestSearchLogs_MaxResultsTruncates(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 10)
	for i := range 10 {
		lines = append(lines, "2026-05-20T10:00:0"+string(rune('0'+i))+"Z [default/web-app-abc/app] needle")
	}
	writeLogFile(t, dir, "default", "web-app-abc", lines)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	req := &pb.SearchLogsRequest{Namespace: "default", Query: "needle", MaxResults: 3}
	stream, err := search(t, svc, req)
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if got := len(stream.lines()); got != 3 {
		t.Errorf("got %d lines, want 3", got)
	}
	if !stream.truncated() {
		t.Error("expected the final message to be marked truncated")
	}
}

func TestSearchLogs_NoMatchesNotTruncated(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] nothing interesting",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	stream, err := search(t, svc, &pb.SearchLogsRequest{Namespace: "default", Query: "needle"})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if len(stream.sent) != 0 {
		t.Errorf("expected no sent responses, got %d", len(stream.sent))
	}
}
