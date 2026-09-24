package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/gen/simplelog/v1/simplelogv1connect"
	"github.com/lsparey/simple-logging/internal/storage"
)

// recordOwner is a small test helper around storage.RecordOwner for setting
// up a pod's workload ownership precisely, beyond what writeLogFile's
// Deployment-name-pattern heuristic can express (StatefulSet, DaemonSet,
// Job/CronJob, bare Pod).
func recordOwner(t *testing.T, dir, namespace, pod, kind, name, cronJob string) {
	t.Helper()
	if err := storage.RecordOwner(dir, namespace, pod, kind, name, cronJob); err != nil {
		t.Fatalf("RecordOwner: %v", err)
	}
}

// ── ListWorkloads ────────────────────────────────────────────────────────────

func TestListWorkloads_GroupsByOwnerKindAndName(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{"line"})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")
	writeLogFile(t, dir, "default", "log-agent-abcde", []string{"line"})
	recordOwner(t, dir, "default", "log-agent-abcde", "DaemonSet", "log-agent", "")
	writeLogFile(t, dir, "default", "standalone-pod", []string{"line"})
	recordOwner(t, dir, "default", "standalone-pod", "Pod", "standalone-pod", "")

	checker := &fakeChecker{active: map[string]bool{"default/cache-0": true}}
	svc := NewLogService(dir, checker, checker)
	resp, err := call(context.Background(), svc.ListWorkloads, &pb.ListWorkloadsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListWorkloads: %v", err)
	}

	if len(resp.Workloads) != 3 {
		t.Fatalf("expected 3 workloads, got %d: %+v", len(resp.Workloads), resp.Workloads)
	}
	byKey := make(map[string]*pb.WorkloadInfo)
	for _, w := range resp.Workloads {
		byKey[w.Kind+"/"+w.Name] = w
	}

	sts := byKey["StatefulSet/cache"]
	if sts == nil {
		t.Fatal("missing StatefulSet/cache")
	}
	if !sts.Active {
		t.Error("expected StatefulSet/cache to be active")
	}
	if len(sts.Pods) != 1 || sts.Pods[0] != "cache-0" {
		t.Errorf("StatefulSet/cache pods = %v, want [cache-0]", sts.Pods)
	}

	ds := byKey["DaemonSet/log-agent"]
	if ds == nil {
		t.Fatal("missing DaemonSet/log-agent")
	}
	if ds.Active {
		t.Error("expected DaemonSet/log-agent to be inactive")
	}

	bare := byKey["Pod/standalone-pod"]
	if bare == nil {
		t.Fatal("missing Pod/standalone-pod")
	}
}

func TestListWorkloads_CronJobPodAppearsUnderBothJobAndCronJob(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "backup-2893471000-x2f9p", []string{"line"})
	recordOwner(t, dir, "default", "backup-2893471000-x2f9p", "Job", "backup-2893471000", "backup")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	resp, err := call(context.Background(), svc.ListWorkloads, &pb.ListWorkloadsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListWorkloads: %v", err)
	}

	if len(resp.Workloads) != 2 {
		t.Fatalf("expected 2 workloads (Job + CronJob), got %d: %+v", len(resp.Workloads), resp.Workloads)
	}
	byKey := make(map[string]*pb.WorkloadInfo)
	for _, w := range resp.Workloads {
		byKey[w.Kind+"/"+w.Name] = w
	}
	if byKey["Job/backup-2893471000"] == nil {
		t.Error("missing Job/backup-2893471000")
	}
	if byKey["CronJob/backup"] == nil {
		t.Error("missing CronJob/backup")
	}
}

