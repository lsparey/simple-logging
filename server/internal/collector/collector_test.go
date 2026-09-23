package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/lsparey/simple-logging/internal/storage"

	"go.uber.org/zap"
)

func makePod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "uid-1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}
}

func TestCollector_OnAdd_IsActive(t *testing.T) {
	dir := t.TempDir()
	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	t.Cleanup(coll.Close)

	pod := makePod("default", "my-pod")
	coll.OnAdd(pod)

	if !coll.IsActive("default", "my-pod") {
		t.Error("expected pod to be active immediately after OnAdd")
	}
}

func TestCollector_OnDelete_NotActive(t *testing.T) {
	dir := t.TempDir()
	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	t.Cleanup(coll.Close)

	pod := makePod("default", "my-pod")
	coll.OnAdd(pod)
	coll.OnDelete(pod)

	if coll.IsActive("default", "my-pod") {
		t.Error("expected pod to be inactive after OnDelete")
	}
}

func TestCollector_OnDelete_Unknown_NoOp(t *testing.T) {
	dir := t.TempDir()
	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	t.Cleanup(coll.Close)

	// OnDelete for a pod never added should not panic.
	pod := makePod("default", "ghost-pod")
	coll.OnDelete(pod)

	if coll.IsActive("default", "ghost-pod") {
		t.Error("unexpected active state for unknown pod")
	}
}

func TestCollector_OnAdd_Restart_RemainsActive(t *testing.T) {
	dir := t.TempDir()
	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	t.Cleanup(coll.Close)

	pod := makePod("default", "my-pod")
	coll.OnAdd(pod) // first start

	// Simulate a pod restart (same name, new container/UID).
	pod2 := makePod("default", "my-pod")
	pod2.UID = "uid-2"
	coll.OnAdd(pod2) // restart

	if !coll.IsActive("default", "my-pod") {
		t.Error("expected pod to remain active after restart")
	}
}

func TestCollector_MultiplePods_Independent(t *testing.T) {
	dir := t.TempDir()
	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	t.Cleanup(coll.Close)

	podA := makePod("default", "pod-a")
	podB := makePod("default", "pod-b")

	coll.OnAdd(podA)
	coll.OnAdd(podB)
	coll.OnDelete(podA)

	if coll.IsActive("default", "pod-a") {
		t.Error("pod-a should be inactive after delete")
	}
	if !coll.IsActive("default", "pod-b") {
		t.Error("pod-b should still be active")
	}
}

func TestParseCRILogLine(t *testing.T) {
	wantTS := time.Date(2026, 1, 1, 0, 0, 0, 123000000, time.UTC)

	cases := []struct {
		name        string
		line        string
		wantTS      time.Time
		wantContent string
		wantPartial bool
	}{
		{
			name:        "CRI full line",
			line:        "2026-01-01T00:00:00.123000000Z stdout F hello world",
			wantTS:      wantTS,
			wantContent: "hello world",
			wantPartial: false,
		},
		{
			name:        "CRI partial line",
			line:        "2026-01-01T00:00:00.123000000Z stdout P hello wo",
			wantTS:      wantTS,
			wantContent: "hello wo",
			wantPartial: true,
		},
		{
			name:        "Docker JSON line",
			line:        `{"log":"hello world\n","stream":"stdout","time":"2026-01-01T00:00:00.123000000Z"}`,
			wantTS:      wantTS,
			wantContent: "hello world",
			wantPartial: false,
		},
		{
			name:        "Docker JSON partial line",
			line:        `{"log":"hello wo","stream":"stdout","time":"2026-01-01T00:00:00.123000000Z"}`,
			wantTS:      wantTS,
			wantContent: "hello wo",
			wantPartial: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, content, isPartial := parseCRILogLine(tc.line)
			if !ts.Equal(tc.wantTS) {
				t.Errorf("timestamp: got %v, want %v", ts, tc.wantTS)
			}
			if content != tc.wantContent {
				t.Errorf("content: got %q, want %q", content, tc.wantContent)
			}
			if isPartial != tc.wantPartial {
				t.Errorf("isPartial: got %v, want %v", isPartial, tc.wantPartial)
			}
		})
	}
}

