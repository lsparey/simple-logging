// Package metrics holds simple-logging's self-metrics: counters and gauges
// for collection, retention and search, registered on a private Prometheus
// registry. The same values back the optional /metrics endpoint and the
// GetStats RPC, so users without Prometheus see them in the UI too.
//
// Every recording method is safe to call on a nil *Metrics, so components
// constructed without one (mostly tests) need no nil checks of their own.
package metrics

import (
	"math"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// Stream sources, used as the "source" label on streams_active.
const (
	SourceFile = "file"
	SourceAPI  = "api"
)

// Reasons a log segment is deleted, used as the "reason" label on
// retention_segments_deleted_total.
const (
	ReasonRetention = "retention"
	ReasonDiskGuard = "disk_guard"
)

// Metrics is the set of simple-logging self-metrics.
type Metrics struct {
	registry  *prometheus.Registry
	startedAt time.Time

	streamsActive            *prometheus.GaugeVec
	linesWritten             *prometheus.CounterVec
	bytesWritten             *prometheus.CounterVec
	linesDropped             prometheus.Counter
	apiReconnects            prometheus.Counter
	retentionSegmentsDeleted *prometheus.CounterVec
	searchesActive           prometheus.Gauge
	searchDuration           prometheus.Histogram
	searchBytesScanned       prometheus.Counter
}

// New creates the metrics and registers them, plus the standard Go runtime
// and process collectors, on a fresh registry. diskUsageRatio reports the
// fraction (0–1) of the log volume in use; it is called at scrape time.
func New(diskUsageRatio func() (float64, error)) *Metrics {
	m := &Metrics{
		registry:  prometheus.NewRegistry(),
		startedAt: time.Now(),

		streamsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplelog_streams_active",
			Help: "Container log streams currently being collected, by source (file or api).",
		}, []string{"source"}),
		linesWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simplelog_lines_written_total",
			Help: "Log lines written to storage, by namespace.",
		}, []string{"namespace"}),
		bytesWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simplelog_bytes_written_total",
			Help: "Bytes of log lines written to storage, by namespace.",
		}, []string{"namespace"}),
		linesDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simplelog_lines_dropped_total",
			Help: "Log lines dropped because a write to storage failed (e.g. a full or read-only volume).",
		}),
		apiReconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simplelog_api_reconnects_total",
			Help: "Times a Kubernetes log API stream was reopened after ending or failing.",
		}),
		retentionSegmentsDeleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simplelog_retention_segments_deleted_total",
			Help: "Log segments deleted, by reason (retention, or disk_guard when the volume crossed its high water mark).",
		}, []string{"reason"}),
		searchesActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "simplelog_searches_active",
			Help: "Server-side searches currently running.",
		}),
		searchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "simplelog_search_duration_seconds",
			Help:    "Wall-clock duration of server-side searches, including cancelled ones.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		}),
		searchBytesScanned: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simplelog_search_bytes_scanned_total",
			Help: "Bytes of log segments read by server-side searches.",
		}),
	}

	// Pre-create the fixed label values so they report 0 rather than being
	// absent until the first event.
	m.streamsActive.WithLabelValues(SourceFile)
	m.streamsActive.WithLabelValues(SourceAPI)
	m.retentionSegmentsDeleted.WithLabelValues(ReasonRetention)
	m.retentionSegmentsDeleted.WithLabelValues(ReasonDiskGuard)

	m.registry.MustRegister(
		m.streamsActive,
		m.linesWritten,
		m.bytesWritten,
		m.linesDropped,
		m.apiReconnects,
		m.retentionSegmentsDeleted,
		m.searchesActive,
		m.searchDuration,
		m.searchBytesScanned,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "simplelog_disk_usage_ratio",
			Help: "Fraction of the log storage volume in use (0–1).",
		}, func() float64 {
			if diskUsageRatio == nil {
				return math.NaN()
			}
			ratio, err := diskUsageRatio()
			if err != nil {
				return math.NaN()
			}
			return ratio
		}),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Handler serves the metrics in Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// StreamStarted records a container log stream starting from source.
func (m *Metrics) StreamStarted(source string) {
	if m == nil {
		return
	}
	m.streamsActive.WithLabelValues(source).Inc()
}

