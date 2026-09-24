package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
	"github.com/lsparey/simple-logging/internal/auth"
	"github.com/lsparey/simple-logging/internal/metrics"
)

func TestGetStats_ReportsMetrics(t *testing.T) {
	m := metrics.New(nil)
	m.StreamStarted(metrics.SourceFile)
	m.LineWritten("default", 42)
	m.LineDropped()

	svc := NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{})
	svc.SetMetrics(m)
	resp, err := newTestClient(t, svc).GetStats(context.Background(), connect.NewRequest(&pb.GetStatsRequest{}))
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	got := resp.Msg
	if got.StreamsActiveFile != 1 || got.LinesWrittenTotal != 1 || got.BytesWrittenTotal != 42 || got.LinesDroppedTotal != 1 {
		t.Errorf("unexpected stats: %+v", got)
	}
	if got.StartedAtUnixMs == 0 {
		t.Error("StartedAtUnixMs not set")
	}
}

func TestGetStats_WithoutMetricsReportsZeros(t *testing.T) {
	svc := NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{})
	resp, err := newTestClient(t, svc).GetStats(context.Background(), connect.NewRequest(&pb.GetStatsRequest{}))
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if resp.Msg.LinesWrittenTotal != 0 || resp.Msg.StartedAtUnixMs != 0 {
		t.Errorf("expected zeros, got %+v", resp.Msg)
	}
}

func TestSearchLogs_RecordsMetrics(t *testing.T) {
	dir := t.TempDir()
	line := "2026-05-20T10:00:00Z [default/web-app-abc/app] something ERROR happened"
	writeLogFile(t, dir, "default", "web-app-abc", []string{line})

	m := metrics.New(nil)
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	svc.SetMetrics(m)
	if _, err := search(t, svc, &pb.SearchLogsRequest{Namespace: "default", Query: "error"}); err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}

	s := m.Snapshot()
	if s.SearchBytesScanned != int64(len(line))+1 {
		t.Errorf("SearchBytesScanned: got %d, want %d", s.SearchBytesScanned, len(line)+1)
	}
	if s.SearchesActive != 0 {
		t.Errorf("SearchesActive after the search ended: got %d, want 0", s.SearchesActive)
	}
}

func get(t *testing.T, url string, setAuth func(*http.Request)) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if setAuth != nil {
		setAuth(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestServer_MetricsRouteOnlyWhenEnabled(t *testing.T) {
	svc := NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{})

	_, off := newTestHTTPServer(t, svc, ServerOptions{})
	// Without Metrics, /metrics falls through to the SPA like any other
	// unknown path, exactly as before metrics existed.
	if _, body := get(t, off.URL+"/metrics", nil); strings.Contains(body, "simplelog_") {
		t.Error("/metrics served metrics while disabled")
	}

	_, on := newTestHTTPServer(t, svc, ServerOptions{Metrics: metrics.New(nil).Handler()})
	status, body := get(t, on.URL+"/metrics", nil)
	if status != http.StatusOK || !strings.Contains(body, "simplelog_lines_dropped_total") {
		t.Errorf("/metrics: got %d, body missing simplelog metrics", status)
	}
}

// loadTestHTPasswd returns basic auth accepting alice / secret.
func loadTestHTPasswd(t *testing.T) *auth.Basic {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "htpasswd")
	if err := os.WriteFile(path, []byte("alice:"+string(hash)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	basic, err := auth.LoadHTPasswd(path)
	if err != nil {
		t.Fatal(err)
	}
	return basic
}

func TestServer_BasicAuth(t *testing.T) {
	basic := loadTestHTPasswd(t)

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	_, ts := newTestHTTPServer(t, svc, ServerOptions{Auth: basic, Metrics: metrics.New(nil).Handler()})
	withCreds := func(r *http.Request) { r.SetBasicAuth("alice", "secret") }

	for _, path := range []string{"/", "/config.js", "/download?ns=default&pod=x&container=app"} {
		if status, _ := get(t, ts.URL+path, nil); status != http.StatusUnauthorized {
			t.Errorf("%s without credentials: got %d, want 401", path, status)
		}
	}
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		if status, _ := get(t, ts.URL+path, nil); status != http.StatusOK {
			t.Errorf("%s without credentials: got %d, want 200", path, status)
		}
	}
	if status, _ := get(t, ts.URL+"/config.js", withCreds); status != http.StatusOK {
		t.Errorf("/config.js with credentials: got %d, want 200", status)
	}

	// RPCs need credentials too.
	client := newAuthClient(ts.URL, nil)
	if _, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("RPC without credentials: got %v, want Unauthenticated", err)
	}
	client = newAuthClient(ts.URL, withCreds)
	if _, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{})); err != nil {
		t.Errorf("RPC with credentials: %v", err)
	}
}

// newAuthClient returns a Connect client that passes each request through
// setAuth (nil sends no credentials).
func newAuthClient(baseURL string, setAuth func(*http.Request)) simplelogv1connect.LogServiceClient {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if setAuth != nil {
			setAuth(r)
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	return simplelogv1connect.NewLogServiceClient(httpClient, baseURL)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