func TestListWorkloads_MultipleCronJobRunsCollapseIntoOneCronJobEntry(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "backup-2893471000-aaaaa", []string{"line"})
	recordOwner(t, dir, "default", "backup-2893471000-aaaaa", "Job", "backup-2893471000", "backup")
	writeLogFile(t, dir, "default", "backup-2893481000-bbbbb", []string{"line"})
	recordOwner(t, dir, "default", "backup-2893481000-bbbbb", "Job", "backup-2893481000", "backup")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	resp, err := call(context.Background(), svc.ListWorkloads, &pb.ListWorkloadsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListWorkloads: %v", err)
	}

	// 2 distinct Job entries + 1 shared CronJob entry.
	if len(resp.Workloads) != 3 {
		t.Fatalf("expected 3 workloads, got %d: %+v", len(resp.Workloads), resp.Workloads)
	}
	for _, w := range resp.Workloads {
		if w.Kind == "CronJob" {
			if len(w.Pods) != 2 {
				t.Errorf("CronJob/backup pods = %v, want both job runs' pods", w.Pods)
			}
		}
	}
}

func TestListWorkloads_SkipsPodsWithoutOwnerKind(t *testing.T) {
	dir := t.TempDir()
	// writeLogFile only records an owner for names matching the Deployment
	// hash pattern; "plain-pod" does not, so it has no OwnerKind yet —
	// simulating a pod the collector hasn't (re-)observed since upgrading.
	writeLogFile(t, dir, "default", "plain-pod", []string{"line"})

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	resp, err := call(context.Background(), svc.ListWorkloads, &pb.ListWorkloadsRequest{Namespace: "default"})
	if err != nil {
		t.Fatalf("ListWorkloads: %v", err)
	}
	if len(resp.Workloads) != 0 {
		t.Errorf("expected no workloads for an unobserved pod, got %+v", resp.Workloads)
	}
}

func TestListWorkloads_RequiresNamespace(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	_, err := call(context.Background(), svc.ListWorkloads, &pb.ListWorkloadsRequest{})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

// ── GetWorkloadLogs ──────────────────────────────────────────────────────────

func TestGetWorkloadLogs_MergesAcrossPodsForNonDeploymentKind(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{
		"2026-05-20T10:00:00Z [default/cache-0/app] from cache-0",
	})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")
	writeLogFile(t, dir, "default", "cache-1", []string{
		"2026-05-20T10:00:01Z [default/cache-1/app] from cache-1",
	})
	recordOwner(t, dir, "default", "cache-1", "StatefulSet", "cache", "")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	resp, err := call(context.Background(), svc.GetWorkloadLogs, &pb.GetWorkloadLogsRequest{
		Namespace: "default", Kind: "StatefulSet", Name: "cache",
	})
	if err != nil {
		t.Fatalf("GetWorkloadLogs: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Fatalf("expected 2 merged lines, got %d: %v", len(resp.Lines), resp.Lines)
	}
	if !strings.Contains(resp.Lines[0], "cache-0") || !strings.Contains(resp.Lines[1], "cache-1") {
		t.Errorf("expected lines merged in time order, got %v", resp.Lines)
	}
}

func TestGetWorkloadLogs_KindPodSelectsOneOwnedPodDirectly(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "web-app-abc12-xyz89", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc12-xyz89/app] from replica 1",
	})
	recordOwner(t, dir, "default", "web-app-abc12-xyz89", "Deployment", "web-app", "")
	writeLogFile(t, dir, "default", "web-app-abc12-uvw34", []string{
		"2026-05-20T10:00:01Z [default/web-app-abc12-uvw34/app] from replica 2",
	})
	recordOwner(t, dir, "default", "web-app-abc12-uvw34", "Deployment", "web-app", "")

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	resp, err := call(context.Background(), svc.GetWorkloadLogs, &pb.GetWorkloadLogsRequest{
		Namespace: "default", Kind: "Pod", Name: "web-app-abc12-xyz89",
	})
	if err != nil {
		t.Fatalf("GetWorkloadLogs: %v", err)
	}
	if len(resp.Lines) != 1 || !strings.Contains(resp.Lines[0], "replica 1") {
		t.Errorf("expected only web-app-abc12-xyz89's own line, got %v", resp.Lines)
	}
}

