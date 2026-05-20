package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newDebugMux(t *testing.T, dir string, active map[string]bool) *http.ServeMux {
	t.Helper()
	svc := NewLogService(dir, &fakeChecker{active: active})
	mux := http.NewServeMux()
	registerDebugRoutes(mux, svc)
	return mux
}

func TestDebug_ListNamespaces(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "default"), 0755)
	os.MkdirAll(filepath.Join(dir, "kube-system"), 0755)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/namespaces", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Namespaces []string `json:"namespaces"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	got := make(map[string]bool)
	for _, ns := range body.Namespaces {
		got[ns] = true
	}
	for _, want := range []string{"default", "kube-system"} {
		if !got[want] {
			t.Errorf("missing namespace %q", want)
		}
	}
}

func TestDebug_ListNamespaces_Empty(t *testing.T) {
	dir := t.TempDir()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/namespaces", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Must be [] not null.
	if body := rec.Body.String(); body != "{\"namespaces\":[]}\n" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestDebug_ListPods(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "pod-a", []string{"line"})
	writeLogFile(t, dir, "default", "pod-b", []string{"line"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pods?namespace=default", nil)
	newDebugMux(t, dir, map[string]bool{"default/pod-a": true}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Pods []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"pods"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(body.Pods))
	}
}

func TestDebug_ListPods_MissingNamespace(t *testing.T) {
	dir := t.TempDir()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pods", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDebug_GetLogs(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		"2026-05-20T10:00:00Z [default/pod/app] alpha",
		"2026-05-20T10:00:01Z [default/pod/app] beta",
		"2026-05-20T10:00:02Z [default/pod/app] gamma",
	}
	writeLogFile(t, dir, "default", "pod", lines)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?namespace=default&pod=pod", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Lines         []string `json:"lines"`
		NextPageToken string   `json:"next_page_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(body.Lines))
	}
}

func TestDebug_GetLogs_Pagination(t *testing.T) {
	dir := t.TempDir()
	var fileLines []string
	for i := 0; i < 5; i++ {
		fileLines = append(fileLines, fmt.Sprintf("2026-05-20T10:00:%02dZ [ns/pod/app] line %d", i, i))
	}
	writeLogFile(t, dir, "ns", "pod", fileLines)

	mux := newDebugMux(t, dir, nil)

	// Page 1.
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/debug/logs?namespace=ns&pod=pod&page_size=2", nil))

	var page1 struct {
		Lines         []string `json:"lines"`
		NextPageToken string   `json:"next_page_token"`
	}
	json.NewDecoder(rec1.Body).Decode(&page1)
	if len(page1.Lines) != 2 || page1.NextPageToken == "" {
		t.Fatalf("page1: got %d lines, token=%q", len(page1.Lines), page1.NextPageToken)
	}

	// Page 2 using the token.
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet,
		"/debug/logs?namespace=ns&pod=pod&page_size=2&page_token="+page1.NextPageToken, nil))

	var page2 struct {
		Lines         []string `json:"lines"`
		NextPageToken string   `json:"next_page_token"`
	}
	json.NewDecoder(rec2.Body).Decode(&page2)
	if len(page2.Lines) != 2 {
		t.Fatalf("page2: got %d lines", len(page2.Lines))
	}
}

func TestDebug_GetLogs_NotFound(t *testing.T) {
	dir := t.TempDir()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?namespace=ns&pod=nope", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDebug_GetLogs_InvalidPageSize(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "ns", "pod", []string{"line"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?namespace=ns&pod=pod&page_size=abc", nil)
	newDebugMux(t, dir, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDebug_MethodNotAllowed(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"/debug/namespaces", "/debug/pods", "/debug/logs"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		newDebugMux(t, dir, nil).ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s POST: expected 405, got %d", path, rec.Code)
		}
	}
}
