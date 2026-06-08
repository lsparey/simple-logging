package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lsparey/simple-logging/internal/indexes"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

const (
	defaultPageSize = 200
	maxPageSize     = 1000
)

// ActiveChecker is satisfied by the Collector; it reports whether a pod is
// currently being streamed. Using an interface keeps the API package decoupled
// from the collector package.
type ActiveChecker interface {
	IsActive(namespace, pod string) bool
}

// JsonLoggingChecker is satisfied by the Collector; it reports whether a pod's
// log output has been detected as JSON-formatted.
type JsonLoggingChecker interface {
	IsJsonLogging(namespace, pod string) bool
}

// DeploymentMapper resolves the deployment that owns a pod and enumerates
// deployments seen in a namespace.
type DeploymentMapper interface {
	// GetDeploymentName returns the deployment name for a pod if known.
	GetDeploymentName(namespace, podName string) (string, bool)
	// ListKnownDeployments returns all deployment names observed in the namespace.
	ListKnownDeployments(namespace string) []string
}

// LogService implements the generated pb.LogServiceServer interface.
type LogService struct {
	pb.UnimplementedLogServiceServer
	logsRoot    string
	active      ActiveChecker
	jsonLogging JsonLoggingChecker
	deployments DeploymentMapper
	indexes     *indexes.Manager
}

// NewLogService creates a LogService backed by files in logsRoot.
func NewLogService(logsRoot string, active ActiveChecker, jsonLogging JsonLoggingChecker, deployments DeploymentMapper) *LogService {
	return NewLogServiceWithIndexes(logsRoot, active, jsonLogging, deployments, indexes.NewManager(logsRoot))
}

// NewLogServiceWithIndexes creates a LogService using a shared index manager.
func NewLogServiceWithIndexes(logsRoot string, active ActiveChecker, jsonLogging JsonLoggingChecker, deployments DeploymentMapper, indexManager *indexes.Manager) *LogService {
	return &LogService{logsRoot: logsRoot, active: active, jsonLogging: jsonLogging, deployments: deployments, indexes: indexManager}
}

// ListNamespaces returns the names of all namespace subdirectories under logsRoot.
func (s *LogService) ListNamespaces(_ context.Context, _ *pb.ListNamespacesRequest) (*pb.ListNamespacesResponse, error) {
	entries, err := os.ReadDir(s.logsRoot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read logs root: %v", err)
	}

	var namespaces []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			namespaces = append(namespaces, e.Name())
		}
	}

	return &pb.ListNamespacesResponse{Namespaces: namespaces}, nil
}

// ListPods returns metadata for every pod with a log file in the given namespace.
func (s *LogService) ListPods(_ context.Context, req *pb.ListPodsRequest) (*pb.ListPodsResponse, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	nsDir := filepath.Join(s.logsRoot, req.Namespace)
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &pb.ListPodsResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "read namespace dir: %v", err)
	}

	var pods []*pb.PodInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		podName := strings.TrimSuffix(e.Name(), ".log")
		pods = append(pods, &pb.PodInfo{
			Name:        podName,
			Namespace:   req.Namespace,
			Active:      s.active.IsActive(req.Namespace, podName),
			JsonLogging: s.jsonLogging.IsJsonLogging(req.Namespace, podName),
		})
	}

	return &pb.ListPodsResponse{Pods: pods}, nil
}

