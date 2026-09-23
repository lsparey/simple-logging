package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// ListDeployments, GetDeploymentLogs and StreamDeploymentLogs are thin
// wrappers around the equivalent Workload RPCs with kind="Deployment",
// kept for one release so existing clients don't break; see ListWorkloads,
// GetWorkloadLogs and StreamWorkloadLogs in workload_service.go for the
// real implementation. Remove alongside the frontend's switch to the
// Workload RPCs.

// ListDeployments returns all deployments with log files in the given namespace.
//
// Deprecated: superseded by ListWorkloads.
func (s *LogService) ListDeployments(ctx context.Context, req *pb.ListDeploymentsRequest) (*pb.ListDeploymentsResponse, error) {
	resp, err := s.ListWorkloads(ctx, &pb.ListWorkloadsRequest{Namespace: req.Namespace})
	if err != nil {
		return nil, err
	}
	deployments := make([]*pb.DeploymentInfo, 0, len(resp.Workloads))
	for _, w := range resp.Workloads {
		if w.Kind != "Deployment" {
			continue
		}
		deployments = append(deployments, &pb.DeploymentInfo{
			Name:        w.Name,
			Namespace:   w.Namespace,
			Active:      w.Active,
			JsonLogging: w.JsonLogging,
		})
	}
	return &pb.ListDeploymentsResponse{Deployments: deployments}, nil
}

// GetDeploymentLogs returns a paginated, time-sorted page of log lines from
// all pods belonging to the given deployment.
//
// Deprecated: superseded by GetWorkloadLogs.
func (s *LogService) GetDeploymentLogs(ctx context.Context, req *pb.GetDeploymentLogsRequest) (*pb.GetDeploymentLogsResponse, error) {
	if req.Namespace == "" || req.Deployment == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace and deployment are required")
	}
	resp, err := s.GetWorkloadLogs(ctx, &pb.GetWorkloadLogsRequest{
		Namespace:    req.Namespace,
		Kind:         "Deployment",
		Name:         req.Deployment,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		PageSize:     req.PageSize,
		PageToken:    req.PageToken,
		LoadLastPage: req.LoadLastPage,
	})
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "no logs found for deployment %s/%s", req.Namespace, req.Deployment)
		}
		return nil, err
	}
	return &pb.GetDeploymentLogsResponse{
		Lines:         resp.Lines,
		NextPageToken: resp.NextPageToken,
		PrevPageToken: resp.PrevPageToken,
	}, nil
}

// deploymentToWorkloadStream adapts a StreamDeploymentLogs server stream to
// the LogService_StreamWorkloadLogsServer interface StreamWorkloadLogs
// expects, translating each sent message. All other grpc.ServerStream
// methods (Context, SetHeader, ...) are satisfied by the embedded stream.
type deploymentToWorkloadStream struct {
	pb.LogService_StreamDeploymentLogsServer
}

func (a *deploymentToWorkloadStream) Send(resp *pb.StreamWorkloadLogsResponse) error {
	return a.LogService_StreamDeploymentLogsServer.Send(&pb.StreamDeploymentLogsResponse{Line: resp.Line})
}

// StreamDeploymentLogs tails all active pods for a deployment and streams
// merged log lines in real time.
//
// Deprecated: superseded by StreamWorkloadLogs.
func (s *LogService) StreamDeploymentLogs(req *pb.StreamDeploymentLogsRequest, stream pb.LogService_StreamDeploymentLogsServer) error {
	if req.Namespace == "" || req.Deployment == "" {
		return status.Error(codes.InvalidArgument, "namespace and deployment are required")
	}
	err := s.StreamWorkloadLogs(&pb.StreamWorkloadLogsRequest{
		Namespace: req.Namespace,
		Kind:      "Deployment",
		Name:      req.Deployment,
	}, &deploymentToWorkloadStream{stream})
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return status.Errorf(codes.NotFound, "no logs found for deployment %s/%s", req.Namespace, req.Deployment)
		}
		return err
	}
	return nil
}

// ── Merge-sort and pagination helpers, shared with workload_service.go ──────

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
