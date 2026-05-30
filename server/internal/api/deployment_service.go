package api

import (
	"bufio"
	"container/heap"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// deploymentPodsForNamespace returns all pod names on disk that belong to the
// given deployment. It uses the DeploymentMapper for pods the collector has
// observed, and falls back to a heuristic for historical pods the current
// process has not seen (e.g. from a previous run).
func (s *LogService) deploymentPodsForNamespace(namespace, deployment string) ([]string, error) {
	nsDir := filepath.Join(s.logsRoot, namespace)
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, status.Errorf(codes.Internal, "read namespace dir: %v", err)
	}

	seen := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		podName := strings.TrimSuffix(e.Name(), ".log")

		// Try the mapper first (exact, from live pod observation).
		if dep, ok := s.deployments.GetDeploymentName(namespace, podName); ok {
			if dep == deployment {
				seen[podName] = struct{}{}
			}
			continue
		}

		// Heuristic fallback for historical pods not seen by the current process:
		// pod name = <deployment>-<rsHash>-<podHash>
		// Check whether the pod name starts with "<deployment>-" and has two more
		// dash-separated segments after that (each 5+ lowercase-alphanumeric chars).
		if isDeploymentPod(podName, deployment) {
			seen[podName] = struct{}{}
		}
	}

	pods := make([]string, 0, len(seen))
	for p := range seen {
		pods = append(pods, p)
	}
	sort.Strings(pods)
	return pods, nil
}

// isDeploymentPod returns true when podName looks like it belongs to deployment
// using the standard Kubernetes naming convention:
//
//	<deployment>-<rsHash>-<podHash>
//
// where rsHash and podHash are lowercase alphanumeric strings.
func isDeploymentPod(podName, deployment string) bool {
	prefix := deployment + "-"
	if !strings.HasPrefix(podName, prefix) {
		return false
	}
	rest := podName[len(prefix):]
	// rest should be "<rsHash>-<podHash>" — two alphanumeric segments separated by a dash.
	dashIdx := strings.Index(rest, "-")
	if dashIdx < 1 {
		return false
	}
	rsHash := rest[:dashIdx]
	podHash := rest[dashIdx+1:]
	return isAlphanumLower(rsHash) && isAlphanumLower(podHash) && len(podHash) >= 4
}

func isAlphanumLower(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ListDeployments returns all deployments with log files in the given namespace.
func (s *LogService) ListDeployments(_ context.Context, req *pb.ListDeploymentsRequest) (*pb.ListDeploymentsResponse, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	nsDir := filepath.Join(s.logsRoot, req.Namespace)
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &pb.ListDeploymentsResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "read namespace dir: %v", err)
	}

	// Collect deployment names by inspecting each pod log file.
	deploymentActive := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		podName := strings.TrimSuffix(e.Name(), ".log")

		var depName string
		if d, ok := s.deployments.GetDeploymentName(req.Namespace, podName); ok {
			depName = d
		} else {
			// Heuristic: strip the two trailing hash segments.
			depName = inferDeploymentName(podName)
		}
		if depName == "" {
			continue
		}

		active := s.active.IsActive(req.Namespace, podName)
		if cur, exists := deploymentActive[depName]; exists {
			deploymentActive[depName] = cur || active
		} else {
			deploymentActive[depName] = active
		}
	}

	// Also include deployments tracked by the collector but not yet flushed to disk.
	for _, d := range s.deployments.ListKnownDeployments(req.Namespace) {
		if _, exists := deploymentActive[d]; !exists {
			deploymentActive[d] = false
		}
	}

	deployments := make([]*pb.DeploymentInfo, 0, len(deploymentActive))
	for name, active := range deploymentActive {
		deployments = append(deployments, &pb.DeploymentInfo{
			Name:      name,
			Namespace: req.Namespace,
			Active:    active,
		})
	}
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].Name < deployments[j].Name
	})

	return &pb.ListDeploymentsResponse{Deployments: deployments}, nil
}

