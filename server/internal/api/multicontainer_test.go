package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
	"github.com/lsparey/simple-logging/internal/metrics"
	"github.com/lsparey/simple-logging/internal/storage"
)

// interleavedLines are two containers' lines of one pod whose timestamps
// alternate, so any per-container concatenation comes out of order.
var interleavedLines = []string{
	"2026-05-20T10:00:00Z [default/mypod/app] app 1",
	"2026-05-20T10:00:01Z [default/mypod/sidecar] sidecar 1",
	"2026-05-20T10:00:02Z [default/mypod/app] app 2",
	"2026-05-20T10:00:03Z [default/mypod/sidecar] sidecar 2",
	"2026-05-20T10:00:04Z [default/mypod/app] app 3",
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

func TestGetLogs_MultiContainerPodIsInTimestampOrder(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "mypod", interleavedLines)
	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))

	resp, err := client.GetLogs(context.Background(), connect.NewRequest(&pb.GetLogsRequest{Namespace: "default", Pod: "mypod"}))
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if strings.Join(resp.Msg.Lines, "\n") != strings.Join(interleavedLines, "\n") {
		t.Errorf("lines = %q, want %q", resp.Msg.Lines, interleavedLines)
	}
}

func TestGetLogs_MultiContainerPodPagesForwardAndBack(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "mypod", interleavedLines)
	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))
	get := func(req *pb.GetLogsRequest) *pb.GetLogsResponse {
		t.Helper()
		req.Namespace, req.Pod, req.PageSize = "default", "mypod", 2
		resp, err := client.GetLogs(context.Background(), connect.NewRequest(req))
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		return resp.Msg
	}

	last := get(&pb.GetLogsRequest{LoadLastPage: true})
	if strings.Join(last.Lines, "|") != strings.Join(interleavedLines[3:], "|") {
		t.Fatalf("last page = %q", last.Lines)
	}
	var older []string
	for token := last.PrevPageToken; token != ""; {
		page := get(&pb.GetLogsRequest{PageToken: token})
		older = append(page.Lines, older...)
		token = page.PrevPageToken
	}
	if got := append(older, last.Lines...); strings.Join(got, "|") != strings.Join(interleavedLines, "|") {
		t.Errorf("paging back collected %q, want every line in order", got)
	}
}

func TestDownload_MultiContainerPodIsInTimestampOrder(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "mypod", interleavedLines)
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})

	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=mypod", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != strings.Join(interleavedLines, "\n") {
		t.Errorf("download =\n%s\nwant every line in timestamp order", got)
	}
}

func TestStreamLogs_TailsEveryContainerIncludingNewOnes(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "mypod", interleavedLines)
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := startLineStream(ctx, newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamLogs,
		&pb.StreamLogsRequest{Namespace: "default", Pod: "mypod"})
	time.Sleep(100 * time.Millisecond)

	segment := func(container string) string {
		return filepath.Join(dir, "default", "mypod", container, "2026-05-20.log")
	}
	appendLine(t, segment("app"), "2026-05-20T10:00:05Z [default/mypod/app] live app")
	appendLine(t, segment("sidecar"), "2026-05-20T10:00:06Z [default/mypod/sidecar] live sidecar")
	// A container with no logs when the tail started, read from its start.
	appendLine(t, segment("late"), "2026-05-20T10:00:07Z [default/mypod/late] live late")

	want := map[string]bool{"live app": false, "live sidecar": false, "live late": false}
	deadline := time.After(3 * time.Second)
	for received := 0; received < len(want); {
		select {
		case line := <-stream.lines:
			for w := range want {
				if strings.HasSuffix(line, w) && !want[w] {
					want[w] = true
					received++
				}
			}
			if strings.Contains(line, "app 1") {
				t.Errorf("replayed history: %q", line)
			}
		case <-deadline:
			t.Fatalf("timed out; received %v", want)
		}
	}
	cancel()
	<-stream.err
}

