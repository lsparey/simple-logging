package collector

import (
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