func TestParseCRILogLine_FallsBackToNowOnUnrecognisedFormat(t *testing.T) {
	before := time.Now().UTC()
	ts, content, isPartial := parseCRILogLine("nospacesatall")
	after := time.Now().UTC()

	if ts.Before(before) || ts.After(after) {
		t.Errorf("expected fallback timestamp to be roughly now, got %v (window %v..%v)", ts, before, after)
	}
	if content != "nospacesatall" {
		t.Errorf("expected the raw line to be returned unchanged, got %q", content)
	}
	if isPartial {
		t.Error("unrecognised lines should not be marked partial")
	}
}

func TestParseCRILogLine_FallsBackToNowOnUnparseableTimestamp(t *testing.T) {
	before := time.Now().UTC()
	ts, content, isPartial := parseCRILogLine("not-a-timestamp stdout F hello world")
	after := time.Now().UTC()

	if ts.Before(before) || ts.After(after) {
		t.Errorf("expected fallback timestamp to be roughly now, got %v (window %v..%v)", ts, before, after)
	}
	if content != "hello world" {
		t.Errorf("expected content to still be extracted, got %q", content)
	}
	if isPartial {
		t.Error("expected the F flag to be recognised as non-partial")
	}
}

func TestJSONProbe_AllowsStartupLines(t *testing.T) {
	var probe jsonProbe
	lines := []string{
		"yarn run v1.22.22",
		"$ node --enable-source-maps dist/main",
		"Enabling inline tracing for this subgraph.",
		`{"level":"info","message":"started"}`,
		`{"level":"info","message":"listening"}`,
		`{"level":"info","message":"request"}`,
		`{"level":"info","message":"response"}`,
		`{"level":"info","message":"complete"}`,
	}

	for i, line := range lines {
		decided, isJSON := probe.observe(line)
		if i < len(lines)-1 && decided {
			t.Fatalf("probe decided too early after line %d", i+1)
		}
		if i == len(lines)-1 && (!decided || !isJSON) {
			t.Fatalf("probe did not recognise JSON logging: decided=%v isJSON=%v", decided, isJSON)
		}
	}
}

func TestJSONProbe_RejectsMostlyPlainText(t *testing.T) {
	var probe jsonProbe
	for i := 0; i < jsonProbeLines; i++ {
		line := fmt.Sprintf("plain log line %d", i)
		decided, isJSON := probe.observe(line)
		if i < jsonProbeLines-1 && decided {
			t.Fatalf("probe decided too early after line %d", i+1)
		}
		if i == jsonProbeLines-1 && (!decided || isJSON) {
			t.Fatalf("probe did not reject plain logging: decided=%v isJSON=%v", decided, isJSON)
		}
	}
}

