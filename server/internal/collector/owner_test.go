package collector

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveOwner(t *testing.T) {
	cases := []struct {
		name      string
		pod       *corev1.Pod
		wantKind  string
		wantOwner string
	}{
		{
			name: "ReplicaSet with matching pod-template-hash resolves to Deployment",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "web-app-6d8c7f9c4b-x2f9p",
					Labels:          map[string]string{"pod-template-hash": "6d8c7f9c4b"},
					OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-app-6d8c7f9c4b"}},
				},
			},
			wantKind:  "Deployment",
			wantOwner: "web-app",
		},
		{
			name: "ReplicaSet without a matching pod-template-hash label is reported as-is",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "manual-rs-x2f9p",
					OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "manual-rs"}},
				},
			},
			wantKind:  "ReplicaSet",
			wantOwner: "manual-rs",
		},
		{
			name: "StatefulSet owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "cache-0",
					OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "cache"}},
				},
			},
			wantKind:  "StatefulSet",
			wantOwner: "cache",
		},
		{
			name: "DaemonSet owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "log-agent-abcde",
					OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "log-agent"}},
				},
			},
			wantKind:  "DaemonSet",
			wantOwner: "log-agent",
		},
		{
			name: "Job owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "backup-28934710-x2f9p",
					OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: "backup-2893471000"}},
				},
			},
			wantKind:  "Job",
			wantOwner: "backup-2893471000",
		},
		{
			name: "no owner reference falls back to bare Pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "standalone-pod"},
			},
			wantKind:  "Pod",
			wantOwner: "standalone-pod",
		},
		{
			name: "unrecognised owner kind falls back to bare Pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "custom-pod",
					OwnerReferences: []metav1.OwnerReference{{Kind: "SomeCustomController", Name: "widget"}},
				},
			},
			wantKind:  "Pod",
			wantOwner: "custom-pod",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, name := resolveOwner(tc.pod)
			if kind != tc.wantKind || name != tc.wantOwner {
				t.Errorf("resolveOwner() = (%q, %q), want (%q, %q)", kind, name, tc.wantKind, tc.wantOwner)
			}
		})
	}
}

func TestInferCronJobName(t *testing.T) {
	cases := []struct {
		jobName   string
		wantCron  string
		wantMatch bool
	}{
		{"backup-2893471000", "backup", true},
		{"nightly-report-1234567890", "nightly-report", true},
		{"backup-289347100", "", false},   // 9 digits, not 10
		{"backup-28934710000", "", false}, // 11 digits, not 10
		{"backup-289347100a", "", false},  // non-numeric
		{"no-dash-at-all-here-", "", false},
		{"standalone", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.jobName, func(t *testing.T) {
			cron, ok := inferCronJobName(tc.jobName)
			if ok != tc.wantMatch || cron != tc.wantCron {
				t.Errorf("inferCronJobName(%q) = (%q, %v), want (%q, %v)", tc.jobName, cron, ok, tc.wantCron, tc.wantMatch)
			}
		})
	}
}
