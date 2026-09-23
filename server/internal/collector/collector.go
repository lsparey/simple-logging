package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/lsparey/simple-logging/internal/indexes"
	"github.com/lsparey/simple-logging/internal/storage"
)

const (
	// jsonProbeLines is the maximum number of non-empty log lines to sample.
	jsonProbeLines = 10
	// jsonRequiredMatches allows startup chatter before structured logs begin.
	jsonRequiredMatches = 5

	// apiStreamMinBackoff is the initial delay before reopening an API log
	// stream that ended while the pod is still running.
	apiStreamMinBackoff = time.Second
	// apiStreamMaxBackoff caps the reconnect delay so a pod stuck in
	// CrashLoopBackOff costs at most one log request per interval.
	apiStreamMaxBackoff = 30 * time.Second
)

type jsonProbe struct {
	samples int
	matches int
}

func (p *jsonProbe) observe(line string) (decided, isJSON bool) {
	if strings.TrimSpace(line) == "" {
		return false, false
	}

	p.samples++
	if isJSONLine(line) {
		p.matches++
	}

	if p.matches >= jsonRequiredMatches {
		return true, true
	}
	if p.samples >= jsonProbeLines {
		return true, false
	}
	return false, false
}

type podKey struct {
	namespace string
	name      string
}

// containerKey identifies one container's log stream within a pod. A pod's
// non-init containers each get an independent stream; init containers are
// out of scope (they normally run to completion before steady-state logging
// matters).
type containerKey struct {
	namespace string
	pod       string
	container string
}

type activeStream struct {
	cancel context.CancelFunc
}

// Collector manages one log-streaming goroutine per (pod, container) pair —
// every non-init container in a pod is streamed independently. It is safe
// for concurrent use from the PodWatcher callbacks.
type Collector struct {
	cs       kubernetes.Interface
	logsRoot string
	// nodeLogsRoot is the host path where the CRI writes pod log files
	// (typically /var/log/pods). When non-empty the collector tails files
	// directly, bypassing the Kubernetes log API and eliminating containerd
	// streaming overhead.
	nodeLogsRoot string
	// nodeName is the node this collector runs on. When set alongside
	// nodeLogsRoot only pods scheduled on this node are tailed from the
	// filesystem; pods on other nodes are streamed via the Kubernetes log API
	// (hybrid mode). Empty means "tail every pod" for backwards compatibility.
	nodeName string
	log      *zap.Logger

	// apiMinBackoff/apiMaxBackoff bound the reconnect delay for API streams.
	// They are fields so tests can shorten them.
	apiMinBackoff time.Duration
	apiMaxBackoff time.Duration

	mu      sync.Mutex
	wg      sync.WaitGroup
	streams map[containerKey]*activeStream
	// activeContainerCount is a ref count of running container streams per
	// pod, kept in step with streams so IsActive stays O(1) instead of
	// scanning every stream in the cluster on each call.
	activeContainerCount map[podKey]int

	// deploymentPods maps "namespace/deployment" -> set of pod names.
	deploymentPods map[string]map[string]struct{}
	// podDeployment maps "namespace/pod" -> deployment name.
	podDeployment map[string]string

	// jsonLogging tracks which pods have been determined to use JSON log formatting.
	jsonLogging map[podKey]bool

	indexes *indexes.Manager

	// droppedLines counts log lines dropped because a write to storage
	// failed. See writeLogLine.
	droppedLines atomic.Int64
}

// Option configures optional Collector behaviour.
type Option func(*Collector)

// WithNodeName tells the collector which node it is running on. Combined with
// a non-empty nodeLogsRoot this enables hybrid mode: pods on this node are
// tailed from the host filesystem and pods on other nodes are streamed via the
// Kubernetes log API. An empty name leaves the behaviour unchanged.
func WithNodeName(name string) Option {
	return func(c *Collector) { c.nodeName = name }
}

// New creates a Collector that writes pod logs to files under logsRoot.
// If nodeLogsRoot is non-empty (e.g. "/var/log/pods" mounted as a hostPath),
// the collector tails log files directly from the node filesystem instead of
// using the Kubernetes log-streaming API.
func New(cs kubernetes.Interface, logsRoot, nodeLogsRoot string, log *zap.Logger, opts ...Option) *Collector {
	return NewWithIndexes(cs, logsRoot, nodeLogsRoot, log, indexes.NewManager(logsRoot), opts...)
}

