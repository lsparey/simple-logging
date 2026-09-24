package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/lsparey/simple-logging/internal/indexes"
	"github.com/lsparey/simple-logging/internal/metrics"
	"github.com/lsparey/simple-logging/internal/storage"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
)

const (
	// maxListedLogFiles bounds how many files ListLogFiles returns, so a
	// namespace with a huge number of log/index files doesn't make the
	// storage dashboard slow to load.
	maxListedLogFiles = 50

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

// LogService implements the generated simplelogv1connect.LogServiceHandler
// interface.
type LogService struct {
	simplelogv1connect.UnimplementedLogServiceHandler
	logsRoot    string
	active      ActiveChecker
	jsonLogging JsonLoggingChecker
	indexes     *indexes.Manager

	// diskHighWaterPercent/diskLowWaterPercent are the configured disk guard
	// thresholds, reported by ListLogFiles alongside current usage. Zero
	// until SetDiskWaterMarks is called.
	diskHighWaterPercent int
	diskLowWaterPercent  int

	// metrics backs GetStats and the search counters. Nil (the default)
	// makes GetStats report zeros.
	metrics *metrics.Metrics
}

// SetMetrics makes the service record search metrics in m and report m from
// GetStats.
func (s *LogService) SetMetrics(m *metrics.Metrics) {
	s.metrics = m
}

// SetDiskWaterMarks records the disk guard's configured thresholds so
// ListLogFiles can report them alongside current usage.
func (s *LogService) SetDiskWaterMarks(highPercent, lowPercent int) {
	s.diskHighWaterPercent = highPercent
	s.diskLowWaterPercent = lowPercent
}

// NewLogService creates a LogService backed by files in logsRoot.
func NewLogService(logsRoot string, active ActiveChecker, jsonLogging JsonLoggingChecker) *LogService {
	return NewLogServiceWithIndexes(logsRoot, active, jsonLogging, indexes.NewManager(logsRoot))
}

// NewLogServiceWithIndexes creates a LogService using a shared index manager.
func NewLogServiceWithIndexes(logsRoot string, active ActiveChecker, jsonLogging JsonLoggingChecker, indexManager *indexes.Manager) *LogService {
	return &LogService{logsRoot: logsRoot, active: active, jsonLogging: jsonLogging, indexes: indexManager}
}

// ListNamespaces returns the names of all namespace subdirectories under logsRoot.
func (s *LogService) ListNamespaces(_ context.Context, _ *connect.Request[pb.ListNamespacesRequest]) (*connect.Response[pb.ListNamespacesResponse], error) {
	entries, err := os.ReadDir(s.logsRoot)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read logs root: %v", err))
	}

	var namespaces []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			namespaces = append(namespaces, e.Name())
		}
	}

	return connect.NewResponse(&pb.ListNamespacesResponse{Namespaces: namespaces}), nil
}

// ListPods returns metadata for every pod with a log directory in the given namespace.
func (s *LogService) ListPods(_ context.Context, r *connect.Request[pb.ListPodsRequest]) (*connect.Response[pb.ListPodsResponse], error) {
	req := r.Msg
	if req.Namespace == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("namespace is required"))
	}

	podNames, err := storage.ListPodDirs(s.logsRoot, req.Namespace)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read namespace dir: %v", err))
	}

	pods := make([]*pb.PodInfo, 0, len(podNames))
	for _, podName := range podNames {
		containers, err := storage.ListContainers(s.logsRoot, req.Namespace, podName)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read pod dir %q: %v", podName, err))
		}
		pods = append(pods, &pb.PodInfo{
			Name:        podName,
			Namespace:   req.Namespace,
			Active:      s.active.IsActive(req.Namespace, podName),
			JsonLogging: s.jsonLogging.IsJsonLogging(req.Namespace, podName),
			Containers:  containers,
		})
	}

	return connect.NewResponse(&pb.ListPodsResponse{Pods: pods}), nil
}

