package k8s

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// PodEventHandler is called when a pod is added to or deleted from the cluster.
// pod is guaranteed to be non-nil and fully populated.
type PodEventHandler struct {
	OnAdd    func(pod *corev1.Pod)
	OnDelete func(pod *corev1.Pod)
}

// PodWatcher watches all pods in all namespaces using a shared informer.
// It calls the provided handlers when pods are added or deleted.
type PodWatcher struct {
	informer cache.SharedIndexInformer
	log      *zap.Logger
}

// NewPodWatcher creates a PodWatcher that uses the given clientset and calls
// handler on pod add and delete events. resyncPeriod controls how often the
// informer performs a full re-list; 0 disables periodic resync.
func NewPodWatcher(cs kubernetes.Interface, handler PodEventHandler, resyncPeriod time.Duration, log *zap.Logger) (*PodWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		cs,
		resyncPeriod,
		// Watch all namespaces (no namespace filter).
	)

	podInformer := factory.Core().V1().Pods().Informer()

	_, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := toPod(obj)
			if !ok {
				return
			}
			// Only act on pods that are running or have run (not Pending
			// with no containers started yet).
			if pod.Status.Phase == corev1.PodPending {
				return
			}
			log.Debug("pod added", zap.String("namespace", pod.Namespace), zap.String("pod", pod.Name))
			if handler.OnAdd != nil {
				handler.OnAdd(pod)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod, ok1 := toPod(oldObj)
			newPod, ok2 := toPod(newObj)
			if !ok1 || !ok2 {
				return
			}
			// Treat a transition into Running as an "add" so that pods which
			// were Pending at startup get picked up once they start.
			if oldPod.Status.Phase == corev1.PodPending && newPod.Status.Phase == corev1.PodRunning {
				log.Debug("pod transitioned to Running", zap.String("namespace", newPod.Namespace), zap.String("pod", newPod.Name))
				if handler.OnAdd != nil {
					handler.OnAdd(newPod)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := toPod(obj)
			if !ok {
				// The object may be a DeletedFinalStateUnknown tombstone.
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				pod, ok = toPod(tombstone.Obj)
				if !ok {
					return
				}
			}
			log.Debug("pod deleted", zap.String("namespace", pod.Namespace), zap.String("pod", pod.Name))
			if handler.OnDelete != nil {
				handler.OnDelete(pod)
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add pod event handler: %w", err)
	}

	return &PodWatcher{informer: podInformer, log: log}, nil
}

// Start begins the informer's list-watch loop. It blocks until ctx is
// cancelled, making it suitable for running in a dedicated goroutine.
func (w *PodWatcher) Start(ctx context.Context) {
	w.log.Info("pod watcher starting")
	w.informer.Run(ctx.Done())
	w.log.Info("pod watcher stopped")
}

// WaitForCacheSync blocks until the informer's local cache is fully populated
// from the initial list call. Returns an error if ctx is cancelled first.
func (w *PodWatcher) WaitForCacheSync(ctx context.Context) error {
	if !cache.WaitForCacheSync(ctx.Done(), w.informer.HasSynced) {
		return fmt.Errorf("pod informer cache sync timed out or context cancelled")
	}
	return nil
}

// toPod safely casts an interface{} to *corev1.Pod.
func toPod(obj interface{}) (*corev1.Pod, bool) {
	pod, ok := obj.(*corev1.Pod)
	return pod, ok
}