// NewWithIndexes creates a Collector with a shared index manager.
func NewWithIndexes(cs kubernetes.Interface, logsRoot, nodeLogsRoot string, log *zap.Logger, indexManager *indexes.Manager, opts ...Option) *Collector {
	c := &Collector{
		cs:                   cs,
		logsRoot:             logsRoot,
		nodeLogsRoot:         nodeLogsRoot,
		log:                  log,
		apiMinBackoff:        apiStreamMinBackoff,
		apiMaxBackoff:        apiStreamMaxBackoff,
		streams:              make(map[containerKey]*activeStream),
		activeContainerCount: make(map[podKey]int),
		deploymentPods:       make(map[string]map[string]struct{}),
		podDeployment:        make(map[string]string),
		jsonLogging:          make(map[podKey]bool),
		indexes:              indexManager,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// usesFileTail reports whether pod's logs are read from the node filesystem
// rather than the Kubernetes log API. File tailing requires nodeLogsRoot and,
// when this collector knows its own node, that the pod is scheduled on it.
func (c *Collector) usesFileTail(pod *corev1.Pod) bool {
	if c.nodeLogsRoot == "" {
		return false
	}
	if c.nodeName == "" {
		return true
	}
	return pod.Spec.NodeName == c.nodeName
}

// OnAdd is called by the PodWatcher when a pod starts or transitions to
// Running. It starts one stream per non-init container. If a stream is
// already running for a given container (restart scenario — the pod was
// recreated under the same name), that container's old stream is cancelled
// before its new one begins; other containers are unaffected.
func (c *Collector) OnAdd(pod *corev1.Pod) {
	pk := podKey{namespace: pod.Namespace, name: pod.Name}
	containers := collectableContainers(pod)

	type startItem struct {
		container string
		ctx       context.Context
		isRestart bool
	}

	c.mu.Lock()
	c.trackDeployment(pod)
	items := make([]startItem, 0, len(containers))
	for _, containerName := range containers {
		ck := containerKey{namespace: pod.Namespace, pod: pod.Name, container: containerName}
		isRestart := false
		if existing, ok := c.streams[ck]; ok {
			existing.cancel()
			isRestart = true
		} else {
			c.activeContainerCount[pk]++
		}
		ctx, cancel := context.WithCancel(context.Background())
		c.streams[ck] = &activeStream{cancel: cancel}
		items = append(items, startItem{container: containerName, ctx: ctx, isRestart: isRestart})
	}
	c.mu.Unlock()

	for _, item := range items {
		// Immediately probe the stored log file so the JSON-logging flag is
		// available before the live stream delivers its first batch of
		// lines. Only the default container feeds the pod-level flag (see
		// isDefaultContainer in runFileTail/streamAPIOnce), so there is
		// nothing useful to probe for the others.
		if item.container == defaultContainer(pod) {
			c.detectJsonFromFile(pod.Namespace, pod.Name, item.container)
		}

		c.wg.Add(1)
		go func(item startItem) {
			defer c.wg.Done()
			c.runStream(item.ctx, pod, item.container, item.isRestart)
		}(item)
	}
}

// collectableContainers returns the names of a pod's non-init containers.
// pod.Spec.Containers already excludes init containers (a separate slice),
// so they are out of scope with no extra filtering.
func collectableContainers(pod *corev1.Pod) []string {
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		names = append(names, container.Name)
	}
	return names
}

// detectJsonFromFile scans the first jsonProbeLines non-empty lines of the
// container's oldest stored log segment and marks the pod as JSON-logging
// once enough valid JSON objects are found. This provides an instant result
// on server restarts before the live stream has had a chance to deliver
// enough lines.
func (c *Collector) detectJsonFromFile(namespace, podName, container string) {
	segments, err := storage.ListSegments(c.logsRoot, namespace, podName, container)
	if err != nil || len(segments) == 0 {
		return // no stored segments yet — nothing to do
	}
	path := filepath.Join(storage.ContainerDir(c.logsRoot, namespace, podName, container), segments[0]+".log")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Lines in the file have the prefix: "TIMESTAMP [ns/pod/container] <rawLog>"
	// Strip up to and including the first "] " to recover the original log line.
	var probe jsonProbe
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && probe.samples < jsonProbeLines {
		line := scanner.Text()
		idx := strings.Index(line, "] ")
		if idx < 0 {
			continue
		}
		payload := line[idx+2:]
		if decided, isJSON := probe.observe(payload); decided {
			c.setJsonLogging(namespace, podName, isJSON)
			return
		}
	}
}

// Close cancels all active streams and waits for their goroutines to finish.
// It should be called when the collector is no longer needed (e.g. on shutdown
// or at the end of a test).
func (c *Collector) Close() {
	c.mu.Lock()
	for _, s := range c.streams {
		s.cancel()
	}
	c.mu.Unlock()
	c.wg.Wait()
}

// OnDelete is called by the PodWatcher when a pod is deleted.
// It cancels every running container stream for the pod and prunes it from
// all tracking maps so they don't grow unbounded over the collector's lifetime.
func (c *Collector) OnDelete(pod *corev1.Pod) {
	pk := podKey{namespace: pod.Namespace, name: pod.Name}

	c.mu.Lock()
	stopped := 0
	for _, containerName := range collectableContainers(pod) {
		ck := containerKey{namespace: pod.Namespace, pod: pod.Name, container: containerName}
		if existing, ok := c.streams[ck]; ok {
			existing.cancel()
			delete(c.streams, ck)
			stopped++
		}
	}
	if stopped > 0 {
		if c.activeContainerCount[pk] -= stopped; c.activeContainerCount[pk] <= 0 {
			delete(c.activeContainerCount, pk)
		}
	}
	delete(c.jsonLogging, pk)

	podMapKey := pod.Namespace + "/" + pod.Name
	if depName, tracked := c.podDeployment[podMapKey]; tracked {
		delete(c.podDeployment, podMapKey)
		depKey := pod.Namespace + "/" + depName
		if pods, ok := c.deploymentPods[depKey]; ok {
			delete(pods, pod.Name)
			if len(pods) == 0 {
				delete(c.deploymentPods, depKey)
			}
		}
	}
	c.mu.Unlock()

	if stopped > 0 {
		c.log.Info("stopped log streams",
			zap.String("namespace", pod.Namespace),
			zap.String("pod", pod.Name),
			zap.Int("containers", stopped),
		)
	}
}

// IsActive reports whether a pod currently has at least one container being streamed.
func (c *Collector) IsActive(namespace, pod string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeContainerCount[podKey{namespace: namespace, name: pod}] > 0
}

// IsJsonLogging reports whether the given pod's log output has been detected
// as JSON-formatted.
func (c *Collector) IsJsonLogging(namespace, pod string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jsonLogging[podKey{namespace: namespace, name: pod}]
}

// setJsonLogging records the JSON-logging status for a pod. It is called from
// the streaming goroutine once enough sample lines have been observed.
func (c *Collector) setJsonLogging(namespace, pod string, isJson bool) {
	c.mu.Lock()
	c.jsonLogging[podKey{namespace: namespace, name: pod}] = isJson
	c.mu.Unlock()
}

// GetDeploymentName returns the deployment name for a pod if it is known.
func (c *Collector) GetDeploymentName(namespace, podName string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.podDeployment[namespace+"/"+podName]
	return d, ok
}

// ListKnownDeployments returns the names of all deployments the collector has
// observed in the given namespace.
func (c *Collector) ListKnownDeployments(namespace string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := namespace + "/"
	var result []string
	for key := range c.deploymentPods {
		if strings.HasPrefix(key, prefix) {
			result = append(result, strings.TrimPrefix(key, prefix))
		}
	}
	return result
}

// trackDeployment updates the deployment<->pod mappings for a pod. mu must be
// held by the caller.
func (c *Collector) trackDeployment(pod *corev1.Pod) {
	rsHash := pod.Labels["pod-template-hash"]
	if rsHash == "" {
		return
	}
	// pod.Name == <deployment>-<rsHash>-<podHash>
	// Strip the last segment (pod-specific hash) then check the remainder
	// ends with "-<rsHash>" to derive the deployment name.
	lastDash := strings.LastIndex(pod.Name, "-")
	if lastDash < 0 {
		return
	}
	nameWithoutPodHash := pod.Name[:lastDash]
	suffix := "-" + rsHash
	if !strings.HasSuffix(nameWithoutPodHash, suffix) {
		return
	}
	deploymentName := nameWithoutPodHash[:len(nameWithoutPodHash)-len(suffix)]
	if deploymentName == "" {
		return
	}

	depKey := pod.Namespace + "/" + deploymentName
	podMapKey := pod.Namespace + "/" + pod.Name

	if c.deploymentPods[depKey] == nil {
		c.deploymentPods[depKey] = make(map[string]struct{})
	}
	c.deploymentPods[depKey][pod.Name] = struct{}{}
	c.podDeployment[podMapKey] = deploymentName
}

// runStream is the entry point for each per-container goroutine. It creates
// the storage writer, writes a restart separator if needed, then dispatches
// to either runFileTail (node-local file) or runAPIStream (Kubernetes log API).
func (c *Collector) runStream(ctx context.Context, pod *corev1.Pod, containerName string, isRestart bool) {
	fileTail := c.usesFileTail(pod)
	source := "api"
	if fileTail {
		source = "file"
	}
	log := c.log.With(
		zap.String("namespace", pod.Namespace),
		zap.String("pod", pod.Name),
		zap.String("container", containerName),
		zap.String("node", pod.Spec.NodeName),
		zap.String("source", source),
	)

	writer, err := storage.NewSegmentWriter(c.logsRoot, pod.Namespace, pod.Name, containerName)
	if err != nil {
		log.Error("failed to open log file", zap.Error(err))
		return
	}
	defer func() {
		if cerr := writer.Close(); cerr != nil {
			log.Warn("failed to close log file", zap.Error(cerr))
		}
	}()

	// Write a separator line when a pod restarts so log consumers can identify
	// the boundary between distinct container lifecycles.
	if isRestart && writer.HasContent() {
		now := time.Now().UTC()
		sep := fmt.Sprintf("--- pod restarted at %s ---", now.Format(time.RFC3339))
		if ok := writer.Write(now, sep); !ok {
			log.Warn("failed to write restart separator, dropped")
		}
	}

	if fileTail {
		c.runFileTail(ctx, pod, containerName, writer, log)
	} else {
		c.runAPIStream(ctx, pod, containerName, writer, log)
	}
}

// runFileTail tails the pod's log file directly from the node filesystem,
// completely bypassing the Kubernetes log API. This eliminates the persistent
// HTTP streaming connections that cause elevated CPU in containerd/kubelet.
//
// Kubernetes names log files by the container's restart count:
//
//	/var/log/pods/<ns>_<name>_<uid>/<container>/<restartCount>.log
//
// The current file is always <restartCount>.log. When a container restarts,
// <restartCount+1>.log is created and we switch to it automatically.
//
// inotify (via fsnotify) is used to wake the goroutine only when data is
// written. When inotify is unavailable the implementation falls back to
// polling at 500ms intervals.
//
// Each line in the file is either CRI format or Docker JSON:
//
//	CRI:    <RFC3339Nano> <stream> <flag> <content>
//	Docker: {"log":"<content>\n","stream":"stdout","time":"..."}
func (c *Collector) runFileTail(ctx context.Context, pod *corev1.Pod, containerName string, writer *storage.SegmentWriter, log *zap.Logger) {
	// PodInfo.json_logging is a single pod-level flag, so only the default
	// container's probe feeds it — otherwise whichever container's probe
	// decided most recently would arbitrarily win.
	isDefaultContainer := containerName == defaultContainer(pod)

	containerDir := filepath.Join(c.nodeLogsRoot,
		fmt.Sprintf("%s_%s_%s", pod.Namespace, pod.Name, string(pod.UID)),
		containerName)

	// The file for the currently-running container is named after its restart
	// count. A container on its third run writes to 2.log, not 0.log.
	restartCount := containerRestartCount(pod, containerName)

	// Only skip pre-existing history on the very first file we open. After a
	// container restart we always read from the start of the new file.
	seekToEnd := writer.HasContent()

	var partial strings.Builder
	var partialTS time.Time
	var probe jsonProbe
	jsonDecided := false

	// Create a single fsnotify watcher shared across all files for this pod.
	// If fsnotify is unavailable (e.g. inotify limit hit) we fall back to polling.
	watcher, watchErr := fsnotify.NewWatcher()
	if watchErr != nil {
		log.Warn("fsnotify unavailable, falling back to polling",
			zap.Error(watchErr))
	} else {
		defer watcher.Close()
		// Watch the container directory so we detect creation of the next
		// log file (container restart) via a Create event on the parent dir.
		if werr := watcher.Add(containerDir); werr != nil {
			log.Warn("fsnotify: failed to watch container dir, falling back to polling",
				zap.String("dir", containerDir), zap.Error(werr))
			watcher.Close()
			watcher = nil
		}
	}

	// waitForWrite blocks until fsnotify signals a write/create on the watched
	// directory, or the 500ms polling fallback fires, or ctx is cancelled.
	// Returns false if the context is done.
	waitForWrite := func() bool {
		if watcher != nil {
			select {
			case <-ctx.Done():
				return false
			case <-watcher.Events:
				// Drain any queued events so we don't spin on a backlog.
				for len(watcher.Events) > 0 {
					<-watcher.Events
				}
				return true
			case <-watcher.Errors:
				return true // treat watcher errors as a wake-up; next read will clarify
			}
		}
		// Polling fallback.
		select {
		case <-ctx.Done():
			return false
		case <-time.After(500 * time.Millisecond):
			return true
		}
	}

	for {
		logPath := filepath.Join(containerDir, fmt.Sprintf("%d.log", restartCount))
		nextLogPath := filepath.Join(containerDir, fmt.Sprintf("%d.log", restartCount+1))

		f, err := waitForLogFile(ctx, logPath, watcher)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("gave up waiting for log file", zap.String("path", logPath), zap.Error(err))
			}
			return
		}

		// Watch the log file itself for Write events (in addition to the dir).
		if watcher != nil {
			if werr := watcher.Add(logPath); werr != nil {
				log.Warn("fsnotify: failed to watch log file",
					zap.String("path", logPath), zap.Error(werr))
			}
		}

		if seekToEnd {
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				log.Warn("failed to seek to end of log file", zap.Error(err))
			}
			seekToEnd = false
		}

		log.Info("log file tail started", zap.String("path", logPath))

		reader := bufio.NewReader(f)
		restarted := false

		for {
			rawLine, err := reader.ReadString('\n')
			if err == io.EOF {
				// Check whether the next log file appeared (container restarted).
				if _, serr := os.Stat(nextLogPath); serr == nil {
					log.Info("container restarted, switching log file",
						zap.String("next", nextLogPath))
					if watcher != nil {
						_ = watcher.Remove(logPath)
					}
					_ = f.Close()
					restartCount++
					now := time.Now().UTC()
					sep := fmt.Sprintf("--- container restarted at %s ---", now.Format(time.RFC3339))
					if ok := writer.Write(now, sep); !ok {
						log.Warn("failed to write restart separator, dropped")
					}
					partial.Reset()
					restarted = true
					break
				}
				if !waitForWrite() {
					_ = f.Close()
					return
				}
				continue
			}
			if err != nil {
				if ctx.Err() == nil {
					log.Error("log file read error", zap.String("path", logPath), zap.Error(err))
				}
				_ = f.Close()
				return
			}

			// Parse CRI/Docker-JSON format and reassemble partial lines.
			lineTS, content, isPartial := parseCRILogLine(strings.TrimRight(rawLine, "\n"))
			if isPartial {
				if partial.Len() == 0 {
					partialTS = lineTS
				}
				partial.WriteString(content)
				continue
			}
			logContent := content
			if partial.Len() > 0 {
				partial.WriteString(content)
				logContent = partial.String()
				partial.Reset()
				lineTS = partialTS
			}

			// Probe the first jsonProbeLines non-empty lines to detect JSON logging.
			if !jsonDecided {
				if decided, isJSON := probe.observe(logContent); decided {
					jsonDecided = true
					if isDefaultContainer {
						c.setJsonLogging(pod.Namespace, pod.Name, isJSON)
					}
				}
			}

			line := fmt.Sprintf("%s [%s/%s/%s] %s",
				lineTS.Format(time.RFC3339Nano),
				pod.Namespace, pod.Name, containerName,
				logContent,
			)
			c.writeLogLine(writer, pod.Namespace, pod.Name, containerName, lineTS, line)
		}

		if !restarted {
			return
		}
	}
}