// StreamStopped records a container log stream from source ending.
func (m *Metrics) StreamStopped(source string) {
	if m == nil {
		return
	}
	m.streamsActive.WithLabelValues(source).Dec()
}

// LineWritten records one log line of size bytes written for namespace.
func (m *Metrics) LineWritten(namespace string, bytes int) {
	if m == nil {
		return
	}
	m.linesWritten.WithLabelValues(namespace).Inc()
	m.bytesWritten.WithLabelValues(namespace).Add(float64(bytes))
}

// LineDropped records one log line dropped because a write failed.
func (m *Metrics) LineDropped() {
	if m == nil {
		return
	}
	m.linesDropped.Inc()
}

// APIReconnect records a Kubernetes log API stream being reopened.
func (m *Metrics) APIReconnect() {
	if m == nil {
		return
	}
	m.apiReconnects.Inc()
}

// SegmentsDeleted records n log segments deleted for reason.
func (m *Metrics) SegmentsDeleted(reason string, n int) {
	if m == nil || n == 0 {
		return
	}
	m.retentionSegmentsDeleted.WithLabelValues(reason).Add(float64(n))
}

// SearchStarted records a server-side search starting, and returns a func to
// call when it ends (however it ends) that records its duration.
func (m *Metrics) SearchStarted() (done func()) {
	if m == nil {
		return func() {}
	}
	start := time.Now()
	m.searchesActive.Inc()
	return func() {
		m.searchesActive.Dec()
		m.searchDuration.Observe(time.Since(start).Seconds())
	}
}

// SearchScanned records bytes of log segments read by a search.
func (m *Metrics) SearchScanned(bytes int64) {
	if m == nil || bytes == 0 {
		return
	}
	m.searchBytesScanned.Add(float64(bytes))
}

// Stats is a point-in-time summary of the metrics, with per-label series
// summed, for callers that don't speak Prometheus (the GetStats RPC).
type Stats struct {
	StartedAt                time.Time
	StreamsActiveFile        int64
	StreamsActiveAPI         int64
	LinesWritten             int64
	BytesWritten             int64
	LinesDropped             int64
	APIReconnects            int64
	RetentionSegmentsDeleted int64
	DiskGuardSegmentsDeleted int64
	SearchesActive           int64
	SearchBytesScanned       int64
}

// Snapshot returns the current values of the metrics.
func (m *Metrics) Snapshot() Stats {
	if m == nil {
		return Stats{}
	}
	return Stats{
		StartedAt:                m.startedAt,
		StreamsActiveFile:        gaugeValue(m.streamsActive.WithLabelValues(SourceFile)),
		StreamsActiveAPI:         gaugeValue(m.streamsActive.WithLabelValues(SourceAPI)),
		LinesWritten:             sumCounterVec(m.linesWritten),
		BytesWritten:             sumCounterVec(m.bytesWritten),
		LinesDropped:             counterValue(m.linesDropped),
		APIReconnects:            counterValue(m.apiReconnects),
		RetentionSegmentsDeleted: counterValue(m.retentionSegmentsDeleted.WithLabelValues(ReasonRetention)),
		DiskGuardSegmentsDeleted: counterValue(m.retentionSegmentsDeleted.WithLabelValues(ReasonDiskGuard)),
		SearchesActive:           gaugeValue(m.searchesActive),
		SearchBytesScanned:       counterValue(m.searchBytesScanned),
	}
}

func counterValue(c prometheus.Counter) int64 {
	var out dto.Metric
	if err := c.Write(&out); err != nil {
		return 0
	}
	return int64(out.GetCounter().GetValue())
}

func gaugeValue(g prometheus.Gauge) int64 {
	var out dto.Metric
	if err := g.Write(&out); err != nil {
		return 0
	}
	return int64(out.GetGauge().GetValue())
}

// sumCounterVec sums every series of v across all its label values.
func sumCounterVec(v *prometheus.CounterVec) int64 {
	ch := make(chan prometheus.Metric)
	go func() {
		v.Collect(ch)
		close(ch)
	}()
	var total float64
	for metric := range ch {
		var out dto.Metric
		if err := metric.Write(&out); err == nil {
			total += out.GetCounter().GetValue()
		}
	}
	return int64(total)
}
