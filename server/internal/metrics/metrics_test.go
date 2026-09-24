package metrics

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSnapshot(t *testing.T) {
	m := New(func() (float64, error) { return 0.5, nil })

	m.StreamStarted(SourceFile)
	m.StreamStarted(SourceAPI)
	m.StreamStarted(SourceAPI)
	m.StreamStopped(SourceAPI)
	m.LineWritten("default", 10)
	m.LineWritten("kube-system", 5)
	m.LineDropped()
	m.APIReconnect()
	m.SegmentsDeleted(ReasonRetention, 3)
	m.SegmentsDeleted(ReasonDiskGuard, 2)
	done := m.SearchStarted()
	m.SearchScanned(100)

	s := m.Snapshot()
	if s.StreamsActiveFile != 1 || s.StreamsActiveAPI != 1 {
		t.Errorf("streams: got file=%d api=%d, want 1 and 1", s.StreamsActiveFile, s.StreamsActiveAPI)
	}
	if s.LinesWritten != 2 || s.BytesWritten != 15 {
		t.Errorf("written: got lines=%d bytes=%d, want 2 and 15", s.LinesWritten, s.BytesWritten)
	}
	if s.LinesDropped != 1 || s.APIReconnects != 1 {
		t.Errorf("dropped=%d reconnects=%d, want 1 and 1", s.LinesDropped, s.APIReconnects)
	}
	if s.RetentionSegmentsDeleted != 3 || s.DiskGuardSegmentsDeleted != 2 {
		t.Errorf("deleted: retention=%d disk_guard=%d, want 3 and 2", s.RetentionSegmentsDeleted, s.DiskGuardSegmentsDeleted)
	}
	if s.SearchesActive != 1 || s.SearchBytesScanned != 100 {
		t.Errorf("search: active=%d scanned=%d, want 1 and 100", s.SearchesActive, s.SearchBytesScanned)
	}

	done()
	if got := m.Snapshot().SearchesActive; got != 0 {
		t.Errorf("SearchesActive after done: got %d, want 0", got)
	}
}

func TestNilMetricsIsANoOp(t *testing.T) {
	var m *Metrics
	m.StreamStarted(SourceFile)
	m.LineWritten("default", 1)
	m.LineDropped()
	m.APIReconnect()
	m.SegmentsDeleted(ReasonRetention, 1)
	m.SearchStarted()()
	m.SearchScanned(1)
	if (m.Snapshot() != Stats{}) {
		t.Error("nil Snapshot should be zero")
	}
}

func TestHandlerExposesMetrics(t *testing.T) {
	m := New(func() (float64, error) { return 0, errors.New("statfs failed") })
	m.LineWritten("default", 10)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{
		`simplelog_lines_written_total{namespace="default"} 1`,
		`simplelog_bytes_written_total{namespace="default"} 10`,
		`simplelog_streams_active{source="api"} 0`,
		`simplelog_retention_segments_deleted_total{reason="disk_guard"} 0`,
		`simplelog_disk_usage_ratio NaN`,
		`simplelog_search_duration_seconds_bucket`,
		`go_goroutines`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in /metrics output", want)
		}
	}
}