func TestDetectJSONFromFile_AllowsStartupLines(t *testing.T) {
	dir := t.TempDir()
	namespace := "default"
	pod := "api"
	container := "app"
	containerDir := filepath.Join(dir, namespace, pod, container)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	lines := []string{
		"yarn run v1.22.22",
		"$ node --enable-source-maps dist/main",
		"Enabling inline tracing for this subgraph.",
		`{"level":"info","message":"started"}`,
		`{"level":"info","message":"listening"}`,
		`{"level":"info","message":"request"}`,
		`{"level":"info","message":"response"}`,
		`{"level":"info","message":"complete"}`,
	}
	f, err := os.Create(filepath.Join(containerDir, "2026-06-08.log"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i, line := range lines {
		fmt.Fprintf(f, "2026-06-08T10:00:%02dZ [%s/%s/%s] %s\n", i, namespace, pod, container, line)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	coll.detectJsonFromFile(namespace, pod, container)

	if !coll.IsJsonLogging(namespace, pod) {
		t.Error("expected stored log with startup lines to be detected as JSON")
	}
}

// waitFor polls cond until it returns true or timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// countLines returns how many lines across all of ns/pod's stored log
// segments (any container) contain substr. A pod with no segments counts as
// zero.
func countLines(t *testing.T, logsRoot, ns, pod, substr string) int {
	t.Helper()
	n := 0
	containers, err := storage.ListContainers(logsRoot, ns, pod)
	if err != nil {
		return 0
	}
	for _, container := range containers {
		segments, err := storage.ListSegments(logsRoot, ns, pod, container)
		if err != nil {
			continue
		}
		for _, segment := range segments {
			data, err := os.ReadFile(filepath.Join(storage.ContainerDir(logsRoot, ns, pod, container), segment+".log"))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, substr) {
					n++
				}
			}
		}
	}
	return n
}

func TestUsesFileTail(t *testing.T) {
	cases := []struct {
		name         string
		nodeLogsRoot string
		nodeName     string
		podNode      string
		want         bool
	}{
		{"api mode never tails", "", "", "node-a", false},
		{"api mode ignores node name", "", "node-a", "node-a", false},
		{"fileTail mode tails every pod", "/var/log/pods", "", "node-b", true},
		{"hybrid tails local pod", "/var/log/pods", "node-a", "node-a", true},
		{"hybrid streams remote pod", "/var/log/pods", "node-a", "node-b", false},
		{"hybrid streams unscheduled pod", "/var/log/pods", "node-a", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			coll := New(fake.NewSimpleClientset(), t.TempDir(), tc.nodeLogsRoot, zap.NewNop(), WithNodeName(tc.nodeName))
			pod := makePod("default", "p")
			pod.Spec.NodeName = tc.podNode
			if got := coll.usesFileTail(pod); got != tc.want {
				t.Errorf("usesFileTail: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollector_Hybrid_RemotePodStreamsViaAPI(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()
	// The fake clientset serves "fake logs" for any GetLogs call and reports
	// NotFound for the pod itself, so the stream runs exactly once.
	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop(), WithNodeName("node-a"))
	coll.apiMinBackoff, coll.apiMaxBackoff = 10*time.Millisecond, 10*time.Millisecond
	t.Cleanup(coll.Close)

	pod := makePod("default", "remote-pod")
	pod.Spec.NodeName = "node-b"
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "remote-pod", "fake logs") >= 1
	}) {
		t.Fatal("expected remote pod's logs to be collected via the API")
	}
	// Give the reconnect loop several backoff periods to misbehave.
	time.Sleep(100 * time.Millisecond)
	if n := countLines(t, logsRoot, "default", "remote-pod", "fake logs"); n != 1 {
		t.Errorf("expected exactly one stream for a pod that no longer exists, got %d lines", n)
	}
}

func TestCollector_Hybrid_LocalPodTailsFile(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()

	pod := makePod("default", "local-pod")
	pod.Spec.NodeName = "node-a"
	containerDir := filepath.Join(nodeLogs, "default_local-pod_uid-1", "app")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatal(err)
	}
	criLine := "2026-01-01T00:00:00.000000000Z stdout F hello from file\n"
	if err := os.WriteFile(filepath.Join(containerDir, "0.log"), []byte(criLine), 0644); err != nil {
		t.Fatal(err)
	}

	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop(), WithNodeName("node-a"))
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "local-pod", "hello from file") == 1
	}) {
		t.Fatal("expected local pod's logs to be tailed from the node filesystem")
	}
	if n := countLines(t, logsRoot, "default", "local-pod", "fake logs"); n != 0 {
		t.Errorf("local pod must not be streamed via the API, found %d API lines", n)
	}

	data, err := os.ReadFile(filepath.Join(logsRoot, "default", "local-pod", "app", "2026-01-01.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "2026-01-01T00:00:00Z") {
		t.Errorf("expected stored line to use the CRI source timestamp, got: %s", data)
	}
}

func TestAPIStream_ReconnectsWhilePodRunning(t *testing.T) {
	logsRoot := t.TempDir()
	pod := makePod("default", "running-pod")
	pod.Status.Phase = corev1.PodRunning

	coll := New(fake.NewSimpleClientset(pod), logsRoot, "", zap.NewNop())
	coll.apiMinBackoff, coll.apiMaxBackoff = 5*time.Millisecond, 20*time.Millisecond
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "running-pod", "fake logs") >= 3
	}) {
		t.Fatal("expected the API stream to be reopened while the pod is still running")
	}
}

