package collector

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/lsparey/simple-logging/internal/storage"
)

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
}

// New creates a Collector that streams pod logs to files under logsRoot.
func New(cs kubernetes.Interface, logsRoot string, log *zap.Logger) *Collector {
	return &Collector{
		cs:       cs,
		logsRoot: logsRoot,
		log:      log,
		streams:  make(map[podKey]*activeStream),
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
	for scanner.Scan() {
		line := fmt.Sprintf("%s [%s/%s/%s] %s",
			time.Now().UTC().Format(time.RFC3339),
			pod.Namespace, pod.Name, containerName,
			scanner.Text(),
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