func TestGetWorkloadLogs_NotFoundForUnknownWorkload(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	_, err := call(context.Background(), svc.GetWorkloadLogs, &pb.GetWorkloadLogsRequest{
		Namespace: "default", Kind: "StatefulSet", Name: "nonexistent",
	})
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestGetWorkloadLogs_RequiresNamespaceKindAndName(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	cases := []*pb.GetWorkloadLogsRequest{
		{Kind: "StatefulSet", Name: "cache"},
		{Namespace: "default", Name: "cache"},
		{Namespace: "default", Kind: "StatefulSet"},
	}
	for _, req := range cases {
		if _, err := call(context.Background(), svc.GetWorkloadLogs, req); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("request %+v: expected InvalidArgument, got %v", req, err)
		}
	}
}

// ── StreamWorkloadLogs ───────────────────────────────────────────────────────

func TestStreamWorkloadLogs_TailsActivePods(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{
		"2026-05-20T10:00:00Z [default/cache-0/app] existing line",
	})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")

	checker := &fakeChecker{active: map[string]bool{"default/cache-0": true}}
	svc := NewLogService(dir, checker, checker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := startLineStream(ctx, newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamWorkloadLogs,
		&pb.StreamWorkloadLogsRequest{Namespace: "default", Kind: "StatefulSet", Name: "cache"})

	// Give the goroutine time to open and seek to EOF.
	time.Sleep(50 * time.Millisecond)

	logPath := fmt.Sprintf("%s/default/cache-0/app/2026-05-20.log", dir)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	fmt.Fprintln(f, "2026-05-20T10:00:01Z [default/cache-0/app] new line")
	f.Close()

	select {
	case line := <-stream.lines:
		if !strings.Contains(line, "new line") {
			t.Errorf("unexpected line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for streamed line")
	}
	cancel()
	<-stream.err
}

func TestStreamWorkloadLogs_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	err := <-startLineStream(context.Background(), newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamWorkloadLogs,
		&pb.StreamWorkloadLogsRequest{Namespace: "default", Kind: "StatefulSet", Name: "ghost"}).err
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestStreamWorkloadLogs_NamespaceWideTailsEveryPod(t *testing.T) {
	dir := t.TempDir()
	writeLogFile(t, dir, "default", "cache-0", []string{
		"2026-05-20T10:00:00Z [default/cache-0/app] existing cache line",
	})
	recordOwner(t, dir, "default", "cache-0", "StatefulSet", "cache", "")
	writeLogFile(t, dir, "default", "web-app-abc", []string{
		"2026-05-20T10:00:00Z [default/web-app-abc/app] existing web line",
	})

	checker := &fakeChecker{active: map[string]bool{"default/cache-0": true, "default/web-app-abc": true}}
	svc := NewLogService(dir, checker, checker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := startLineStream(ctx, newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamWorkloadLogs,
		&pb.StreamWorkloadLogsRequest{Namespace: "default"})

	time.Sleep(50 * time.Millisecond)

	appendLine(t, fmt.Sprintf("%s/default/cache-0/app/2026-05-20.log", dir), "2026-05-20T10:00:01Z [default/cache-0/app] new cache line")
	appendLine(t, fmt.Sprintf("%s/default/web-app-abc/app/2026-05-20.log", dir), "2026-05-20T10:00:01Z [default/web-app-abc/app] new web line")

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case line := <-stream.lines:
			seen[line] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for both tails, got: %v", seen)
		}
	}
	cancel()
	<-stream.err
}

func TestStreamWorkloadLogs_NamespaceWideNotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	err := <-startLineStream(context.Background(), newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamWorkloadLogs,
		&pb.StreamWorkloadLogsRequest{Namespace: "empty-ns"}).err
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestStreamWorkloadLogs_RejectsKindWithoutName(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{})
	err := <-startLineStream(context.Background(), newTestClient(t, svc), simplelogv1connect.LogServiceClient.StreamWorkloadLogs,
		&pb.StreamWorkloadLogsRequest{Namespace: "default", Kind: "StatefulSet"}).err
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}