// containerRestartCount returns the restart count for the named container from
// the pod status, or 0 if the container is not yet in the status list.
func containerRestartCount(pod *corev1.Pod, containerName string) int32 {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			return cs.RestartCount
		}
	}
	return 0
}

// waitForLogFile waits up to 60 seconds for the log file at path to appear,
// then opens and returns it. If watcher is non-nil it uses fsnotify events on
// the parent directory to wake up; otherwise it falls back to 500ms polling.
func waitForLogFile(ctx context.Context, path string, watcher *fsnotify.Watcher) (*os.File, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for %s to appear", path)
		}
		if watcher != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-watcher.Events:
				for len(watcher.Events) > 0 {
					<-watcher.Events
				}
			case <-watcher.Errors:
			}
		} else {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
}

// parseCRILogLine parses a single raw line from a pod log file on the host.
// It handles two formats:
//
// CRI (containerd):
//
//	<RFC3339Nano> <stream> <flag> <content>
//
// Docker JSON (Docker Desktop / dockerd):
//
//	{"log":"<content>\n","stream":"stdout","time":"<RFC3339Nano>"}
//
// Returns the line's source timestamp, the log content, and whether it is a
// partial line. For Docker JSON, partial lines are those whose "log" value
// does not end with a newline. Falls back to the current wall-clock time and
// the raw line unchanged if the format or its timestamp is not recognised.
func parseCRILogLine(line string) (ts time.Time, content string, isPartial bool) {
	// Docker JSON format — line starts with '{'.
	if len(line) > 0 && line[0] == '{' {
		var entry struct {
			Log  string `json:"log"`
			Time string `json:"time"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			partial := !strings.HasSuffix(entry.Log, "\n")
			ts, err := time.Parse(time.RFC3339Nano, entry.Time)
			if err != nil {
				ts = time.Now().UTC()
			}
			return ts, strings.TrimSuffix(entry.Log, "\n"), partial
		}
	}

	// CRI format: <timestamp> <stream> <flag> <content>
	i := strings.Index(line, " ")
	if i < 0 {
		return time.Now().UTC(), line, false
	}
	tsToken := line[:i]
	rest := line[i+1:]
	// Skip past stream token (stdout/stderr).
	i = strings.Index(rest, " ")
	if i < 0 {
		return time.Now().UTC(), line, false
	}
	rest = rest[i+1:]
	// Read flag token.
	i = strings.Index(rest, " ")
	if i < 0 {
		return time.Now().UTC(), line, false
	}
	flag := rest[:i]
	ts, err := time.Parse(time.RFC3339Nano, tsToken)
	if err != nil {
		ts = time.Now().UTC()
	}
	return ts, rest[i+1:], flag == "P"
}

// runAPIStream streams pod logs via the Kubernetes log API. Used for pods
// whose log files are not reachable on the local filesystem: every pod in
// "api" mode, or pods scheduled on other nodes in hybrid mode.
//
// A follow stream ends whenever the container exits, kubelet restarts, or the
// connection through kube-apiserver is dropped. Remote nodes make the last two
// far more likely, so the stream is reopened with exponential backoff until
// the pod reaches a terminal phase, disappears, or ctx is cancelled. Each
// reconnect resumes from the timestamp of the last line received so history
// is not replayed (at most one second may be duplicated, because the API only
// accepts whole-second SinceTime values).
func (c *Collector) runAPIStream(ctx context.Context, pod *corev1.Pod, containerName string, writer *storage.SegmentWriter, log *zap.Logger) {
	// If we already have a log file for this pod, skip replaying the full
	// historical log. The stored file already contains the history.
	var since *metav1.Time
	if writer.HasContent() {
		t := metav1.NewTime(time.Now().Add(-time.Second))
		since = &t
	}

	var probe jsonProbe
	jsonDecided := false
	backoff := c.apiMinBackoff

	for {
		lastLine, err := c.streamAPIOnce(ctx, pod, containerName, writer, log, since, &probe, &jsonDecided)
		if ctx.Err() != nil {
			return // cancelled — not an error
		}
		if !lastLine.IsZero() {
			t := metav1.NewTime(lastLine)
			since = &t
			backoff = c.apiMinBackoff // progress was made, start the backoff over
		}

		if err == nil {
			// Clean EOF: the container exited. Stop if the pod is gone for
			// good; otherwise kubelet may be about to restart the container.
			if c.podFinished(ctx, pod) {
				log.Info("log stream ended, pod finished")
				return
			}
			if ctx.Err() != nil {
				return // cancelled during the lookup
			}
		} else {
			log.Warn("log stream interrupted", zap.Error(err))
		}

		log.Info("reconnecting log stream", zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, c.apiMaxBackoff)
	}
}

// streamAPIOnce opens a single follow stream and copies lines to writer until
// it ends. It returns the wall-clock time of the last line written (zero if
// none) and a non-nil error if the stream failed to open or ended abnormally.
func (c *Collector) streamAPIOnce(ctx context.Context, pod *corev1.Pod, containerName string, writer *storage.SegmentWriter, log *zap.Logger, since *metav1.Time, probe *jsonProbe, jsonDecided *bool) (time.Time, error) {
	// PodInfo.json_logging is a single pod-level flag, so only the default
	// container's probe feeds it (see runFileTail).
	isDefaultContainer := containerName == defaultContainer(pod)

	logOpts := &corev1.PodLogOptions{
		Container:  containerName,
		Follow:     true,
		SinceTime:  since,
		Timestamps: true,
	}
	req := c.cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("open log stream: %w", err)
	}
	defer stream.Close()

	log.Info("log stream started")

	var lastLine time.Time
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		rawLine := scanner.Text()

		// Timestamps: true prefixes each line with "<RFC3339Nano> ". Split it
		// off so the source timestamp is used instead of receipt wall-clock
		// time, falling back to now if a line is ever missing the prefix.
		lineTS := time.Now().UTC()
		content := rawLine
		if idx := strings.IndexByte(rawLine, ' '); idx > 0 {
			if t, err := time.Parse(time.RFC3339Nano, rawLine[:idx]); err == nil {
				lineTS = t
				content = rawLine[idx+1:]
			}
		}

		// Probe the first jsonProbeLines non-empty lines to detect JSON logging.
		if !*jsonDecided {
			if decided, isJSON := probe.observe(content); decided {
				*jsonDecided = true
				if isDefaultContainer {
					c.setJsonLogging(pod.Namespace, pod.Name, isJSON)
				}
			}
		}

		line := fmt.Sprintf("%s [%s/%s/%s] %s",
			lineTS.Format(time.RFC3339Nano),
			pod.Namespace, pod.Name, containerName,
			content,
		)
		c.writeLogLine(writer, pod.Namespace, pod.Name, containerName, lineTS, line)
		lastLine = lineTS
	}

	if serr := scanner.Err(); serr != nil {
		return lastLine, serr
	}
	log.Debug("log stream ended")
	return lastLine, nil
}

// podFinished reports whether the pod has reached a terminal phase or no
// longer exists, in which case its log stream should not be reopened. Any
// other lookup error is treated as "still running" so a transient API blip
// does not silently abandon a live pod.
func (c *Collector) podFinished(ctx context.Context, pod *corev1.Pod) bool {
	current, err := c.cs.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return apierrors.IsNotFound(err)
	}
	if current.UID != pod.UID {
		return true // a different pod now owns this name; its own OnAdd will stream it
	}
	switch current.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return true
	}
	return false
}

// defaultContainer returns the name of the first (default) container in the pod.
func defaultContainer(pod *corev1.Pod) string {
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Name
	}
	return ""
}

// isJSONLine returns true when line is a valid JSON object (starts with '{').
func isJSONLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	return json.Valid([]byte(trimmed))
}

// writeLogLine writes line to storage. Write failures are transient (see
// storage.SegmentWriter): the line is dropped and counted rather than ending
// the caller's stream, so a temporarily full or read-only volume doesn't stop
// collection of pods whose writers aren't affected, or block reading from the
// source once the volume recovers.
func (c *Collector) writeLogLine(writer *storage.SegmentWriter, namespace, pod, container string, lineTS time.Time, line string) {
	segment, offset, length, ok := writer.WriteWithLocation(lineTS, line)
	if !ok {
		c.droppedLines.Add(1)
		return
	}
	if c.indexes != nil {
		c.indexes.ObserveLineAt(namespace, pod, container, segment, offset, length, line)
	}
}

// DroppedLines returns the number of log lines dropped so far because a
// write to storage failed (e.g. a full or temporarily read-only volume).
func (c *Collector) DroppedLines() int64 {
	return c.droppedLines.Load()
}
