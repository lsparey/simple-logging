package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

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
	log      *zap.Logger

	mu      sync.Mutex
	streams map[podKey]*activeStream

	// deploymentPods maps "namespace/deployment" -> set of pod names.
	deploymentPods map[string]map[string]struct{}
	// podDeployment maps "namespace/pod" -> deployment name.
	podDeployment map[string]string

	// jsonLogging tracks which pods have been determined to use JSON log formatting.
	jsonLogging map[podKey]bool
}

// New creates a Collector that streams pod logs to files under logsRoot.
func New(cs kubernetes.Interface, logsRoot string, log *zap.Logger) *Collector {
	return &Collector{
		cs:             cs,
		logsRoot:       logsRoot,
		log:            log,
		streams:        make(map[podKey]*activeStream),
		deploymentPods: make(map[string]map[string]struct{}),
		podDeployment:  make(map[string]string),
		jsonLogging:    make(map[podKey]bool),
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

	go c.runStream(ctx, pod, isRestart)
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

// runStream opens the Kubernetes log stream for pod and writes formatted lines
// to the pod's log file. It exits when ctx is cancelled or the stream closes.
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

	req := c.cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: containerName,
		Follow:    true,
	})

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
		if werr := writer.Write(line); werr != nil {
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
