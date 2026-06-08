package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

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
	nsDir := filepath.Join(dir, namespace)
	if err := os.MkdirAll(nsDir, 0755); err != nil {
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
	f, err := os.Create(filepath.Join(nsDir, pod+".log"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i, line := range lines {
		fmt.Fprintf(f, "2026-06-08T10:00:%02dZ [%s/%s/app] %s\n", i, namespace, pod, line)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	coll := New(fake.NewSimpleClientset(), dir, "", zap.NewNop())
	coll.detectJsonFromFile(namespace, pod)

	if !coll.IsJsonLogging(namespace, pod) {
		t.Error("expected stored log with startup lines to be detected as JSON")
	}
}
