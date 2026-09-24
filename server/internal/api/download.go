package api

// GET /download streams a pod's or a workload's log lines as a single
// text/plain file, honouring the same namespace/pod/kind+name/container/time
// scoping as GetLogs and GetWorkloadLogs.
//
// Query parameters:
//   ns        - namespace (required)
//   pod       - a single pod name (mutually exclusive with kind+name)
//   kind,name - a workload (mutually exclusive with pod); every pod
//               belonging to the workload is merged into one download
//   container - restrict to one container; all containers if omitted
//   from,to   - Unix timestamps (seconds) bounding the export; unbounded on
//               that side if omitted

import (
	"container/heap"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/lsparey/simple-logging/internal/storage"
)

func downloadHandler(svc *LogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		ns := q.Get("ns")
		pod := q.Get("pod")
		kind := q.Get("kind")
		name := q.Get("name")
		container := q.Get("container")

		if ns == "" {
			http.Error(w, "ns is required", http.StatusBadRequest)
			return
		}
		havePod := pod != ""
		haveWorkload := kind != "" && name != ""
		if havePod == haveWorkload {
			http.Error(w, "specify exactly one of pod, or kind and name", http.StatusBadRequest)
			return
		}

		start, err := parseUnixSecondsParam(q.Get("from"))
		if err != nil {
			http.Error(w, "invalid from", http.StatusBadRequest)
			return
		}
		end, err := parseUnixSecondsParam(q.Get("to"))
		if err != nil {
			http.Error(w, "invalid to", http.StatusBadRequest)
			return
		}

		var pods []string
		var identifier string
		if havePod {
			pods = []string{pod}
			identifier = pod
		} else {
			pods, err = svc.workloadPodsForNamespace(ns, kind, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			identifier = fmt.Sprintf("%s-%s", kind, name)
		}

		iterators := make([]*podLineIterator, 0, len(pods))
		defer func() {
			for _, it := range iterators {
				it.close()
			}
		}()
		// One iterator per container, since each container's segments are
		// only in order with themselves; the merge below interleaves them.
		for _, p := range pods {
			containers, err := storage.ListContainers(svc.logsRoot, ns, p)
			if err != nil {
				http.Error(w, fmt.Sprintf("list containers for pod %s: %v", p, err), http.StatusInternalServerError)
				return
			}
			for _, c := range containers {
				if container != "" && c != container {
					continue
				}
				it, err := newPodLineIterator(svc.logsRoot, ns, p, c, start, end)
				if err != nil {
					http.Error(w, fmt.Sprintf("open logs for pod %s: %v", p, err), http.StatusInternalServerError)
					return
				}
				if it != nil {
					iterators = append(iterators, it)
				}
			}
		}
		if len(iterators) == 0 {
			http.Error(w, "no logs found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadFilename(ns, identifier, start, end)))
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		_ = mergeLinesByTime(w, flusher, iterators)
	}
}

func parseUnixSecondsParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0), nil
}

func downloadFilename(ns, identifier string, start, end time.Time) string {
	name := ns + "_" + identifier
	if !start.IsZero() || !end.IsZero() {
		name += "_" + formatFilenameTime(start) + "_" + formatFilenameTime(end)
	}
	return name + ".log"
}

func formatFilenameTime(t time.Time) string {
	if t.IsZero() {
		return "open"
	}
	return t.UTC().Format("20060102T150405Z")
}

// podLineIterator yields one container's log lines in chronological order,
// filtered to a time range. (A pod's containers are each in order only with
// themselves, so callers open one iterator per container and merge them.)
type podLineIterator struct {
	reader     *chunkReader
	start, end time.Time
}

// newPodLineIterator opens an iterator over pod's log chunks. It returns a
// nil iterator (and nil error) when the pod has no matching chunks, so
// callers can skip it without treating that as a failure.
func newPodLineIterator(logsRoot, namespace, pod, container string, start, end time.Time) (*podLineIterator, error) {
	chunks, err := podChunks(logsRoot, namespace, pod)
	if err != nil {
		return nil, err
	}
	if container != "" {
		filtered := chunks[:0]
		for _, c := range chunks {
			if c.container == container {
				filtered = append(filtered, c)
			}
		}
		chunks = filtered
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	reader, err := openChunkReader(chunks, 0, 0)
	if err != nil {
		return nil, err
	}
	return &podLineIterator{reader: reader, start: start, end: end}, nil
}

func (it *podLineIterator) close() {
	if it.reader != nil {
		it.reader.close()
	}
}

// next returns the iterator's next line within the time range, or ok=false
// once every chunk has been exhausted.
func (it *podLineIterator) next() (line string, ts time.Time, ok bool, err error) {
	for {
		line, ok, err = it.reader.readLine()
		if err != nil || !ok {
			return "", time.Time{}, false, err
		}
		ts = parseLineTimestamp(line)
		if !it.start.IsZero() && ts.Before(it.start) {
			continue
		}
		if !it.end.IsZero() && ts.After(it.end) {
			continue
		}
		return line, ts, true, nil
	}
}

// mergeItem is one peeked-ahead candidate line from one iterator, held in
// mergeHeap while its source produces the next one.
type mergeItem struct {
	line   string
	ts     time.Time
	idx    int // global peek order, tiebreaker for equal timestamps
	source int // index into the iterators slice
}

type mergeHeap []mergeItem

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	if h[i].ts.Equal(h[j].ts) {
		return h[i].idx < h[j].idx
	}
	return h[i].ts.Before(h[j].ts)
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *mergeHeap) Push(x any)   { *h = append(*h, x.(mergeItem)) }
func (h *mergeHeap) Pop() any     { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// mergeLinesByTime performs a streaming k-way merge across iterators,
// writing each line (in global chronological order) to w. Memory use is
// O(len(iterators)): at most one peeked line is held per iterator at a time,
// regardless of total output size.
func mergeLinesByTime(w io.Writer, flusher http.Flusher, iterators []*podLineIterator) error {
	h := &mergeHeap{}
	heap.Init(h)
	counter := 0

	refill := func(source int) error {
		line, ts, ok, err := iterators[source].next()
		if err != nil || !ok {
			return err
		}
		heap.Push(h, mergeItem{line: line, ts: ts, idx: counter, source: source})
		counter++
		return nil
	}

	for i := range iterators {
		if err := refill(i); err != nil {
			return err
		}
	}

	linesSinceFlush := 0
	for h.Len() > 0 {
		item := heap.Pop(h).(mergeItem)
		if _, err := io.WriteString(w, item.line+"\n"); err != nil {
			return err
		}
		if err := refill(item.source); err != nil {
			return err
		}
		linesSinceFlush++
		if flusher != nil && linesSinceFlush >= 500 {
			flusher.Flush()
			linesSinceFlush = 0
		}
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}