// ListLogFiles returns metadata summarising the largest persisted pod log
// segments and index files, up to maxListedLogFiles, along with totals across
// all of them. Log segments are grouped by (namespace, pod, container),
// since a container's history is now many daily files rather than one.
func (s *LogService) ListLogFiles(_ context.Context, _ *connect.Request[pb.ListLogFilesRequest]) (*connect.Response[pb.ListLogFilesResponse], error) {
	namespaceEntries, err := os.ReadDir(s.logsRoot)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read logs root: %v", err))
	}

	type logSummary struct {
		namespace, pod, container string
		fileCount                 int
		sizeBytes                 int64
		modifiedAtUnixMs          int64
	}
	logSummaries := make(map[string]*logSummary)
	var totalSize int64
	var totalLogFileCount, totalIndexFileCount int32

	for _, namespaceEntry := range namespaceEntries {
		if !namespaceEntry.IsDir() || strings.HasPrefix(namespaceEntry.Name(), ".") {
			continue
		}
		namespace := namespaceEntry.Name()

		pods, err := storage.ListPodDirs(s.logsRoot, namespace)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read namespace dir %q: %v", namespace, err))
		}
		for _, pod := range pods {
			containers, err := storage.ListContainers(s.logsRoot, namespace, pod)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read pod dir %q: %v", pod, err))
			}
			for _, container := range containers {
				containerDir := storage.ContainerDir(s.logsRoot, namespace, pod, container)
				entries, err := os.ReadDir(containerDir)
				if err != nil {
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read container dir %q: %v", containerDir, err))
				}
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					if _, err := storage.SegmentDate(entry.Name()); err != nil {
						continue
					}
					info, err := entry.Info()
					if err != nil {
						return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat log segment %q: %v", entry.Name(), err))
					}
					key := namespace + "/" + pod + "/" + container
					summary := logSummaries[key]
					if summary == nil {
						summary = &logSummary{namespace: namespace, pod: pod, container: container}
						logSummaries[key] = summary
					}
					size := info.Size()
					summary.fileCount++
					summary.sizeBytes += size
					summary.modifiedAtUnixMs = max(summary.modifiedAtUnixMs, info.ModTime().UnixMilli())
					totalSize += size
					totalLogFileCount++
				}
			}
		}
	}

	var files []*pb.LogFileInfo
	for _, summary := range logSummaries {
		fileLabel := fmt.Sprintf("%d files", summary.fileCount)
		if summary.fileCount == 1 {
			fileLabel = "1 file"
		}
		files = append(files, &pb.LogFileInfo{
			Namespace:        summary.namespace,
			Name:             fileLabel,
			SizeBytes:        summary.sizeBytes,
			Kind:             "Log",
			ModifiedAtUnixMs: summary.modifiedAtUnixMs,
			Subject:          fmt.Sprintf("%s / %s (%s)", summary.namespace, summary.pod, summary.container),
		})
	}

	type indexSummary struct {
		fileCount        int
		sizeBytes        int64
		modifiedAtUnixMs int64
	}
	indexSummaries := make(map[string]*indexSummary)
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
		subject := indexFileGroup(relativePath)
		summary := indexSummaries[subject]
		if summary == nil {
			summary = &indexSummary{}
			indexSummaries[subject] = summary
		}
		summary.fileCount++
		summary.sizeBytes += size
		summary.modifiedAtUnixMs = max(summary.modifiedAtUnixMs, info.ModTime().UnixMilli())
		totalSize += size
		totalIndexFileCount++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read index files: %v", err))
	}
	for subject, summary := range indexSummaries {
		fileLabel := fmt.Sprintf("%d files", summary.fileCount)
		if summary.fileCount == 1 {
			fileLabel = "1 file"
		}
		if subject == "Index metadata" {
			fileLabel = "indexes.json"
		}
		files = append(files, &pb.LogFileInfo{
			Namespace:        ".indexes",
			Name:             fileLabel,
			SizeBytes:        summary.sizeBytes,
			Kind:             "Index",
			ModifiedAtUnixMs: summary.modifiedAtUnixMs,
			Subject:          subject,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].SizeBytes > files[j].SizeBytes
	})
	if len(files) > maxListedLogFiles {
		files = files[:maxListedLogFiles]
	}

	diskUsedPercent, err := storage.DiskUsedPercent(s.logsRoot)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check disk usage: %v", err))
	}

	return connect.NewResponse(&pb.ListLogFilesResponse{
		Files:                files,
		TotalSizeBytes:       totalSize,
		TotalLogFileCount:    totalLogFileCount,
		TotalIndexFileCount:  totalIndexFileCount,
		DiskUsedPercent:      int32(diskUsedPercent),
		DiskHighWaterPercent: int32(s.diskHighWaterPercent),
		DiskLowWaterPercent:  int32(s.diskLowWaterPercent),
	}), nil
}