func TestAPIStream_StopsWhenPodFinished(t *testing.T) {
	for _, phase := range []corev1.PodPhase{corev1.PodSucceeded, corev1.PodFailed} {
		t.Run(string(phase), func(t *testing.T) {
			logsRoot := t.TempDir()
			pod := makePod("default", "done-pod")
			pod.Status.Phase = phase

			coll := New(fake.NewSimpleClientset(pod), logsRoot, "", zap.NewNop())
			coll.apiMinBackoff, coll.apiMaxBackoff = 5*time.Millisecond, 5*time.Millisecond
			t.Cleanup(coll.Close)
			coll.OnAdd(pod)

			if !waitFor(t, 5*time.Second, func() bool {
				return countLines(t, logsRoot, "default", "done-pod", "fake logs") >= 1
			}) {
				t.Fatal("expected one stream of the finished pod's remaining logs")
			}
			time.Sleep(100 * time.Millisecond)
			if n := countLines(t, logsRoot, "default", "done-pod", "fake logs"); n != 1 {
				t.Errorf("expected exactly one stream for a finished pod, got %d lines", n)
			}
		})
	}
}

func TestAPIStream_UsesSourceTimestamp(t *testing.T) {
	logsRoot := t.TempDir()
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, &runtime.Unknown{Raw: []byte("2020-01-01T00:00:00.000000000Z hello from api\n")}, nil
	})

	coll := New(cs, logsRoot, "", zap.NewNop())
	coll.apiMinBackoff, coll.apiMaxBackoff = 10*time.Millisecond, 10*time.Millisecond
	t.Cleanup(coll.Close)

	pod := makePod("default", "ts-pod")
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "ts-pod", "hello from api") >= 1
	}) {
		t.Fatal("expected the API stream's log line to be collected")
	}

	data, err := os.ReadFile(filepath.Join(logsRoot, "default", "ts-pod", "app", "2020-01-01.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "2020-01-01T00:00:00Z") {
		t.Errorf("expected stored line to use the API's source timestamp, got: %s", data)
	}
}

func TestAPIStream_StopsWhenPodReplaced(t *testing.T) {
	logsRoot := t.TempDir()
	// The cluster now has a pod with the same name but a different UID: the
	// old stream must stop and leave collection to the new pod's OnAdd.
	replacement := makePod("default", "my-pod")
	replacement.UID = "uid-2"
	replacement.Status.Phase = corev1.PodRunning

	coll := New(fake.NewSimpleClientset(replacement), logsRoot, "", zap.NewNop())
	coll.apiMinBackoff, coll.apiMaxBackoff = 5*time.Millisecond, 5*time.Millisecond
	t.Cleanup(coll.Close)

	old := makePod("default", "my-pod") // uid-1
	coll.OnAdd(old)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "my-pod", "fake logs") >= 1
	}) {
		t.Fatal("expected the initial stream to deliver logs")
	}
	time.Sleep(100 * time.Millisecond)
	if n := countLines(t, logsRoot, "default", "my-pod", "fake logs"); n != 1 {
		t.Errorf("expected the stale stream to stop after the pod was replaced, got %d lines", n)
	}
}

