package api

import (
	"bufio"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"connectrpc.com/connect"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/internal/storage"
)

// workloadPodsForNamespace returns all pod names on disk whose resolved owner
// (persisted in meta.json by the collector, see storage.RecordOwner) matches
// (kind, name), sorted alphabetically. A pod owned by a CronJob-created Job
// matches both its own "Job" entry and its "CronJob" entry.
//
// kind "Pod" is special-cased to match name against the pod's own name
// directly, regardless of its real owner: it means "this one pod's logs",
// the individual-pod counterpart to the other kinds' owner-based grouping
// (used by the sidebar's Pods section, which lists every pod, not just
// unowned ones — most real pods have an owner, so restricting "Pod" to
// bare/unowned pods here would make that section mostly empty).
//
// A pod the collector hasn't (re-)observed since upgrading to a version that
// records ownership has no OwnerKind yet and is not returned by any
// owner-based lookup until it is; this self-heals as the informer's initial
// list resyncs every currently-running pod. It is unaffected by kind "Pod"
// lookups, which don't consult OwnerKind at all.
func (s *LogService) workloadPodsForNamespace(namespace, kind, name string) ([]string, error) {
	podNames, err := storage.ListPodDirs(s.logsRoot, namespace)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read namespace dir: %v", err))
	}

	if kind == "Pod" {
		for _, podName := range podNames {
			if podName == name {
				return []string{podName}, nil
			}
		}
		return nil, nil
	}

	var pods []string
	for _, podName := range podNames {
		meta, err := storage.ReadPodMeta(s.logsRoot, namespace, podName)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read pod meta %q: %v", podName, err))
		}
		if meta.OwnerKind == kind && meta.OwnerName == name {
			pods = append(pods, podName)
			continue
		}
		if kind == "CronJob" && meta.OwnerKind == "Job" && meta.CronJobName == name {
			pods = append(pods, podName)
		}
	}
	sort.Strings(pods)
	return pods, nil
}

// workloadKey identifies one workload entry: a (kind, name) pair, scoped to
// the namespace being listed.
type workloadKey struct{ kind, name string }

// ListWorkloads returns every workload with log files in the given namespace:
// Deployment, StatefulSet, DaemonSet, Job, CronJob, or bare Pod.
func (s *LogService) ListWorkloads(_ context.Context, r *connect.Request[pb.ListWorkloadsRequest]) (*connect.Response[pb.ListWorkloadsResponse], error) {
	req := r.Msg
	if req.Namespace == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("namespace is required"))
	}

	podNames, err := storage.ListPodDirs(s.logsRoot, req.Namespace)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read namespace dir: %v", err))
	}

	active := make(map[workloadKey]bool)
	jsonLog := make(map[workloadKey]bool)
	pods := make(map[workloadKey][]string)

	addPod := func(k workloadKey, podName string) {
		isActive := s.active.IsActive(req.Namespace, podName)
		if cur, ok := active[k]; ok {
			active[k] = cur || isActive
		} else {
			active[k] = isActive
		}
		if s.jsonLogging.IsJsonLogging(req.Namespace, podName) {
			jsonLog[k] = true
		} else if _, ok := jsonLog[k]; !ok {
			jsonLog[k] = false
		}
		pods[k] = append(pods[k], podName)
	}

	for _, podName := range podNames {
		meta, err := storage.ReadPodMeta(s.logsRoot, req.Namespace, podName)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read pod meta %q: %v", podName, err))
		}
		if meta.OwnerKind == "" {
			continue // not yet (re-)observed by the collector since upgrading
		}
		addPod(workloadKey{meta.OwnerKind, meta.OwnerName}, podName)
		if meta.OwnerKind == "Job" && meta.CronJobName != "" {
			addPod(workloadKey{"CronJob", meta.CronJobName}, podName)
		}
	}

	workloads := make([]*pb.WorkloadInfo, 0, len(active))
	for k, isActive := range active {
		podList := pods[k]
		sort.Strings(podList)
		workloads = append(workloads, &pb.WorkloadInfo{
			Kind:        k.kind,
			Name:        k.name,
			Namespace:   req.Namespace,
			Active:      isActive,
			JsonLogging: jsonLog[k],
			Pods:        podList,
		})
	}
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].Kind != workloads[j].Kind {
			return workloads[i].Kind < workloads[j].Kind
		}
		return workloads[i].Name < workloads[j].Name
	})

	return connect.NewResponse(&pb.ListWorkloadsResponse{Workloads: workloads}), nil
}