func indexFileGroup(relativePath string) string {
	if filepath.ToSlash(relativePath) == "indexes.json" {
		return "Index metadata"
	}

	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	if len(parts) < 4 || parts[0] != "keys" || (parts[2] != "values" && parts[2] != "shards") {
		return "Index data"
	}

	keyBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "Index data"
	}

	return string(keyBytes)
}

// ── Segmented log reading ────────────────────────────────────────────────────

// logChunk is one container's log segment file for a pod.
type logChunk struct {
	container string
	segment   string // "YYYY-MM-DD"
	path      string
}

// podChunks returns every segment file for every container of a pod, ordered
// by segment date then container name. A pod has only ever had one collected
// container in practice (multi-container collection is a later phase), so
// this ordering is exactly chronological in the case that matters today; with
// multiple containers it is a day-granularity approximation, refined once
// per-container merging lands.
func podChunks(logsRoot, namespace, pod string) ([]logChunk, error) {
	containers, err := storage.ListContainers(logsRoot, namespace, pod)
	if err != nil {
		return nil, err
	}
	var chunks []logChunk
	for _, container := range containers {
		segments, err := storage.ListSegments(logsRoot, namespace, pod, container)
		if err != nil {
			return nil, err
		}
		for _, segment := range segments {
			chunks = append(chunks, logChunk{
				container: container,
				segment:   segment,
				path:      filepath.Join(storage.ContainerDir(logsRoot, namespace, pod, container), segment+".log"),
			})
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].segment != chunks[j].segment {
			return chunks[i].segment < chunks[j].segment
		}
		return chunks[i].container < chunks[j].container
	})
	return chunks, nil
}

func findChunkIndex(chunks []logChunk, container, segment string) int {
	for i, c := range chunks {
		if c.container == container && c.segment == segment {
			return i
		}
	}
	return -1
}

// latestSegmentPath returns the path of the newest segment for a container,
// or "" if it has none.
func latestSegmentPath(logsRoot, namespace, pod, container string) (string, error) {
	segments, err := storage.ListSegments(logsRoot, namespace, pod, container)
	if err != nil {
		return "", err
	}
	if len(segments) == 0 {
		return "", nil
	}
	return filepath.Join(storage.ContainerDir(logsRoot, namespace, pod, container), segments[len(segments)-1]+".log"), nil
}

// logsPageToken is a GetLogs pagination cursor: which segment and byte offset
// within it a page starts or ends at. Segment identity (rather than a raw
// index into the current chunk list) keeps a token valid across new segments
// appearing or old ones being deleted by retention between calls.
type logsPageToken struct {
	Container string `json:"c"`
	Segment   string `json:"s"`
	Offset    int64  `json:"o"`
}

func encodeLogsPageToken(t logsPageToken) string {
	b, _ := json.Marshal(t) //nolint:errcheck // logsPageToken always marshals
	return base64.StdEncoding.EncodeToString(b)
}