// ListLogFiles returns metadata for every persisted pod log and index file.
func (s *LogService) ListLogFiles(_ context.Context, _ *pb.ListLogFilesRequest) (*pb.ListLogFilesResponse, error) {
	namespaceEntries, err := os.ReadDir(s.logsRoot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read logs root: %v", err)
	}

	var files []*pb.LogFileInfo
	var totalSize int64
	for _, namespaceEntry := range namespaceEntries {
		if !namespaceEntry.IsDir() || strings.HasPrefix(namespaceEntry.Name(), ".") {
			continue
		}

		namespace := namespaceEntry.Name()
		entries, err := os.ReadDir(filepath.Join(s.logsRoot, namespace))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "read namespace dir %q: %v", namespace, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				return nil, status.Errorf(codes.Internal, "stat log file %q: %v", entry.Name(), err)
			}

			size := info.Size()
			files = append(files, &pb.LogFileInfo{
				Namespace:        namespace,
				Name:             entry.Name(),
				SizeBytes:        size,
				Kind:             "Log",
				ModifiedAtUnixMs: info.ModTime().UnixMilli(),
			})
			totalSize += size
		}
	}

	indexRoot := filepath.Join(s.logsRoot, ".indexes")
	err = filepath.WalkDir(indexRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relativePath, err := filepath.Rel(indexRoot, path)
		if err != nil {
			return err
		}

		size := info.Size()
		files = append(files, &pb.LogFileInfo{
			Namespace:        ".indexes",
			Name:             filepath.ToSlash(relativePath),
			SizeBytes:        size,
			Kind:             "Index",
			ModifiedAtUnixMs: info.ModTime().UnixMilli(),
		})
		totalSize += size
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "read index files: %v", err)
	}

	return &pb.ListLogFilesResponse{
		Files:          files,
		TotalSizeBytes: totalSize,
	}, nil
}

// encodeOffsetToken encodes a byte offset as a base64 8-byte big-endian token.
func encodeOffsetToken(offset int64) string {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(offset))
	return base64.StdEncoding.EncodeToString(b)
}

// decodeOffsetToken decodes a base64 8-byte big-endian token back to a byte offset.
func decodeOffsetToken(token string) (int64, error) {
	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(b) != 8 {
		return 0, status.Error(codes.InvalidArgument, "invalid page_token")
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}

// findPageStartBefore scans backward through f from endOffset to find the byte
// offset where reading forward would yield exactly n lines ending at endOffset.
// Returns 0 if there are fewer than n lines before endOffset (meaning start
// from the beginning of the file).
func findPageStartBefore(f *os.File, endOffset int64, n int) (int64, error) {
	if n <= 0 || endOffset <= 0 {
		return 0, nil
	}

	const bufSize = 32 * 1024
	count := 0
	pos := endOffset

	// If the file ends with '\n', that is just the line terminator for the last
	// line — skip it so we don't count an empty trailing entry.
	var tail [1]byte
	if _, err := f.ReadAt(tail[:], endOffset-1); err == nil && tail[0] == '\n' {
		count = -1
	}

	for pos > 0 {
		readStart := pos - int64(bufSize)
		if readStart < 0 {
			readStart = 0
		}
		bufLen := int(pos - readStart)
		buf := make([]byte, bufLen)
		if _, err := f.ReadAt(buf, readStart); err != nil {
			return 0, status.Errorf(codes.Internal, "scan log file: %v", err)
		}
		for i := bufLen - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				count++
				if count == n {
					return readStart + int64(i) + 1, nil
				}
			}
		}
		pos = readStart
	}
	return 0, nil // fewer than n lines total
}