// inferDeploymentName attempts to derive a deployment name from a pod name
// using the heuristic that the last two dash-separated alphanumeric segments
// are the ReplicaSet hash and pod hash.
func inferDeploymentName(podName string) string {
	parts := strings.Split(podName, "-")
	if len(parts) < 3 {
		return ""
	}
	podHash := parts[len(parts)-1]
	rsHash := parts[len(parts)-2]
	if !isAlphanumLower(podHash) || !isAlphanumLower(rsHash) || len(podHash) < 4 {
		return ""
	}
	return strings.Join(parts[:len(parts)-2], "-")
}

// ── GetDeploymentLogs ─────────────────────────────────────────────────────────

// logEntry is a single log line together with its parsed timestamp, used for
// merge-sorting across multiple pod log files.
type logEntry struct {
	ts   time.Time
	idx  int // global insertion order, used as a tiebreaker for equal timestamps
	line string
}

// logEntryHeap is a min-heap of logEntry, ordered by timestamp with insertion
// order as a tiebreaker so that lines sharing an identical timestamp are always
// returned in the order they were read from their log files.
type logEntryHeap []logEntry

func (h logEntryHeap) Len() int           { return len(h) }
func (h logEntryHeap) Less(i, j int) bool {
	if h[i].ts.Equal(h[j].ts) {
		return h[i].idx < h[j].idx
	}
	return h[i].ts.Before(h[j].ts)
}
func (h logEntryHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *logEntryHeap) Push(x interface{}) { *h = append(*h, x.(logEntry)) }
func (h *logEntryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
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
		return nanosToken{}, status.Error(codes.InvalidArgument, "invalid page_token")
	}
	switch len(b) {
	case 8:
		return nanosToken{nanos: int64(binary.BigEndian.Uint64(b)), backward: false}, nil
	case 9:
		if b[0] != 0x01 {
			return nanosToken{}, status.Error(codes.InvalidArgument, "invalid page_token")
		}
		return nanosToken{nanos: int64(binary.BigEndian.Uint64(b[1:])), backward: true}, nil
	default:
		return nanosToken{}, status.Error(codes.InvalidArgument, "invalid page_token")
	}
}

// GetDeploymentLogs returns a paginated, time-sorted page of log lines from
// all pods belonging to the given deployment.
func (s *LogService) GetDeploymentLogs(_ context.Context, req *pb.GetDeploymentLogsRequest) (*pb.GetDeploymentLogsResponse, error) {
	if req.Namespace == "" || req.Deployment == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace and deployment are required")
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	// Decode the cursor: forward (8-byte nanos) or backward (9-byte with 0x01 flag).
	var afterNanos int64
	var beforeNanos int64
	if req.PageToken != "" {
		tok, err := decodeNanosToken(req.PageToken)
		if err != nil {
			return nil, err
		}
		if tok.backward {
			beforeNanos = tok.nanos
		} else {
			afterNanos = tok.nanos
		}
	}

	// reversed = we want the most recent N lines (last page or backward cursor).
	reversed := req.LoadLastPage || beforeNanos > 0

	pods, err := s.deploymentPodsForNamespace(req.Namespace, req.Deployment)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, status.Errorf(codes.NotFound, "no logs found for deployment %s/%s", req.Namespace, req.Deployment)
	}

	var startTime, endTime time.Time
	if req.StartTime != 0 {
		startTime = time.Unix(req.StartTime, 0)
	}
	if req.EndTime != 0 {
		endTime = time.Unix(req.EndTime, 0)
	}
	afterTime := time.Unix(0, afterNanos)
	beforeTime := time.Unix(0, beforeNanos)

	// Read all matching lines from every pod log file into the heap.
	h := &logEntryHeap{}
	heap.Init(h)
	var insertIdx int

	for _, pod := range pods {
		logPath := filepath.Join(s.logsRoot, req.Namespace, pod+".log")
		f, err := os.Open(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, status.Errorf(codes.Internal, "open log file: %v", err)
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			ts := parseLineTimestamp(line)

			if !startTime.IsZero() && ts.Before(startTime) {
				continue
			}
			if !endTime.IsZero() && ts.After(endTime) {
				continue
			}
			// Forward cursor: skip lines at or before afterTime.
			if afterNanos > 0 && !ts.After(afterTime) {
				continue
			}
			// Backward cursor: skip lines at or after beforeTime.
			if beforeNanos > 0 && !ts.Before(beforeTime) {
				continue
			}
			heap.Push(h, logEntry{ts: ts, idx: insertIdx, line: line})
			insertIdx++
		}
		f.Close()
	}

	// Drain the heap into a sorted slice.
	allSorted := make([]logEntry, 0, h.Len())
	for h.Len() > 0 {
		allSorted = append(allSorted, heap.Pop(h).(logEntry))
	}
	// allSorted is ascending by timestamp.

	resp := &pb.GetDeploymentLogsResponse{}

	if reversed {
		// Take the last pageSize lines (most recent).
		start := len(allSorted) - pageSize
		if start < 0 {
			start = 0
		}
		page := allSorted[start:]
		lines := make([]string, len(page))
		for i, e := range page {
			lines[i] = e.line
		}
		resp.Lines = lines

		// prev_page_token: if there are lines before this page, encode a "before
		// first line of this page" backward cursor.
		if start > 0 && len(page) > 0 {
			resp.PrevPageToken = encodeBackwardNanosToken(page[0].ts.UnixNano())
		}
		// next_page_token: for a backward cursor, provide a forward cursor so
		// the caller can navigate back toward newer logs.
		if beforeNanos > 0 && len(page) > 0 {
			resp.NextPageToken = encodeForwardNanosToken(page[len(page)-1].ts.UnixNano())
		}
		// For load_last_page there is no next page (we are already at the end).
	} else {
		// Take the first pageSize lines.
		end := pageSize
		if end > len(allSorted) {
			end = len(allSorted)
		}
		page := allSorted[:end]
		lines := make([]string, len(page))
		for i, e := range page {
			lines[i] = e.line
		}
		resp.Lines = lines

		// next_page_token: there are more lines after this page.
		if len(allSorted) > pageSize && len(page) > 0 {
			resp.NextPageToken = encodeForwardNanosToken(page[len(page)-1].ts.UnixNano())
		}
		// prev_page_token: only meaningful when we started mid-stream (afterNanos > 0).
		if afterNanos > 0 && len(page) > 0 {
			resp.PrevPageToken = encodeBackwardNanosToken(page[0].ts.UnixNano())
		}
	}

	return resp, nil
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

