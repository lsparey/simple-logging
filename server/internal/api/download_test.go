package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDownload_RequiresNamespace(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?pod=web-app-abc", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDownload_RequiresExactlyOneOfPodOrWorkload(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("neither pod nor kind+name: status = %d, want 400", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	downloadHandler(svc)(rec2, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=web-app-abc&kind=Deployment&name=web-app", nil))
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("both pod and kind+name: status = %d, want 400", rec2.Code)
	}
}

func TestDownload_SinglePod(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] first",
		"2026-05-20T10:00:01Z [default/web-app-abc/app] second",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=web-app-abc", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "default_web-app-abc") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Errorf("body = %q, missing expected lines", body)
	}
}

func TestDownload_WorkloadMergesPodsChronologically(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{
		"2026-05-20T10:00:00Z [default/cache-0/app] line A",
		"2026-05-20T10:00:02Z [default/cache-0/app] line C",
	})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")
	writeLogFile(t, dir, "default", "cache-1", []string{
		"2026-05-20T10:00:01Z [default/cache-1/app] line B",
	})
	recordOwner(t, dir, "default", "cache-1", "StatefulSet", "cache", "")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&kind=StatefulSet&name=cache", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	lines := strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %v, want 3", lines)
	}
	if !strings.Contains(lines[0], "line A") || !strings.Contains(lines[1], "line B") || !strings.Contains(lines[2], "line C") {
		t.Errorf("lines not merged chronologically: %v", lines)
	}
}

func TestDownload_ContainerFilter(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] app line",
		"2026-05-20T10:00:01Z [default/web-app-abc/sidecar] sidecar line",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=web-app-abc&container=sidecar", nil))

	body := rec.Body.String()
	if strings.Contains(body, "app line") || !strings.Contains(body, "sidecar line") {
		t.Errorf("body = %q, container filter not applied", body)
	}
}

func TestDownload_TimeRangeFilter(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-18T10:00:00Z [default/web-app-abc/app] day 1",
		"2026-05-19T10:00:00Z [default/web-app-abc/app] day 2",
	})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	from := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC).Unix()
	to := time.Date(2026, 5, 18, 18, 0, 0, 0, time.UTC).Unix()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/download?ns=default&pod=web-app-abc&from=%d&to=%d", from, to), nil)
	downloadHandler(svc)(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "day 1") || strings.Contains(body, "day 2") {
		t.Errorf("body = %q, time range filter not applied as expected", body)
	}
}

func TestDownload_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=ghost", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDownload_InvalidTimeParam(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	rec := httptest.NewRecorder()
	downloadHandler(svc)(rec, httptest.NewRequest(http.MethodGet, "/download?ns=default&pod=web-app-abc&from=not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