// GetLogs returns a paginated, optionally time-filtered page of log lines for
// a specific pod. Pagination is cursor-based: the cursor is a base64-encoded
// 8-byte big-endian byte offset into the log file.
func (s *LogService) GetLogs(_ context.Context, req *pb.GetLogsRequest) (*pb.GetLogsResponse, error) {
	if req.Namespace == "" || req.Pod == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace and pod are required")
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	logPath := filepath.Join(s.logsRoot, req.Namespace, req.Pod+".log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "no logs found for pod %s/%s", req.Namespace, req.Pod)
		}
		return nil, status.Errorf(codes.Internal, "open log file: %v", err)
	}
	defer f.Close()

	fileSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "stat log file: %v", err)
	}

	// Decode the byte-offset cursor from the page token, or compute the start
	// offset for load_last_page.
	var startOffset int64
	switch {
	case req.LoadLastPage:
		startOffset, err = findPageStartBefore(f, fileSize, pageSize)
		if err != nil {
			return nil, err
		}
	case req.PageToken != "":
		startOffset, err = decodeOffsetToken(req.PageToken)
		if err != nil {
			return nil, err
		}
	}

	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return nil, status.Errorf(codes.Internal, "seek log file: %v", err)
	}

	// Wrap the file in a counting reader. After each ReadString call, we can
	// calculate the exact file position after the consumed line as:
	//   startOffset + cr.total - int64(br.Buffered())
	cr := &countingReader{r: f}
	br := bufio.NewReader(cr)

	var startTime, endTime time.Time
	if req.StartTime != 0 {
		startTime = time.Unix(req.StartTime, 0)
	}
	if req.EndTime != 0 {
		endTime = time.Unix(req.EndTime, 0)
	}

	var lines []string
	var lastConsumedOffset int64 = startOffset

	for len(lines) < pageSize {
		line, readErr := br.ReadString('\n')
		if len(line) > 0 {
			// Compute the file offset immediately after this line.
			lastConsumedOffset = startOffset + cr.total - int64(br.Buffered())
			trimmed := strings.TrimRight(line, "\r\n")
			if matchesTimeRange(trimmed, startTime, endTime) {
				lines = append(lines, trimmed)
			}
		}
		if readErr != nil {
			break // io.EOF or unexpected error — stop reading
		}
	}

	resp := &pb.GetLogsResponse{Lines: lines}

	// Only set a next-page token if the page is full AND there is more data.
	if len(lines) == pageSize {
		if _, peekErr := br.Peek(1); peekErr == nil {
			resp.NextPageToken = encodeOffsetToken(lastConsumedOffset)
		}
	}

	// Set a prev-page token when there are lines before the current page start.
	if startOffset > 0 {
		prevStart, scanErr := findPageStartBefore(f, startOffset, pageSize)
		if scanErr != nil {
			return nil, scanErr
		}
		resp.PrevPageToken = encodeOffsetToken(prevStart)
	}

	return resp, nil
}

// matchesTimeRange returns true when the log line's RFC3339 timestamp (the
// first space-delimited token) falls within [start, end]. A zero time means
// no bound on that side. Lines with an unparseable timestamp are always included.
func matchesTimeRange(line string, start, end time.Time) bool {
	if start.IsZero() && end.IsZero() {
		return true
	}
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return true
	}
	ts, err := time.Parse(time.RFC3339, line[:idx])
	if err != nil {
		return true
	}
	if !start.IsZero() && ts.Before(start) {
		return false
	}
	if !end.IsZero() && ts.After(end) {
		return false
	}
	return true
}

// countingReader wraps an io.Reader and tracks the total bytes read from it.
type countingReader struct {
	r     io.Reader
	total int64
}

func (cr *countingReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	cr.total += int64(n)
	return
}

// StreamLogs tails a pod's log file and streams new lines as they arrive.
// It seeks to EOF on open so that only lines written after the call begins are
// delivered. The stream runs until the client cancels the context.
func (s *LogService) StreamLogs(req *pb.StreamLogsRequest, stream pb.LogService_StreamLogsServer) error {
	if req.Namespace == "" || req.Pod == "" {
		return status.Error(codes.InvalidArgument, "namespace and pod are required")
	}

	logPath := filepath.Join(s.logsRoot, req.Namespace, req.Pod+".log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return status.Errorf(codes.NotFound, "no logs found for pod %s/%s", req.Namespace, req.Pod)
		}
		return status.Errorf(codes.Internal, "open log file: %v", err)
	}
	defer f.Close()

	// Start from the current end of file so we only send new content.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return status.Errorf(codes.Internal, "seek log file: %v", err)
	}

	br := bufio.NewReader(f)
	ctx := stream.Context()

	for {
		line, readErr := br.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if sendErr := stream.Send(&pb.StreamLogsResponse{Line: trimmed}); sendErr != nil {
				return sendErr
			}
		}
		if readErr == nil {
			// There may be more data immediately; loop without sleeping.
			continue
		}
		if readErr != io.EOF {
			return status.Errorf(codes.Internal, "read log file: %v", readErr)
		}
		// Reached EOF — wait briefly for new data or client cancellation.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
			// Reset the reader so the next ReadString picks up new bytes.
			br.Reset(f)
		}
	}
}
