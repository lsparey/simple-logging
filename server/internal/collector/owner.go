package collector

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// resolveOwner derives the workload that owns pod from its ownerReferences,
// without any extra API calls (avoiding RBAC beyond the pod watch the
// collector already has):
//
//   - ReplicaSet, with a matching pod-template-hash label -> Deployment,
//     stripping the trailing "-<hash>" from the ReplicaSet name. A
//     ReplicaSet without that label (hand-created, not Deployment-managed)
//     is reported as-is.
//   - StatefulSet, DaemonSet -> reported as-is.
//   - Job -> reported as-is; callers that want the CronJob name too should
//     use inferCronJobName on the returned name.
//   - No recognised owner -> "Pod", the pod's own name.
//
// Only the first ownerReference is consulted; a pod controlled by more than
// one owner is not a case Kubernetes itself expects.
func resolveOwner(pod *corev1.Pod) (kind, name string) {
	for _, ref := range pod.OwnerReferences {
		switch ref.Kind {
		case "ReplicaSet":
			if hash := pod.Labels["pod-template-hash"]; hash != "" && strings.HasSuffix(ref.Name, "-"+hash) {
				return "Deployment", strings.TrimSuffix(ref.Name, "-"+hash)
			}
			return "ReplicaSet", ref.Name
		case "StatefulSet", "DaemonSet", "Job":
			return ref.Kind, ref.Name
		}
	}
	return "Pod", pod.Name
}

// inferCronJobName reports whether jobName looks like it was generated for a
// CronJob run (Kubernetes names these "<cronjob>-<10-digit unix seconds>"),
// returning the CronJob name if so. Fetching the Job resource to read its own
// ownerReference would need batch/jobs get RBAC the collector doesn't
// otherwise require; this name-pattern heuristic avoids that.
func inferCronJobName(jobName string) (cronJob string, ok bool) {
	idx := strings.LastIndex(jobName, "-")
	if idx < 0 {
		return "", false
	}
	suffix := jobName[idx+1:]
	if len(suffix) != 10 {
		return "", false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return jobName[:idx], true
}
