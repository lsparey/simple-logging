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
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/lsparey/simple-logging/internal/indexes"
	"github.com/lsparey/simple-logging/internal/storage"
)

// jsonProbeLines is the number of non-empty log lines to sample before
// deciding whether a pod's output is JSON-formatted.
const jsonProbeLines = 5

type podKey struct {
	namespace string
	name      string
}

type activeStream struct {
	cancel context.CancelFunc
}

// Collector manages one log-streaming goroutine per pod. It is safe for
// concurrent use from the PodWatcher callbacks.
type Collector struct {
	cs       kubernetes.Interface
	logsRoot string
	// nodeLogsRoot is the host path where the CRI writes pod log files
	// (typically /var/log/pods). When non-empty the collector tails files
	// directly, bypassing the Kubernetes log API and eliminating containerd
	// streaming overhead.
	nodeLogsRoot string
	log          *zap.Logger

	mu      sync.Mutex
	wg      sync.WaitGroup
	streams map[podKey]*activeStream

	// deploymentPods maps "namespace/deployment" -> set of pod names.
	deploymentPods map[string]map[string]struct{}
	// podDeployment maps "namespace/pod" -> deployment name.
	podDeployment map[string]string

	// jsonLogging tracks which pods have been determined to use JSON log formatting.
	jsonLogging map[podKey]bool

	indexes *indexes.Manager
}

// New creates a Collector that writes pod logs to files under logsRoot.
// If nodeLogsRoot is non-empty (e.g. "/var/log/pods" mounted as a hostPath),
// the collector tails log files directly from the node filesystem instead of
// using the Kubernetes log-streaming API.
func New(cs kubernetes.Interface, logsRoot, nodeLogsRoot string, log *zap.Logger) *Collector {
	return NewWithIndexes(cs, logsRoot, nodeLogsRoot, log, indexes.NewManager(logsRoot))
}

// NewWithIndexes creates a Collector with a shared index manager.
func NewWithIndexes(cs kubernetes.Interface, logsRoot, nodeLogsRoot string, log *zap.Logger, indexManager *indexes.Manager) *Collector {
	return &Collector{
		cs:             cs,
		logsRoot:       logsRoot,
		nodeLogsRoot:   nodeLogsRoot,
		log:            log,
		streams:        make(map[podKey]*activeStream),
		deploymentPods: make(map[string]map[string]struct{}),
		podDeployment:  make(map[string]string),
		jsonLogging:    make(map[podKey]bool),
		indexes:        indexManager,
	}
}

// OnAdd is called by the PodWatcher when a pod starts or transitions to Running.
// If a stream is already running for that pod (restart scenario), the old stream
// is cancelled before a new one begins.
func (c *Collector) OnAdd(pod *corev1.Pod) {
	key := podKey{namespace: pod.Namespace, name: pod.Name}

	c.mu.Lock()
	isRestart := false
	if existing, ok := c.streams[key]; ok {
		existing.cancel()
		isRestart = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.streams[key] = &activeStream{cancel: cancel}
	c.trackDeployment(pod)
	c.mu.Unlock()

	// Immediately probe the stored log file so the JSON-logging flag is available
	// before the live stream delivers its first batch of lines.
	c.detectJsonFromFile(pod.Namespace, pod.Name)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runStream(ctx, pod, isRestart)
	}()
}