func decodeLogsPageToken(token string) (logsPageToken, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return logsPageToken{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	var t logsPageToken
	if err := json.Unmarshal(raw, &t); err != nil {
		return logsPageToken{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	return t, nil
}

// chunkReader reads lines sequentially across a pod's log chunks, advancing
// to the next chunk transparently at each segment's EOF.
type chunkReader struct {
	chunks []logChunk
	idx    int
	offset int64
	f      *os.File
	cr     *countingReader
	br     *bufio.Reader
}

func openChunkReader(chunks []logChunk, idx int, offset int64) (*chunkReader, error) {
	r := &chunkReader{chunks: chunks}
	if err := r.openAt(idx, offset); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *chunkReader) openAt(idx int, offset int64) error {
	f, err := os.Open(r.chunks[idx].path)
	if err != nil {
		return err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return err
	}
	r.idx = idx
	r.offset = offset
	r.f = f
	r.cr = &countingReader{r: f}
	r.br = bufio.NewReader(r.cr)
	return nil
}

func (r *chunkReader) close() {
	if r.f != nil {
		r.f.Close()
	}
}

// position returns the (chunk index, byte offset) immediately after the last
// line returned by readLine.
func (r *chunkReader) position() (int, int64) {
	return r.idx, r.offset + r.cr.total - int64(r.br.Buffered())
}

// readLine returns the next line across chunk boundaries. ok is false once
// every chunk has been exhausted.
func (r *chunkReader) readLine() (line string, ok bool, err error) {
	for {
		raw, readErr := r.br.ReadString('\n')
		if len(raw) > 0 {
			return strings.TrimRight(raw, "\r\n"), true, nil
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF {
			return "", false, readErr
		}
		r.f.Close()
		next := r.idx + 1
		if next >= len(r.chunks) {
			return "", false, nil
		}
		if err := r.openAt(next, 0); err != nil {
			return "", false, err
		}
	}
}

// hasMoreAfterCurrent reports whether there is at least one more byte to read,
// either later in the current chunk or in a subsequent one.
func (r *chunkReader) hasMoreAfterCurrent() bool {
	if _, err := r.br.Peek(1); err == nil {
		return true
	}
	return r.idx+1 < len(r.chunks)
}

// scanBackwardInFile scans backward from endOffset in f, looking for the byte
// offset that is exactly need lines before it. It returns how many lines were
// actually found (equal to need on success); if the file has fewer than need
// lines before endOffset, found < need and offset is 0.
func scanBackwardInFile(f *os.File, endOffset int64, need int) (offset int64, found int, err error) {
	if need <= 0 || endOffset <= 0 {
		return 0, 0, nil
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
			return 0, 0, err
		}
		for i := bufLen - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				count++
				if count == need {
					return readStart + int64(i) + 1, need, nil
				}
			}
		}
		pos = readStart
	}
	if count < 0 {
		count = 0
	}
	return 0, count, nil
}

// findChunkPageStartBefore finds the (chunk index, offset) that is exactly n
// lines before (chunkIdx, offset), scanning backward across chunk boundaries.
// It returns (0, 0) — the start of the first chunk — if chunks has fewer than
// n lines before that position.
func findChunkPageStartBefore(chunks []logChunk, chunkIdx int, offset int64, n int) (int, int64, error) {
	remaining := n
	for i := chunkIdx; i >= 0; i-- {
		end := offset
		if i != chunkIdx {
			info, err := os.Stat(chunks[i].path)
			if err != nil {
				continue // segment vanished (retention race) — skip it
			}
			end = info.Size()
		}
		f, err := os.Open(chunks[i].path)
		if err != nil {
			continue
		}
		start, found, serr := scanBackwardInFile(f, end, remaining)
		f.Close()
		if serr != nil {
			return 0, 0, serr
		}
		remaining -= found
		if remaining <= 0 {
			return i, start, nil
		}
	}
	return 0, 0, nil
}

// GetLogs returns a paginated, optionally time-filtered page of log lines for
// a specific pod, across all of its stored log segments. Pagination is
// cursor-based: the cursor identifies a segment and a byte offset within it.
func (s *LogService) GetLogs(_ context.Context, r *connect.Request[pb.GetLogsRequest]) (*connect.Response[pb.GetLogsResponse], error) {
	req := r.Msg
	if req.Namespace == "" || req.Pod == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("namespace and pod are required"))
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	chunks, err := podChunks(s.logsRoot, req.Namespace, req.Pod)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list log segments: %v", err))
	}
	if len(chunks) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no logs found for pod %s/%s", req.Namespace, req.Pod))
	}

	var startChunk int
	var startOffset int64
	switch {
	case req.LoadLastPage:
		lastIdx := len(chunks) - 1
		info, statErr := os.Stat(chunks[lastIdx].path)
		if statErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("stat log segment: %v", statErr))
		}
		startChunk, startOffset, err = findChunkPageStartBefore(chunks, lastIdx, info.Size(), pageSize)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("scan log segments: %v", err))
		}
	case req.PageToken != "":
		tok, terr := decodeLogsPageToken(req.PageToken)
		if terr != nil {
			return nil, terr
		}
		idx := findChunkIndex(chunks, tok.Container, tok.Segment)
		if idx < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		startChunk, startOffset = idx, tok.Offset
	}

	reader, err := openChunkReader(chunks, startChunk, startOffset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("open log segment: %v", err))
	}
	defer reader.close()

	var startTime, endTime time.Time
	if req.StartTime != 0 {
		startTime = time.Unix(req.StartTime, 0)
	}
	if req.EndTime != 0 {
		endTime = time.Unix(req.EndTime, 0)
	}

	var lines []string
	lastChunk, lastOffset := startChunk, startOffset
	for len(lines) < pageSize {
		line, ok, readErr := reader.readLine()
		if readErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read log segment: %v", readErr))
		}
		if !ok {
			break
		}
		lastChunk, lastOffset = reader.position()
		if matchesTimeRange(line, startTime, endTime) {
			lines = append(lines, line)
		}
	}

	resp := &pb.GetLogsResponse{Lines: lines}

	// Only set a next-page token if the page is full AND there is more data.
	if len(lines) == pageSize && reader.hasMoreAfterCurrent() {
		resp.NextPageToken = encodeLogsPageToken(logsPageToken{
			Container: chunks[lastChunk].container,
			Segment:   chunks[lastChunk].segment,
			Offset:    lastOffset,
		})
	}

	// Set a prev-page token when there are lines before the current page start.
	if startChunk > 0 || startOffset > 0 {
		prevChunk, prevOffset, perr := findChunkPageStartBefore(chunks, startChunk, startOffset, pageSize)
		if perr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("scan log segments: %v", perr))
		}
		resp.PrevPageToken = encodeLogsPageToken(logsPageToken{
			Container: chunks[prevChunk].container,
			Segment:   chunks[prevChunk].segment,
			Offset:    prevOffset,
		})
	}

	return connect.NewResponse(resp), nil
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

