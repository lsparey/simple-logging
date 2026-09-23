package api

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/internal/storage"
)

const (
	defaultSearchMaxResults = 1000
	maxSearchMaxResults     = 5000
)

// searchJob is one segment file to scan for SearchLogs.
type searchJob struct {
	namespace, pod, container string
	segmentDate               time.Time
	path                      string
}

// searchTargets lists the pods/containers to search, honoring the
// namespace/workload/pod/container scoping on req.
func (s *LogService) searchTargets(req *pb.SearchLogsRequest) ([]searchTarget, error) {
	var namespaces []string
	if req.Namespace != "" {
		namespaces = []string{req.Namespace}
	} else {
		entries, err := os.ReadDir(s.logsRoot)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "read logs root: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				namespaces = append(namespaces, e.Name())
			}
		}
	}

	var targets []searchTarget
	for _, namespace := range namespaces {
		var pods []string
		var err error
		switch {
		case req.WorkloadKind != "" && req.WorkloadName != "":
			pods, err = s.workloadPodsForNamespace(namespace, req.WorkloadKind, req.WorkloadName)
		case req.Pod != "":
			pods = []string{req.Pod}
		default:
			pods, err = storage.ListPodDirs(s.logsRoot, namespace)
		}
		if err != nil {
			return nil, err
		}

		for _, pod := range pods {
			var containers []string
			if req.Container != "" {
				containers = []string{req.Container}
			} else {
				containers, err = storage.ListContainers(s.logsRoot, namespace, pod)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "read pod dir %q: %v", pod, err)
				}
			}
			for _, container := range containers {
				targets = append(targets, searchTarget{namespace: namespace, pod: pod, container: container})
			}
		}
	}
	return targets, nil
}

// searchTarget identifies one pod's container to search.
type searchTarget struct {
	namespace, pod, container string
}

// searchJobsFor expands targets into per-segment-file jobs, pruned to those
// whose date falls within [start, end] (zero time = unbounded on that side).
// Segment-level pruning is what keeps a time-scoped search fast: a "last 24
// hours" search touches at most two files per container regardless of how
// much history is retained.
func (s *LogService) searchJobsFor(targets []searchTarget, start, end time.Time) ([]searchJob, error) {
	var jobs []searchJob
	for _, t := range targets {
		segments, err := storage.ListSegments(s.logsRoot, t.namespace, t.pod, t.container)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list log segments: %v", err)
		}
		for _, segment := range segments {
			date, err := storage.SegmentDate(segment + ".log")
			if err != nil {
				continue
			}
			if !start.IsZero() && date.Before(truncateToDay(start)) {
				continue
			}
			if !end.IsZero() && date.After(end) {
				continue
			}
			jobs = append(jobs, searchJob{
				namespace:   t.namespace,
				pod:         t.pod,
				container:   t.container,
				segmentDate: date,
				path:        filepath.Join(storage.ContainerDir(s.logsRoot, t.namespace, t.pod, t.container), segment+".log"),
			})
		}
	}
	return jobs, nil
}

func truncateToDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// searchMatcher reports whether a log line matches the search query.
type searchMatcher func(line string) bool

func newSearchMatcher(query string, isRegex bool) (searchMatcher, error) {
	if isRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid regex: %v", err)
		}
		return re.MatchString, nil
	}
	lower := strings.ToLower(query)
	return func(line string) bool {
		return strings.Contains(strings.ToLower(line), lower)
	}, nil
}