// detectJsonFromFile scans the first jsonProbeLines non-empty lines of the pod's
// stored log file and immediately marks the pod as JSON-logging if they are all
// valid JSON objects. This provides an instant result on server restarts before
// the live stream has had a chance to deliver enough lines.
func (c *Collector) detectJsonFromFile(namespace, podName string) {
	path := filepath.Join(c.logsRoot, namespace, podName+".log")
	f, err := os.Open(path)
	if err != nil {
		return // file doesn't exist yet — nothing to do
	}
	defer f.Close()

	// Lines in the file have the prefix: "TIMESTAMP [ns/pod/container] <rawLog>"
	// Strip up to and including the first "] " to recover the original log line.
	samples, matches := 0, 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && samples < jsonProbeLines {
		line := scanner.Text()
		idx := strings.Index(line, "] ")
		if idx < 0 {
			continue
		}
		payload := line[idx+2:]
		if strings.TrimSpace(payload) == "" {
			continue
		}
		samples++
		if isJSONLine(payload) {
			matches++
		}
	}
	if samples >= jsonProbeLines && matches == samples {
		c.setJsonLogging(namespace, podName, true)
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
// It cancels the running stream goroutine if one exists.
func (c *Collector) OnDelete(pod *corev1.Pod) {
	key := podKey{namespace: pod.Namespace, name: pod.Name}

	c.mu.Lock()
	existing, ok := c.streams[key]
	if ok {
		existing.cancel()
		delete(c.streams, key)
	}
	c.mu.Unlock()

	if ok {
		c.log.Info("stopped log stream",
			zap.String("namespace", pod.Namespace),
			zap.String("pod", pod.Name),
		)
	}
}

// IsActive reports whether a pod is currently being streamed.
func (c *Collector) IsActive(namespace, pod string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.streams[podKey{namespace: namespace, name: pod}]
	return ok
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

// runStream is the entry point for the per-pod goroutine. It creates the
// storage writer, writes a restart separator if needed, then dispatches to
// either runFileTail (node-local file) or runAPIStream (Kubernetes log API).
func (c *Collector) runStream(ctx context.Context, pod *corev1.Pod, isRestart bool) {
	containerName := defaultContainer(pod)
	log := c.log.With(
		zap.String("namespace", pod.Namespace),
		zap.String("pod", pod.Name),
		zap.String("container", containerName),
	)

	writer, err := storage.NewFileWriter(c.logsRoot, pod.Namespace, pod.Name)
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
		sep := fmt.Sprintf("--- pod restarted at %s ---", time.Now().UTC().Format(time.RFC3339))
		if werr := writer.Write(sep); werr != nil {
			log.Warn("failed to write restart separator", zap.Error(werr))
		}
	}

	if c.nodeLogsRoot != "" {
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
func (c *Collector) runFileTail(ctx context.Context, pod *corev1.Pod, containerName string, writer *storage.FileWriter, log *zap.Logger) {
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
	jsonSamples, jsonMatches := 0, 0
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
					sep := fmt.Sprintf("--- container restarted at %s ---",
						time.Now().UTC().Format(time.RFC3339))
					if werr := writer.Write(sep); werr != nil {
						log.Warn("failed to write restart separator", zap.Error(werr))
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
			content, isPartial := parseCRILogLine(strings.TrimRight(rawLine, "\n"))
			if isPartial {
				partial.WriteString(content)
				continue
			}
			logContent := content
			if partial.Len() > 0 {
				partial.WriteString(content)
				logContent = partial.String()
				partial.Reset()
			}

			// Probe the first jsonProbeLines non-empty lines to detect JSON logging.
			if !jsonDecided && strings.TrimSpace(logContent) != "" {
				jsonSamples++
				if isJSONLine(logContent) {
					jsonMatches++
				}
				if jsonSamples >= jsonProbeLines {
					jsonDecided = true
					c.setJsonLogging(pod.Namespace, pod.Name, jsonMatches == jsonSamples)
				}
			}

			line := fmt.Sprintf("%s [%s/%s/%s] %s",
				time.Now().UTC().Format(time.RFC3339),
				pod.Namespace, pod.Name, containerName,
				logContent,
			)
			if werr := c.writeLogLine(writer, pod.Namespace, pod.Name, line); werr != nil {
				log.Error("failed to write log line", zap.Error(werr))
				_ = f.Close()
				return
			}
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
// Returns the log content and whether it is a partial line. For Docker JSON,
// partial lines are those whose "log" value does not end with a newline.
// Falls back to returning the raw line unchanged if the format is not recognised.
func parseCRILogLine(line string) (content string, isPartial bool) {
	// Docker JSON format — line starts with '{'.
	if len(line) > 0 && line[0] == '{' {
		var entry struct {
			Log string `json:"log"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			partial := !strings.HasSuffix(entry.Log, "\n")
			return strings.TrimSuffix(entry.Log, "\n"), partial
		}
	}

	// CRI format: <timestamp> <stream> <flag> <content>
	// Skip past timestamp token.
	i := strings.Index(line, " ")
	if i < 0 {
		return line, false
	}
	rest := line[i+1:]
	// Skip past stream token (stdout/stderr).
	i = strings.Index(rest, " ")
	if i < 0 {
		return line, false
	}
	rest = rest[i+1:]
	// Read flag token.
	i = strings.Index(rest, " ")
	if i < 0 {
		return line, false
	}
	flag := rest[:i]
	return rest[i+1:], flag == "P"
}

// runAPIStream streams pod logs via the Kubernetes log API. Used when
// nodeLogsRoot is not configured (e.g. multi-node clusters or non-hostPath setups).
func (c *Collector) runAPIStream(ctx context.Context, pod *corev1.Pod, containerName string, writer *storage.FileWriter, log *zap.Logger) {
	logOpts := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    true,
	}
	// If we already have a log file for this pod, skip replaying the full
	// historical log. The stored file already contains the history.
	if writer.HasContent() {
		sinceSeconds := int64(1)
		logOpts.SinceSeconds = &sinceSeconds
	}
	req := c.cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)

	stream, err := req.Stream(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return // context was cancelled — not an error
		}
		log.Error("failed to open log stream", zap.Error(err))
		return
	}
	defer stream.Close()

	log.Info("log stream started")

	scanner := bufio.NewScanner(stream)
	jsonSamples := 0
	jsonMatches := 0
	jsonDecided := false

	for scanner.Scan() {
		rawLine := scanner.Text()

		// Probe the first jsonProbeLines non-empty lines to detect JSON logging.
		if !jsonDecided && strings.TrimSpace(rawLine) != "" {
			jsonSamples++
			if isJSONLine(rawLine) {
				jsonMatches++
			}
			if jsonSamples >= jsonProbeLines {
				jsonDecided = true
				c.setJsonLogging(pod.Namespace, pod.Name, jsonMatches == jsonSamples)
			}
		}

		line := fmt.Sprintf("%s [%s/%s/%s] %s",
			time.Now().UTC().Format(time.RFC3339),
			pod.Namespace, pod.Name, containerName,
			rawLine,
		)
		if werr := c.writeLogLine(writer, pod.Namespace, pod.Name, line); werr != nil {
			log.Error("failed to write log line", zap.Error(werr))
			return
		}
	}

	if serr := scanner.Err(); serr != nil && ctx.Err() == nil {
		log.Error("log stream ended with error", zap.Error(serr))
	} else {
		log.Info("log stream ended")
	}
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

func (c *Collector) writeLogLine(writer *storage.FileWriter, namespace, pod, line string) error {
	if err := writer.Write(line); err != nil {
		return err
	}
	if c.indexes != nil {
		c.indexes.ObserveLine(namespace, pod, line)
	}
	return nil
}