func TestStreamLogs_HoldsBackAHalfWrittenLine(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "mypod", interleavedLines[:1])
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := startLineStream(ctx, newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamLogs,
		&pb.StreamLogsRequest{Namespace: "default", Pod: "mypod"})
	time.Sleep(100 * time.Millisecond)

	path := filepath.Join(dir, "default", "mypod", "app", "2026-05-20.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fmt.Fprint(f, "2026-05-20T10:00:05Z [default/mypod/app] first half")
	time.Sleep(400 * time.Millisecond) // more than one tail poll
	fmt.Fprintln(f, " and second half")

	select {
	case line := <-stream.lines:
		if !strings.HasSuffix(line, "first half and second half") {
			t.Errorf("got %q, want the whole line once complete", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
	cancel()
	<-stream.err
}

func TestGetWorkloadLogs_ReadsPastALongLine(t *testing.T) {
	dir := t.TempDir()
	long := "2026-05-20T10:00:01Z [default/mypod/app] " + strings.Repeat("x", 200*1024)
	writeLogFile(t, dir, "default", "mypod", []string{
		"2026-05-20T10:00:00Z [default/mypod/app] before",
		long,
		"2026-05-20T10:00:02Z [default/mypod/app] after",
	})
	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))

	resp, err := client.GetWorkloadLogs(context.Background(), connect.NewRequest(&pb.GetWorkloadLogsRequest{
		Namespace: "default", Kind: "Pod", Name: "mypod",
	}))
	if err != nil {
		t.Fatalf("GetWorkloadLogs: %v", err)
	}
	if len(resp.Msg.Lines) != 3 || resp.Msg.Lines[1] != long || !strings.HasSuffix(resp.Msg.Lines[2], "after") {
		t.Errorf("got %d lines, want before, the long line and after", len(resp.Msg.Lines))
	}
}

func TestSearchLogs_MatchesTheMessageNotThePrefix(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] using the default config",
		"2026-05-20T10:00:01Z [default/web-app-abc/app] ready",
	})
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})

	for query, want := range map[string]int{"default": 1, "web-app": 0, "2026-05-20": 0} {
		results, err := search(t, svc, &pb.SearchLogsRequest{Namespace: "default", Query: query})
		if err != nil {
			t.Fatalf("SearchLogs(%q): %v", query, err)
		}
		if got := len(results.lines()); got != want {
			t.Errorf("SearchLogs(%q) matched %d lines, want %d", query, got, want)
		}
	}
}

func TestServer_BasicAuthDoesNotExemptMetricsWhenDisabled(t *testing.T) {
	basic := loadTestHTPasswd(t)
	_, ts := newTestHTTPServer(t, NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{}), ServerOptions{Auth: basic})
	if status, _ := get(t, ts.URL+"/metrics", nil); status != http.StatusUnauthorized {
		t.Errorf("/metrics with metrics off: got %d, want 401 like any other page", status)
	}

	_, ts = newTestHTTPServer(t, NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{}), ServerOptions{Auth: basic, Metrics: metrics.New(nil).Handler()})
	if status, _ := get(t, ts.URL+"/metrics", nil); status != http.StatusOK {
		t.Errorf("/metrics with metrics on: got %d, want 200", status)
	}
}

// Ensure the storage package's line cap and the readers agree.
func TestStoredLinesAtTheCapAreReadable(t *testing.T) {
	dir := t.TempDir()
	w, err := storage.NewSegmentWriter(dir, "default", "mypod", "app")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	huge := ts.Format(time.RFC3339Nano) + " [default/mypod/app] " + strings.Repeat("y", 3*storage.MaxLineBytes)
	if !w.Write(ts, huge) || !w.Write(ts.Add(time.Second), ts.Add(time.Second).Format(time.RFC3339Nano)+" [default/mypod/app] next") {
		t.Fatal("write failed")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))
	resp, err := client.GetWorkloadLogs(context.Background(), connect.NewRequest(&pb.GetWorkloadLogsRequest{
		Namespace: "default", Kind: "Pod", Name: "mypod",
	}))
	if err != nil {
		t.Fatalf("GetWorkloadLogs: %v", err)
	}
	if len(resp.Msg.Lines) != 2 || len(resp.Msg.Lines[0]) > storage.MaxLineBytes || !strings.HasSuffix(resp.Msg.Lines[0], "[truncated]") {
		t.Errorf("want the capped line, marked truncated, then the next line; got %d lines", len(resp.Msg.Lines))
	}
}