// ── StreamDeploymentLogs ──────────────────────────────────────────────────────

// StreamDeploymentLogs fans out to StreamLogs for all currently active pods in
// the deployment and multiplexes their output onto a single stream.
func (s *LogService) StreamDeploymentLogs(req *pb.StreamDeploymentLogsRequest, stream pb.LogService_StreamDeploymentLogsServer) error {
	if req.Namespace == "" || req.Deployment == "" {
		return status.Error(codes.InvalidArgument, "namespace and deployment are required")
	}

	pods, err := s.deploymentPodsForNamespace(req.Namespace, req.Deployment)
	if err != nil {
		return err
	}

	// Filter to pods that are currently active and have a log file.
	var activePods []string
	for _, pod := range pods {
		if s.active.IsActive(req.Namespace, pod) {
			activePods = append(activePods, pod)
		}
	}
	// If no active pods, still stream from all pod files so we tail any that
	// have existing log data and pick up new writes.
	if len(activePods) == 0 {
		activePods = pods
	}
	if len(activePods) == 0 {
		return status.Errorf(codes.NotFound, "no logs found for deployment %s/%s", req.Namespace, req.Deployment)
	}

	ctx := stream.Context()
	lineCh := make(chan string, 64)

	var wg sync.WaitGroup
	for _, pod := range activePods {
		wg.Add(1)
		go func(podName string) {
			defer wg.Done()
			tailPodToChannel(ctx, filepath.Join(s.logsRoot, req.Namespace, podName+".log"), lineCh)
		}(pod)
	}

	// Close lineCh once all tailers exit so the range below terminates.
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				return nil
			}
			if err := stream.Send(&pb.StreamDeploymentLogsResponse{Line: line}); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// tailPodToChannel seeks to EOF on the log file and sends new lines to ch
// until ctx is cancelled.
func tailPodToChannel(ctx context.Context, logPath string, ch chan<- string) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return
	}

	br := bufio.NewReader(f)
	for {
		line, readErr := br.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			select {
			case ch <- trimmed:
			case <-ctx.Done():
				return
			}
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
			br.Reset(f)
		}
	}
}