// TestCollector_WriteFailureDropsLinesButKeepsStreaming verifies that a
// failed write (e.g. a temporarily full or read-only volume) is dropped and
// counted rather than ending the stream goroutine, and that the writer
// recovers automatically once the underlying problem clears.
func TestCollector_WriteFailureDropsLinesButKeepsStreaming(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()

	pod := makePod("default", "blocked-pod")
	pod.Spec.NodeName = "node-a"
	containerDir := filepath.Join(nodeLogs, "default_blocked-pod_uid-1", "app")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Block today's segment path with a directory in its place, so the
	// collector's first write fails regardless of the test process's
	// privileges (unlike a permission-based block, which root would bypass).
	today := time.Now().UTC().Format("2006-01-02")
	segmentDir := filepath.Join(logsRoot, "default", "blocked-pod", "app")
	if err := os.MkdirAll(segmentDir, 0755); err != nil {
		t.Fatal(err)
	}
	blockedPath := filepath.Join(segmentDir, today+".log")
	if err := os.Mkdir(blockedPath, 0755); err != nil {
		t.Fatal(err)
	}

	criLine := fmt.Sprintf("%sZ stdout F should be dropped\n", time.Now().UTC().Format("2006-01-02T15:04:05.000000000"))
	if err := os.WriteFile(filepath.Join(containerDir, "0.log"), []byte(criLine), 0644); err != nil {
		t.Fatal(err)
	}

	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop(), WithNodeName("node-a"))
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return coll.DroppedLines() >= 1
	}) {
		t.Fatal("expected the failed write to be counted as dropped")
	}
	if !coll.IsActive("default", "blocked-pod") {
		t.Error("expected the stream to remain active after a write failure")
	}

	// Clear the block and give the writer's backoff (>= 1s) time to elapse,
	// then verify a subsequent line gets through.
	if err := os.Remove(blockedPath); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)

	criLine2 := fmt.Sprintf("%sZ stdout F should succeed\n", time.Now().UTC().Format("2006-01-02T15:04:05.000000000"))
	f, err := os.OpenFile(filepath.Join(containerDir, "0.log"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(criLine2); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "blocked-pod", "should succeed") >= 1
	}) {
		t.Fatal("expected the line to be written once the writer recovered")
	}
}

// makeMultiContainerPod returns a pod with the given container names, in order.
func makeMultiContainerPod(namespace, name string, containers ...string) *corev1.Pod {
	pod := makePod(namespace, name)
	pod.Spec.Containers = nil
	for _, c := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: c})
	}
	return pod
}

func TestCollector_MultiContainer_CollectsAllContainers(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()

	pod := makeMultiContainerPod("default", "multi-pod", "app", "sidecar")
	for _, container := range []string{"app", "sidecar"} {
		containerDir := filepath.Join(nodeLogs, "default_multi-pod_uid-1", container)
		if err := os.MkdirAll(containerDir, 0755); err != nil {
			t.Fatal(err)
		}
		criLine := fmt.Sprintf("2026-01-01T00:00:00.000000000Z stdout F hello from %s\n", container)
		if err := os.WriteFile(filepath.Join(containerDir, "0.log"), []byte(criLine), 0644); err != nil {
			t.Fatal(err)
		}
	}

	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool {
		return countLines(t, logsRoot, "default", "multi-pod", "hello from app") == 1 &&
			countLines(t, logsRoot, "default", "multi-pod", "hello from sidecar") == 1
	}) {
		t.Fatal("expected both containers to be collected independently")
	}

	containers, err := storage.ListContainers(logsRoot, "default", "multi-pod")
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if want := []string{"app", "sidecar"}; len(containers) != 2 || containers[0] != want[0] || containers[1] != want[1] {
		t.Errorf("ListContainers = %v, want %v", containers, want)
	}
}