// SearchLogs streams log lines matching req.Query across every pod/container
// selected by the namespace/workload/pod/container scoping, bounded by
// req.MaxResults.
//
// Matching files are scanned concurrently by a bounded worker pool (one
// worker per CPU), in job-queue order (newest-first or oldest-first per
// req.NewestFirst) so that when max_results truncates the search, the
// dropped matches are the least relevant ones. Because workers run
// concurrently, matches from different pods/containers are streamed to the
// client in the order their scans complete, not in strict global timestamp
// order; matches within a single file are always in line (chronological)
// order. A future revision could add a true k-way merge if strict ordering
// turns out to matter in practice.
func (s *LogService) SearchLogs(req *pb.SearchLogsRequest, stream pb.LogService_SearchLogsServer) error {
	if req.Query == "" {
		return status.Error(codes.InvalidArgument, "query is required")
	}

	matches, err := newSearchMatcher(req.Query, req.Regex)
	if err != nil {
		return err
	}

	maxResults := int(req.MaxResults)
	if maxResults <= 0 {
		maxResults = defaultSearchMaxResults
	}
	if maxResults > maxSearchMaxResults {
		maxResults = maxSearchMaxResults
	}

	var start, end time.Time
	if req.StartTimeUnixMs != 0 {
		start = time.UnixMilli(req.StartTimeUnixMs)
	}
	if req.EndTimeUnixMs != 0 {
		end = time.UnixMilli(req.EndTimeUnixMs)
	}

	targets, err := s.searchTargets(req)
	if err != nil {
		return err
	}
	jobs, err := s.searchJobsFor(targets, start, end)
	if err != nil {
		return err
	}
	sort.Slice(jobs, func(i, j int) bool {
		if !jobs[i].segmentDate.Equal(jobs[j].segmentDate) {
			if req.NewestFirst {
				return jobs[i].segmentDate.After(jobs[j].segmentDate)
			}
			return jobs[i].segmentDate.Before(jobs[j].segmentDate)
		}
		if jobs[i].pod != jobs[j].pod {
			return jobs[i].pod < jobs[j].pod
		}
		return jobs[i].container < jobs[j].container
	})

	ctx := stream.Context()
	jobCh := make(chan searchJob)
	resultCh := make(chan *pb.SearchLogsResponse, 64)
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopCh) }) }

	var sent int64

	workers := runtime.GOMAXPROCS(0)
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if searchDone(ctx, stopCh) {
					return
				}
				scanSearchJob(ctx, stopCh, job, matches, start, end, resultCh)
			}
		}()
	}

	go func() {
		defer close(jobCh)
		for _, job := range jobs {
			select {
			case jobCh <- job:
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	truncated := false
	for result := range resultCh {
		if err := ctx.Err(); err != nil {
			stop()
			return status.FromContextError(err).Err()
		}
		if err := stream.Send(result); err != nil {
			stop()
			return err
		}
		sent++
		if sent >= int64(maxResults) {
			truncated = true
			stop()
			break
		}
	}
	// Drain resultCh so scanSearchJob's sends don't block forever after we
	// stop reading (workers select on stopCh too, but a send may already be
	// in flight when stop() is called).
	go func() {
		for range resultCh { //nolint:revive // drain
		}
	}()

	if truncated {
		return stream.Send(&pb.SearchLogsResponse{Truncated: true})
	}
	return nil
}

func searchDone(ctx context.Context, stopCh <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return true
	case <-stopCh:
		return true
	default:
		return false
	}
}

// scanSearchJob scans one segment file for matching lines, sending each to
// resultCh until the file is exhausted or stopCh/ctx signals cancellation.
func scanSearchJob(ctx context.Context, stopCh <-chan struct{}, job searchJob, matches searchMatcher, start, end time.Time, resultCh chan<- *pb.SearchLogsResponse) {
	f, err := os.Open(job.path)
	if err != nil {
		return // segment vanished (retention race) — skip it
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if searchDone(ctx, stopCh) {
			return
		}
		line := scanner.Text()
		ts := parseLineTimestamp(line)
		if !start.IsZero() && ts.Before(start) {
			continue
		}
		if !end.IsZero() && ts.After(end) {
			continue
		}
		if !matches(line) {
			continue
		}
		select {
		case resultCh <- &pb.SearchLogsResponse{
			Line:      line,
			Namespace: job.namespace,
			Pod:       job.pod,
			Container: job.container,
		}:
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}