// GetWorkloadLogs returns a paginated, time-sorted page of log lines merged
// from every pod and container belonging to the given workload.
func (s *LogService) GetWorkloadLogs(ctx context.Context, r *connect.Request[pb.GetWorkloadLogsRequest]) (*connect.Response[pb.GetWorkloadLogsResponse], error) {
	req := r.Msg
	if req.Namespace == "" || req.Kind == "" || req.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("namespace, kind and name are required"))
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

	pods, err := s.workloadPodsForNamespace(req.Namespace, req.Kind, req.Name)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no logs found for %s %s/%s", req.Kind, req.Namespace, req.Name))
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

	// Scan every file but retain only the requested page. Previously every
	// matching line was held in memory before truncating to pageSize.
	h := &logEntryHeap{newest: reversed}
	heap.Init(h)
	var insertIdx int
	var matchingCount int

	for _, pod := range pods {
		chunks, err := podChunks(s.logsRoot, req.Namespace, pod)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list log segments: %v", err))
		}
		for _, chunk := range chunks {
			f, err := os.Open(chunk.path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("open log segment: %v", err))
			}

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if err := ctx.Err(); err != nil {
					f.Close()
					return nil, err
				}
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
				matchingCount++
				if h.Len() > pageSize {
					heap.Pop(h)
				}
			}
			f.Close()
		}
	}

	page := h.entries
	sort.Slice(page, func(i, j int) bool { return logEntryLess(page[i], page[j]) })
	hasMore := matchingCount > len(page)

	resp := &pb.GetWorkloadLogsResponse{Lines: make([]string, len(page))}
	for i, entry := range page {
		resp.Lines[i] = entry.line
	}

	if reversed {
		if hasMore && len(page) > 0 {
			resp.PrevPageToken = encodeBackwardNanosToken(page[0].ts.UnixNano())
		}
		if beforeNanos > 0 && len(page) > 0 {
			resp.NextPageToken = encodeForwardNanosToken(page[len(page)-1].ts.UnixNano())
		}
	} else {
		if hasMore && len(page) > 0 {
			resp.NextPageToken = encodeForwardNanosToken(page[len(page)-1].ts.UnixNano())
		}
		if afterNanos > 0 && len(page) > 0 {
			resp.PrevPageToken = encodeBackwardNanosToken(page[0].ts.UnixNano())
		}
	}

	return connect.NewResponse(resp), nil
}

// StreamWorkloadLogs fans out to a per-pod tail for every currently active
// pod in the workload and multiplexes their output onto a single stream.
//
// Leaving both Kind and Name empty requests a namespace-wide live tail of
// every pod in the namespace, rather than one workload's pods.
func (s *LogService) StreamWorkloadLogs(ctx context.Context, r *connect.Request[pb.StreamWorkloadLogsRequest], stream *connect.ServerStream[pb.StreamWorkloadLogsResponse]) error {
	return s.streamWorkloadLines(ctx, r.Msg, func(line string) error {
		return stream.Send(&pb.StreamWorkloadLogsResponse{Line: line})
	})
}

// streamWorkloadLines implements StreamWorkloadLogs, delivering each line to
// send.
func (s *LogService) streamWorkloadLines(ctx context.Context, req *pb.StreamWorkloadLogsRequest, send func(line string) error) error {
	namespaceWide := req.Kind == "" && req.Name == ""
	if req.Namespace == "" || (!namespaceWide && (req.Kind == "" || req.Name == "")) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("namespace is required, and kind and name must both be set or both be empty"))
	}

	var pods []string
	var err error
	if namespaceWide {
		pods, err = storage.ListPodDirs(s.logsRoot, req.Namespace)
	} else {
		pods, err = s.workloadPodsForNamespace(req.Namespace, req.Kind, req.Name)
	}
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
		if namespaceWide {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("no logs found in namespace %s", req.Namespace))
		}
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("no logs found for %s %s/%s", req.Kind, req.Namespace, req.Name))
	}

	lineCh := make(chan string, 64)

	var wg sync.WaitGroup
	for _, pod := range activePods {
		wg.Add(1)
		go func(podName string) {
			defer wg.Done()
			tailPodToChannel(ctx, s.logsRoot, req.Namespace, podName, lineCh)
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
			if err := send(line); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
