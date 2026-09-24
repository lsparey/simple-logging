package k8s

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func makePod(name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:          name,
			Namespace:     "default",
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

// recorder collects the pods passed to a PodEventHandler.
type recorder struct {
	mu      sync.Mutex
	added   []*corev1.Pod
	deleted []*corev1.Pod
}

func (r *recorder) handler() PodEventHandler {
	return PodEventHandler{
		OnAdd: func(pod *corev1.Pod) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.added = append(r.added, pod)
		},
		OnDelete: func(pod *corev1.Pod) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.deleted = append(r.deleted, pod)
		},
	}
}

func (r *recorder) addedNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.added))
	for _, p := range r.added {
		names = append(names, p.Name)
	}
	return names
}

func (r *recorder) deletedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.deleted)
}

// startWatcher runs a PodWatcher over cs until the test ends, returning once
// its cache has synced.
func startWatcher(t *testing.T, cs *fake.Clientset, rec *recorder) {
	t.Helper()
	w, err := NewPodWatcher(cs, rec.handler(), 0, zap.NewNop())
	if err != nil {
		t.Fatalf("NewPodWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Start(ctx)

	syncCtx, cancelSync := context.WithTimeout(ctx, 5*time.Second)
	defer cancelSync()
	if err := w.WaitForCacheSync(syncCtx); err != nil {
		t.Fatalf("WaitForCacheSync: %v", err)
	}
}

func eventually(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func TestPodWatcher_AddsExistingRunningPodsButNotPendingOnes(t *testing.T) {
	cs := fake.NewClientset(
		makePod("running", corev1.PodRunning),
		makePod("finished", corev1.PodSucceeded),
		makePod("pending", corev1.PodPending),
	)
	rec := &recorder{}
	startWatcher(t, cs, rec)

	if !eventually(t, func() bool { return len(rec.addedNames()) == 2 }) {
		t.Fatalf("added = %v, want running and finished", rec.addedNames())
	}
	for _, name := range rec.addedNames() {
		if name == "pending" {
			t.Error("a Pending pod was added before any container started")
		}
	}
}

func TestPodWatcher_AddsPendingPodOnceItStartsRunning(t *testing.T) {
	pod := makePod("starting", corev1.PodPending)
	cs := fake.NewClientset(pod)
	rec := &recorder{}
	startWatcher(t, cs, rec)

	running := pod.DeepCopy()
	running.Status.Phase = corev1.PodRunning
	if _, err := cs.CoreV1().Pods("default").UpdateStatus(context.Background(), running, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if !eventually(t, func() bool { return len(rec.addedNames()) == 1 }) {
		t.Fatalf("added = %v, want the pod once it is Running", rec.addedNames())
	}
}

func TestPodWatcher_IgnoresUpdatesThatDontLeavePending(t *testing.T) {
	pod := makePod("steady", corev1.PodRunning)
	cs := fake.NewClientset(pod)
	rec := &recorder{}
	startWatcher(t, cs, rec)
	if !eventually(t, func() bool { return len(rec.addedNames()) == 1 }) {
		t.Fatal("expected the initial add")
	}

	done := pod.DeepCopy()
	done.Status.Phase = corev1.PodSucceeded
	if _, err := cs.CoreV1().Pods("default").UpdateStatus(context.Background(), done, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if got := rec.addedNames(); len(got) != 1 {
		t.Errorf("added = %v, want only the initial add", got)
	}
}

func TestPodWatcher_DeletesPods(t *testing.T) {
	cs := fake.NewClientset(makePod("doomed", corev1.PodRunning))
	rec := &recorder{}
	startWatcher(t, cs, rec)

	if err := cs.CoreV1().Pods("default").Delete(context.Background(), "doomed", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !eventually(t, func() bool { return rec.deletedCount() == 1 }) {
		t.Fatal("expected OnDelete for the deleted pod")
	}
}

func TestPodWatcher_StripsManagedFieldsBeforeHandlers(t *testing.T) {
	cs := fake.NewClientset(makePod("big", corev1.PodRunning))
	rec := &recorder{}
	startWatcher(t, cs, rec)

	if !eventually(t, func() bool { return len(rec.addedNames()) == 1 }) {
		t.Fatal("expected the pod to be added")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.added[0].ManagedFields != nil {
		t.Error("managedFields should be stripped from cached pods")
	}
}

func TestPodWatcher_NilHandlersAreAllowed(t *testing.T) {
	cs := fake.NewClientset(makePod("p", corev1.PodRunning))
	w, err := NewPodWatcher(cs, PodEventHandler{}, 0, zap.NewNop())
	if err != nil {
		t.Fatalf("NewPodWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)
	if err := w.WaitForCacheSync(ctx); err != nil {
		t.Fatalf("WaitForCacheSync: %v", err)
	}
	if err := cs.CoreV1().Pods("default").Delete(ctx, "p", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // no panic from the nil OnAdd/OnDelete
}

func TestPodWatcher_WaitForCacheSyncFailsWhenCancelled(t *testing.T) {
	w, err := NewPodWatcher(fake.NewClientset(), PodEventHandler{}, 0, zap.NewNop())
	if err != nil {
		t.Fatalf("NewPodWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // never started, so it can't sync
	if err := w.WaitForCacheSync(ctx); err == nil {
		t.Error("expected an error when ctx is cancelled before the cache syncs")
	}
}

func TestStripUnusedPodFields_PassesTombstonesThrough(t *testing.T) {
	tombstone := cache.DeletedFinalStateUnknown{Key: "default/p", Obj: makePod("p", corev1.PodRunning)}
	got, err := stripUnusedPodFields(tombstone)
	if err != nil {
		t.Fatalf("stripUnusedPodFields: %v", err)
	}
	if _, ok := got.(cache.DeletedFinalStateUnknown); !ok {
		t.Errorf("got %T, want the tombstone unchanged", got)
	}
}

func TestNewClientset_FailsOutsideACluster(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := NewClientset(); err == nil {
		t.Error("expected an error without in-cluster config")
	}
}

func TestPodWatcher_AddsPodThatGoesStraightFromPendingToSucceeded(t *testing.T) {
	pod := makePod("quick-job", corev1.PodPending)
	cs := fake.NewClientset(pod)
	rec := &recorder{}
	startWatcher(t, cs, rec)

	done := pod.DeepCopy()
	done.Status.Phase = corev1.PodSucceeded
	if _, err := cs.CoreV1().Pods("default").UpdateStatus(context.Background(), done, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if !eventually(t, func() bool { return len(rec.addedNames()) == 1 }) {
		t.Fatal("a pod that finished before it was seen Running should still be added")
	}
}