// tailLatestSegment tails a pod's most recent log segment, delivering each
// new line to onLine, until ctx is cancelled or onLine returns an error. It
// switches to a newer segment automatically when one appears (a UTC day
// rollover while the tail is open).
//
// A pod is assumed to have exactly one actively-collected container (multi-
// container collection is a later phase); the container followed is whichever
// sorts last among those with any stored logs.
func tailLatestSegment(ctx context.Context, logsRoot, namespace, pod string, onLine func(string) error) error {
	chunks, err := podChunks(logsRoot, namespace, pod)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return os.ErrNotExist
	}

	container := chunks[len(chunks)-1].container
	currentPath := chunks[len(chunks)-1].path

	var f *os.File
	defer func() {
		if f != nil {
			f.Close()
		}
	}()

	f, err = os.Open(currentPath)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	br := bufio.NewReader(f)
	for {
		line, readErr := br.ReadString('\n')
		if len(line) > 0 {
			if err := onLine(strings.TrimRight(line, "\r\n")); err != nil {
				return err
			}
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF {
			return readErr
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}

		if next, nerr := latestSegmentPath(logsRoot, namespace, pod, container); nerr == nil && next != "" && next != currentPath {
			f.Close()
			f, err = os.Open(next)
			if err != nil {
				return err
			}
			currentPath = next
		}
		br.Reset(f)
	}
}

// StreamLogs tails a pod's most recent log segment and streams new lines as
// they arrive, switching segments across a UTC day rollover. It starts from
// the current end of the segment so only lines written after the call begins
// are delivered. The stream runs until the client cancels the context.
func (s *LogService) StreamLogs(ctx context.Context, r *connect.Request[pb.StreamLogsRequest], stream *connect.ServerStream[pb.StreamLogsResponse]) error {
	req := r.Msg
	if req.Namespace == "" || req.Pod == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("namespace and pod are required"))
	}

	err := tailLatestSegment(ctx, s.logsRoot, req.Namespace, req.Pod, func(line string) error {
		return stream.Send(&pb.StreamLogsResponse{Line: line})
	})
	if err != nil {
		if os.IsNotExist(err) {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("no logs found for pod %s/%s", req.Namespace, req.Pod))
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("read log segment: %v", err))
	}
	return nil
}
