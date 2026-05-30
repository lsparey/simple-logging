package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// TestGetDeploymentLogs_SameTimestampPreservesOrder verifies that a burst of
// log lines from a single pod that all share an identical RFC3339 timestamp
// (e.g. rapid app startup messages) are returned in file-insertion order and
// not arbitrarily reordered by the merge heap.
func TestGetDeploymentLogs_SameTimestampPreservesOrder(t *testing.T) {
	dir := t.TempDir()

	// Pod name follows Kubernetes convention: <deployment>-<rsHash>-<podHash>
	const deployment = "myapp"
	const pod = "myapp-abc12-xyz7890"

	const ts = "2026-05-20T10:00:00Z"
	const n = 10
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s [default/%s/app] startup message %d", ts, pod, i)
	}
	writeLogFile(t, dir, "default", pod, lines)

	svc := NewLogService(dir, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:  "default",
		Deployment: deployment,
	})
	if err != nil {
		t.Fatalf("GetDeploymentLogs: %v", err)
	}

	if len(resp.Lines) != n {
		t.Fatalf("expected %d lines, got %d", n, len(resp.Lines))
	}
	for i, got := range resp.Lines {
		if got != lines[i] {
			t.Errorf("line[%d]: got %q, want %q", i, got, lines[i])
		}
	}
}

// TestGetDeploymentLogs_SameTimestampAcrossPodsPreservesOrder verifies that
// when two pods each emit a burst of startup messages at the same timestamp,
// the lines from each pod are returned in their original file order (i.e. the
// heap merge does not interleave lines from the same pod out of sequence).
func TestGetDeploymentLogs_SameTimestampAcrossPodsPreservesOrder(t *testing.T) {
	dir := t.TempDir()

	const deployment = "myapp"
	podA := "myapp-abc12-aaaa1111"
	podB := "myapp-abc12-bbbb2222"

	const ts = "2026-05-20T10:00:00Z"
	const n = 5

	linesA := make([]string, n)
	for i := range linesA {
		linesA[i] = fmt.Sprintf("%s [default/%s/app] pod-a message %d", ts, podA, i)
	}
	linesB := make([]string, n)
	for i := range linesB {
		linesB[i] = fmt.Sprintf("%s [default/%s/app] pod-b message %d", ts, podB, i)
	}
	writeLogFile(t, dir, "default", podA, linesA)
	writeLogFile(t, dir, "default", podB, linesB)

	svc := NewLogService(dir, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:  "default",
		Deployment: deployment,
	})
	if err != nil {
		t.Fatalf("GetDeploymentLogs: %v", err)
	}

	if len(resp.Lines) != n*2 {
		t.Fatalf("expected %d lines, got %d", n*2, len(resp.Lines))
	}

	// Lines from pod-a must appear in their original order relative to each other.
	var gotA []string
	for _, l := range resp.Lines {
		if strings.Contains(l, "pod-a") {
			gotA = append(gotA, l)
		}
	}
	if len(gotA) != n {
		t.Fatalf("expected %d pod-a lines, got %d", n, len(gotA))
	}
	for i, got := range gotA {
		if got != linesA[i] {
			t.Errorf("pod-a line[%d]: got %q, want %q", i, got, linesA[i])
		}
	}

	// Lines from pod-b must appear in their original order relative to each other.
	var gotB []string
	for _, l := range resp.Lines {
		if strings.Contains(l, "pod-b") {
			gotB = append(gotB, l)
		}
	}
	if len(gotB) != n {
		t.Fatalf("expected %d pod-b lines, got %d", n, len(gotB))
	}
	for i, got := range gotB {
		if got != linesB[i] {
			t.Errorf("pod-b line[%d]: got %q, want %q", i, got, linesB[i])
		}
	}
}
