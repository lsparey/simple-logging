package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
)

func TestServerIntegration_ListNamespaces(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	os.MkdirAll(filepath.Join(dir, "monitoring"), 0755)

	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))

	resp, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{}))
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}

	got := make(map[string]bool)
	for _, ns := range resp.Msg.Namespaces {
		got[ns] = true
	}
	for _, want := range []string{"default", "monitoring"} {
		if !got[want] {
			t.Errorf("missing namespace %q in response", want)
		}
	}
}

func TestServerIntegration_GetLogs_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		"2026-05-20T10:00:00Z [default/pod/app] alpha",
		"2026-05-20T10:00:01Z [default/pod/app] beta",
	}
	writeLogFile(t, dir, "default", "pod", lines)

	client := newTestClient(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}))

	resp, err := client.GetLogs(context.Background(), connect.NewRequest(&pb.GetLogsRequest{
		Namespace: "default",
		Pod:       "pod",
	}))
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Msg.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(resp.Msg.Lines))
	}
}

// TestServer_ServesEveryProtocol checks the one port answers Connect (what
// the browser uses), gRPC-Web, and native gRPC over plaintext HTTP/2.
func TestServer_ServesEveryProtocol(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	_, ts := newTestHTTPServer(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}), ServerOptions{})

	h2c := &http.Client{Transport: &http.Transport{
		Protocols: func() *http.Protocols {
			var p http.Protocols
			p.SetUnencryptedHTTP2(true)
			return &p
		}(),
	}}

	clients := map[string]simplelogv1connect.LogServiceClient{
		"connect":  simplelogv1connect.NewLogServiceClient(ts.Client(), ts.URL),
		"grpc-web": simplelogv1connect.NewLogServiceClient(ts.Client(), ts.URL, connect.WithGRPCWeb()),
		"grpc":     simplelogv1connect.NewLogServiceClient(h2c, ts.URL, connect.WithGRPC()),
	}
	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			resp, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{}))
			if err != nil {
				t.Fatalf("ListNamespaces: %v", err)
			}
			if len(resp.Msg.Namespaces) != 1 || resp.Msg.Namespaces[0] != "default" {
				t.Errorf("namespaces = %v, want [default]", resp.Msg.Namespaces)
			}
		})
	}
}

func TestServer_ProbesAndAPIBeforeAndAfterSetService(t *testing.T) {
	srv, ts := newTestHTTPServer(t, nil, ServerOptions{})

	get := func(path string) int {
		t.Helper()
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := get("/healthz"); code != http.StatusOK {
		t.Errorf("/healthz before SetService: got %d, want 200", code)
	}
	if code := get("/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("/readyz before SetService: got %d, want 503", code)
	}
	if code := get("/download?ns=default&pod=p"); code != http.StatusServiceUnavailable {
		t.Errorf("/download before SetService: got %d, want 503", code)
	}
	client := simplelogv1connect.NewLogServiceClient(ts.Client(), ts.URL)
	_, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{}))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("RPC before SetService: got %v, want Unavailable", err)
	}

	srv.SetService(NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{}))

	if code := get("/readyz"); code != http.StatusOK {
		t.Errorf("/readyz after SetService: got %d, want 200", code)
	}
	if _, err := client.ListNamespaces(context.Background(), connect.NewRequest(&pb.ListNamespacesRequest{})); err != nil {
		t.Errorf("RPC after SetService: %v", err)
	}
}

