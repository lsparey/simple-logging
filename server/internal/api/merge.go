package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// Merge-sort and pagination helpers for reading log lines across several
// pods' segments, used by the Workload RPCs.

// logEntry is a single log line together with its parsed timestamp, used for
// merge-sorting across multiple pod log files.
type logEntry struct {
	ts   time.Time
	idx  int // global insertion order, used as a tiebreaker for equal timestamps
	line string
}

// logEntryHeap retains one bounded edge of the result set. For newest pages,
// the oldest entry is discarded when the page is full; for oldest pages, the
// newest entry is discarded. Memory therefore remains O(page size).
type logEntryHeap struct {
	entries []logEntry
	newest  bool
}

func (h logEntryHeap) Len() int { return len(h.entries) }
func (h logEntryHeap) Less(i, j int) bool {
	less := logEntryLess(h.entries[i], h.entries[j])
	if h.newest {
		return less
	}
	return !less
}
func (h logEntryHeap) Swap(i, j int)       { h.entries[i], h.entries[j] = h.entries[j], h.entries[i] }
func (h *logEntryHeap) Push(x interface{}) { h.entries = append(h.entries, x.(logEntry)) }
func (h *logEntryHeap) Pop() interface{} {
	old := h.entries
	n := len(old)
	x := old[n-1]
	h.entries = old[:n-1]
	return x
}

func logEntryLess(a, b logEntry) bool {
	if a.ts.Equal(b.ts) {
		return a.idx < b.idx
	}
	return a.ts.Before(b.ts)
}

// encodeForwardNanosToken encodes a "lines after this timestamp" cursor.
func encodeForwardNanosToken(nanos int64) string {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(nanos))
	return base64.StdEncoding.EncodeToString(b)
}

// encodeBackwardNanosToken encodes a "lines before this timestamp" cursor.
func encodeBackwardNanosToken(nanos int64) string {
	b := make([]byte, 9)
	b[0] = 0x01
	binary.BigEndian.PutUint64(b[1:], uint64(nanos))
	return base64.StdEncoding.EncodeToString(b)
}

type nanosToken struct {
	nanos    int64
	backward bool // true = "before", false = "after"
}

func decodeNanosToken(token string) (nanosToken, error) {
	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nanosToken{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	switch len(b) {
	case 8:
		return nanosToken{nanos: int64(binary.BigEndian.Uint64(b)), backward: false}, nil
	case 9:
		if b[0] != 0x01 {
			return nanosToken{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		return nanosToken{nanos: int64(binary.BigEndian.Uint64(b[1:])), backward: true}, nil
	default:
		return nanosToken{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
}

// parseLineTimestamp extracts the RFC3339 timestamp from the first
// space-delimited field of a log line. Returns the zero time on failure.
func parseLineTimestamp(line string) time.Time {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, line[:idx])
	if err != nil {
		return time.Time{}
	}
	return ts
}

// tailPodToChannel tails a pod's most recent log segment and sends new lines
// to ch until ctx is cancelled, switching segments across a UTC day rollover.
func tailPodToChannel(ctx context.Context, logsRoot, namespace, pod string, ch chan<- string) {
	_ = tailLatestSegment(ctx, logsRoot, namespace, pod, func(line string) error {
		select {
		case ch <- line:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