func TestCollector_MultiContainer_IsActiveUntilAllContainersStop(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()

	pod := makeMultiContainerPod("default", "multi-pod", "app", "sidecar")
	for _, container := range []string{"app", "sidecar"} {
		containerDir := filepath.Join(nodeLogs, "default_multi-pod_uid-1", container)
		if err := os.MkdirAll(containerDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(containerDir, "0.log"), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !coll.IsActive("default", "multi-pod") {
		t.Fatal("expected pod to be active immediately after OnAdd (any container running)")
	}

	coll.OnDelete(pod)
	if coll.IsActive("default", "multi-pod") {
		t.Error("expected pod to be inactive once every container's stream has stopped")
	}
}

func TestCollector_MultiContainer_JsonLoggingReflectsDefaultContainerOnly(t *testing.T) {
	logsRoot := t.TempDir()
	nodeLogs := t.TempDir()
	pod := makeMultiContainerPod("default", "multi-pod", "app", "sidecar")

	appDir := filepath.Join(nodeLogs, "default_multi-pod_uid-1", "app")
	sidecarDir := filepath.Join(nodeLogs, "default_multi-pod_uid-1", "sidecar")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sidecarDir, 0755); err != nil {
		t.Fatal(err)
	}

	// The default container ("app", first in the pod spec) logs JSON; the
	// sidecar logs plain text.
	var appLines, sidecarLines strings.Builder
	for i := 0; i < jsonRequiredMatches; i++ {
		fmt.Fprintf(&appLines, "2026-01-01T00:00:%02d.000000000Z stdout F {\"level\":\"info\",\"n\":%d}\n", i, i)
	}
	for i := 0; i < jsonProbeLines; i++ {
		fmt.Fprintf(&sidecarLines, "2026-01-01T00:00:%02d.000000000Z stdout F plain text line %d\n", i, i)
	}
	if err := os.WriteFile(filepath.Join(appDir, "0.log"), []byte(appLines.String()), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sidecarDir, "0.log"), []byte(sidecarLines.String()), 0644); err != nil {
		t.Fatal(err)
	}

	coll := New(fake.NewSimpleClientset(), logsRoot, nodeLogs, zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	if !waitFor(t, 5*time.Second, func() bool { return coll.IsJsonLogging("default", "multi-pod") }) {
		t.Fatal("expected pod to be detected as JSON logging based on its default container")
	}

	// Give the sidecar's (plain-text) probe time to also decide; it must not
	// overwrite the pod-level flag the default container's probe set.
	time.Sleep(100 * time.Millisecond)
	if !coll.IsJsonLogging("default", "multi-pod") {
		t.Error("sidecar's plain-text probe must not overwrite the default container's JSON flag")
	}
}

func TestCollector_OnAdd_RecordsOwnerInPodMeta(t *testing.T) {
	logsRoot := t.TempDir()

	pod := makePod("default", "web-app-6d8c7f9c4b-x2f9p")
	pod.Labels = map[string]string{"pod-template-hash": "6d8c7f9c4b"}
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-app-6d8c7f9c4b"}}

	coll := New(fake.NewSimpleClientset(), logsRoot, "", zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	// recordOwner runs synchronously in OnAdd, before the per-container
	// goroutines are started, so meta.json is written by the time OnAdd returns.
	meta, err := storage.ReadPodMeta(logsRoot, "default", "web-app-6d8c7f9c4b-x2f9p")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if meta.OwnerKind != "Deployment" || meta.OwnerName != "web-app" {
		t.Errorf("owner = (%q, %q), want (Deployment, web-app)", meta.OwnerKind, meta.OwnerName)
	}
}

func TestCollector_OnAdd_RecordsCronJobOwnerInPodMeta(t *testing.T) {
	logsRoot := t.TempDir()

	pod := makePod("default", "backup-2893471000-x2f9p")
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "Job", Name: "backup-2893471000"}}

	coll := New(fake.NewSimpleClientset(), logsRoot, "", zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	meta, err := storage.ReadPodMeta(logsRoot, "default", "backup-2893471000-x2f9p")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if meta.OwnerKind != "Job" || meta.OwnerName != "backup-2893471000" || meta.CronJobName != "backup" {
		t.Errorf("meta = %+v, want OwnerKind=Job OwnerName=backup-2893471000 CronJobName=backup", meta)
	}
}

func TestCollector_OnAdd_BarePodOwnerIsPod(t *testing.T) {
	logsRoot := t.TempDir()

	pod := makePod("default", "standalone-pod")

	coll := New(fake.NewSimpleClientset(), logsRoot, "", zap.NewNop())
	t.Cleanup(coll.Close)
	coll.OnAdd(pod)

	meta, err := storage.ReadPodMeta(logsRoot, "default", "standalone-pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if meta.OwnerKind != "Pod" || meta.OwnerName != "standalone-pod" {
		t.Errorf("owner = (%q, %q), want (Pod, standalone-pod)", meta.OwnerKind, meta.OwnerName)
	}
}