func TestServer_ServesSPA(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":        {Data: []byte("<html>app</html>")},
		"assets/app-abc.js": {Data: []byte("console.log('app')")},
		"favicon.svg":       {Data: []byte("<svg/>")},
	}
	_, ts := newTestHTTPServer(t, nil, ServerOptions{UI: ui})

	cases := []struct {
		path, wantBody, wantCache string
	}{
		{"/", "<html>app</html>", "no-cache"},
		{"/index.html", "<html>app</html>", "no-cache"},
		// Client-side routes fall back to index.html.
		{"/ns/default/Deployment/web-app", "<html>app</html>", "no-cache"},
		{"/assets/app-abc.js", "console.log('app')", "public, max-age=31536000, immutable"},
		{"/favicon.svg", "<svg/>", ""},
	}
	for _, tc := range cases {
		resp, err := ts.Client().Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != tc.wantBody {
			t.Errorf("GET %s: got %d %q, want 200 %q", tc.path, resp.StatusCode, body, tc.wantBody)
		}
		if got := resp.Header.Get("Cache-Control"); got != tc.wantCache {
			t.Errorf("GET %s: Cache-Control %q, want %q", tc.path, got, tc.wantCache)
		}
	}
}

func TestServer_SPANotBuilt(t *testing.T) {
	_, ts := newTestHTTPServer(t, nil, ServerOptions{UI: fstest.MapFS{".gitkeep": {}}})

	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "frontend not built") {
		t.Errorf("got %d %q, want 404 explaining the frontend isn't built", resp.StatusCode, body)
	}
}

func TestServer_ConfigJS(t *testing.T) {
	_, ts := newTestHTTPServer(t, nil, ServerOptions{APIURL: "https://api.example.com"})

	resp, err := ts.Client().Get(ts.URL + "/config.js")
	if err != nil {
		t.Fatalf("GET /config.js: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if want := `window.__CONFIG__ = {"apiUrl":"https://api.example.com"};` + "\n"; string(body) != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func preflight(t *testing.T, ts *httptest.Server, origin string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/simplelog.v1.LogService/ListNamespaces", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	// Browsers send these lowercased and sorted, which rs/cors relies on.
	req.Header.Set("Access-Control-Request-Headers", "connect-protocol-version,content-type")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	resp.Body.Close()
	return resp
}

func TestServer_CORSOffByDefault(t *testing.T) {
	_, ts := newTestHTTPServer(t, NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{}), ServerOptions{})

	resp := preflight(t, ts, "http://logs.dev.internal")
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want none", got)
	}
}

func TestServer_CORSAllowedOrigins(t *testing.T) {
	svc := NewLogService(t.TempDir(), &fakeChecker{}, &fakeChecker{})
	_, ts := newTestHTTPServer(t, svc, ServerOptions{CORSAllowedOrigins: []string{"http://logs.dev.internal"}})

	resp := preflight(t, ts, "http://logs.dev.internal")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("preflight status: got %d, want 2xx", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://logs.dev.internal" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want the allowed origin", got)
	}

	resp = preflight(t, ts, "http://evil.example.com")
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin for other origin: got %q, want none", got)
	}
}

// TestServer_ShutdownEndsStreams checks a long-lived streaming RPC doesn't
// hold shutdown open until its deadline.
func TestServer_ShutdownEndsStreams(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod", []string{"2026-05-20T10:00:00Z [default/pod/app] existing"})
	srv, ts := newTestHTTPServer(t, NewLogService(dir, &fakeChecker{}, &fakeChecker{}), ServerOptions{})

	stream := startLineStream(context.Background(), simplelogv1connect.NewLogServiceClient(ts.Client(), ts.URL), simplelogv1connect.LogServiceClient.StreamLogs,
		&pb.StreamLogsRequest{Namespace: "default", Pod: "pod"})
	time.Sleep(50 * time.Millisecond) // let the handler start tailing

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	srv.Shutdown(shutdownCtx)
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Shutdown took %v; streams should end promptly", took)
	}

	select {
	case <-stream.err:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after Shutdown")
	}
}

// Ensure NewServer compiles and wires correctly (smoke test).
func TestNewServer_Smoke(t *testing.T) {
	if srv := NewServer(ServerOptions{UI: fstest.MapFS{}}, zap.NewNop()); srv == nil {
		t.Fatal("expected non-nil Server")
	}
}
