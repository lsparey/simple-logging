package api

// Paging tests for GetLogs and GetDeploymentLogs using a 3-day, 2160-line
// fixture that mirrors the front-end mock.
//
// Fixture layout (per source)
//
//	3 days × 24 hours × 30 lines/hour = 2160 lines
//	  Per hour:
//	    20 "log entry N" lines with distinct timestamps spread across the hour.
//	    10 "burst N"     lines all sharing the same HH:00:00Z timestamp,
//	                     simulating concurrent writes at the same second.
//
//	Day 1  2024-01-13  lines    0 –  719
//	Day 2  2024-01-14  lines  720 – 1439
//	Day 3  2024-01-15  lines 1440 – 2159
//
//	Oldest line (index 0):   "2024-01-13T00:00:00Z INFO log entry 1 from <src>"
//	Newest line (index 2159):"2024-01-15T23:00:00Z INFO burst 10 from <src>"

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// ── Fixture ───────────────────────────────────────────────────────────────────

// generate3DayLogLines builds 2160 log lines spanning three calendar days.
// Each day is divided into 24 hours. Within each hour:
//   - 20 normal entries with distinct timestamps (minute/second derived from i).
//   - 10 burst entries that all share the hour's zero-minute timestamp
//     (HH:00:00Z), reproducing the same-timestamp burst scenario.
func generate3DayLogLines(source string) []string {
	days := []string{"2024-01-13", "2024-01-14", "2024-01-15"}
	lines := make([]string, 0, 2160)
	for _, day := range days {
		for h := 0; h < 24; h++ {
			hh := fmt.Sprintf("%02d", h)
			// 20 normal entries spread evenly across the hour.
			for i := 0; i < 20; i++ {
				mm := fmt.Sprintf("%02d", (i*60)/20)
				ss := fmt.Sprintf("%02d", (i*3)%60)
				lines = append(lines, fmt.Sprintf(
					"%sT%s:%s:%sZ INFO log entry %d from %s",
					day, hh, mm, ss, len(lines)+1, source,
				))
			}
			// 10 burst entries sharing the HH:00:00Z timestamp.
			for b := 1; b <= 10; b++ {
				lines = append(lines, fmt.Sprintf(
					"%sT%s:00:00Z INFO burst %d from %s",
					day, hh, b, source,
				))
			}
		}
	}
	return lines // 3 × 24 × 30 = 2160
}

const (
	fixtureLineCount = 2160
	pagingPageSize   = 200
)

// ── GetLogs paging ────────────────────────────────────────────────────────────

// TestGetLogs_3Day_LoadLastPageShowsMostRecent verifies that LoadLastPage=true
// returns exactly pageSize lines, all from the newest day (2024-01-15), with
// no lines from the two older days.
func TestGetLogs_3Day_LoadLastPageShowsMostRecent(t *testing.T) {
	const pod = "pagingpod"
	dir := t.TempDir()
	writeLogFile(t, dir, "default", pod, generate3DayLogLines(pod))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace:    "default",
		Pod:          pod,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(resp.Lines) != pagingPageSize {
		t.Fatalf("expected %d lines on last page, got %d", pagingPageSize, len(resp.Lines))
	}
	// Every line must be from day 3.
	for _, line := range resp.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Errorf("expected all last-page lines to be from 2024-01-15, got: %q", line)
			break
		}
	}
	// No line must come from day 1 or day 2.
	for _, line := range resp.Lines {
		if strings.Contains(line, "2024-01-13") || strings.Contains(line, "2024-01-14") {
			t.Errorf("expected no old-day lines on last page, got: %q", line)
			break
		}
	}
	// A prevPageToken must be present — there are 1960 older lines.
	if resp.PrevPageToken == "" {
		t.Error("expected prevPageToken to be set when older lines exist")
	}
}

// TestGetLogs_3Day_ForwardPaginationCoversAllLines verifies that paging forward
// from the start (no token) delivers all 2160 fixture lines, in file order,
// with no gaps or duplicates.
func TestGetLogs_3Day_ForwardPaginationCoversAllLines(t *testing.T) {
	const pod = "pagingpod"
	dir := t.TempDir()
	fixture := generate3DayLogLines(pod)
	writeLogFile(t, dir, "default", pod, fixture)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	var all []string
	var nextToken string
	const maxPages = 20
	for i := 0; i < maxPages; i++ {
		resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
			Namespace: "default",
			Pod:       pod,
			PageSize:  pagingPageSize,
			PageToken: nextToken,
		})
		if err != nil {
			t.Fatalf("GetLogs page %d: %v", i+1, err)
		}
		all = append(all, resp.Lines...)
		if resp.NextPageToken == "" {
			break
		}
		nextToken = resp.NextPageToken
	}

	if len(all) != fixtureLineCount {
		t.Fatalf("forward pagination: expected %d total lines, got %d", fixtureLineCount, len(all))
	}
	// First line must be the oldest entry.
	wantFirst := fmt.Sprintf("2024-01-13T00:00:00Z INFO log entry 1 from %s", pod)
	if all[0] != wantFirst {
		t.Errorf("first line: got %q, want %q", all[0], wantFirst)
	}
	// Last line must be the newest burst entry.
	wantLast := fmt.Sprintf("2024-01-15T23:00:00Z INFO burst 10 from %s", pod)
	if all[len(all)-1] != wantLast {
		t.Errorf("last line: got %q, want %q", all[len(all)-1], wantLast)
	}
	// Every line must match the fixture in file order.
	for i, got := range all {
		if got != fixture[i] {
			t.Errorf("line[%d]: got %q, want %q", i, got, fixture[i])
			break
		}
	}
}

// TestGetLogs_3Day_BackwardPaginationReachesOldestLogs starts from the most
// recent page (LoadLastPage=true) and pages backwards using prevPageToken
// until it is exhausted. The final page must start with the oldest log line.
func TestGetLogs_3Day_BackwardPaginationReachesOldestLogs(t *testing.T) {
	const pod = "pagingpod"
	dir := t.TempDir()
	fixture := generate3DayLogLines(pod)
	writeLogFile(t, dir, "default", pod, fixture)

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	// Load the most recent page.
	initial, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace:    "default",
		Pod:          pod,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("initial GetLogs (LoadLastPage): %v", err)
	}
	for _, line := range initial.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Fatalf("initial page: expected all lines from 2024-01-15, got %q", line)
		}
	}

	// Page backwards until prevPageToken is empty.
	const maxPages = 20
	prevToken := initial.PrevPageToken
	var finalResp *pb.GetLogsResponse
	for i := 0; i < maxPages; i++ {
		if prevToken == "" {
			break
		}
		r, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
			Namespace: "default",
			Pod:       pod,
			PageSize:  pagingPageSize,
			PageToken: prevToken,
		})
		if err != nil {
			t.Fatalf("GetLogs backward page %d: %v", i+1, err)
		}
		finalResp = r
		prevToken = r.PrevPageToken
	}

	if finalResp == nil {
		t.Fatal("expected at least one backward page (prevPageToken was already empty after initial load)")
	}
	if len(finalResp.Lines) == 0 {
		t.Fatal("oldest page returned no lines")
	}
	if finalResp.PrevPageToken != "" {
		t.Errorf("expected no prevPageToken on the oldest page, got %q", finalResp.PrevPageToken)
	}

	// The oldest page must consist entirely of day-1 lines.
	for _, line := range finalResp.Lines {
		if !strings.Contains(line, "2024-01-13") {
			t.Errorf("oldest page: expected only 2024-01-13 lines, got %q", line)
			break
		}
	}
	// The very first line of the oldest page must be fixture line 0.
	wantFirst := fmt.Sprintf("2024-01-13T00:00:00Z INFO log entry 1 from %s", pod)
	if finalResp.Lines[0] != wantFirst {
		t.Errorf("oldest page first line: got %q, want %q", finalResp.Lines[0], wantFirst)
	}
}

// TestGetLogs_3Day_FirstForwardPageHasNoPrevToken verifies that the first page
// loaded from the beginning of the file carries no prevPageToken (there is
// nothing before it) but does carry a nextPageToken (there are more lines).
func TestGetLogs_3Day_FirstForwardPageHasNoPrevToken(t *testing.T) {
	const pod = "pagingpod"
	dir := t.TempDir()
	writeLogFile(t, dir, "default", pod, generate3DayLogLines(pod))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
		Namespace: "default",
		Pod:       pod,
		PageSize:  pagingPageSize,
	})
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if resp.PrevPageToken != "" {
		t.Errorf("expected no prevPageToken on the first page, got %q", resp.PrevPageToken)
	}
	if resp.NextPageToken == "" {
		t.Error("expected a nextPageToken when more lines follow the first page")
	}
	// First line must be the oldest entry.
	wantFirst := fmt.Sprintf("2024-01-13T00:00:00Z INFO log entry 1 from %s", pod)
	if len(resp.Lines) == 0 || resp.Lines[0] != wantFirst {
		t.Errorf("first page first line: got %q, want %q",
			func() string {
				if len(resp.Lines) > 0 {
					return resp.Lines[0]
				}
				return "(empty)"
			}(), wantFirst)
	}
}

// TestGetLogs_3Day_BurstLinesIncludedOnCorrectPage verifies that burst entries
// (multiple lines sharing the same HH:00:00Z timestamp) appear together in a
// single page and are not split across two pages by the offset-based cursor.
func TestGetLogs_3Day_BurstLinesIncludedOnCorrectPage(t *testing.T) {
	const pod = "pagingpod"
	dir := t.TempDir()
	writeLogFile(t, dir, "default", pod, generate3DayLogLines(pod))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	// Collect all lines via forward pagination.
	var all []string
	var nextToken string
	for i := 0; i < 20; i++ {
		resp, err := svc.GetLogs(context.Background(), &pb.GetLogsRequest{
			Namespace: "default",
			Pod:       pod,
			PageSize:  pagingPageSize,
			PageToken: nextToken,
		})
		if err != nil {
			t.Fatalf("GetLogs page %d: %v", i+1, err)
		}
		all = append(all, resp.Lines...)
		if resp.NextPageToken == "" {
			break
		}
		nextToken = resp.NextPageToken
	}

	// Count burst lines for a specific hour across all pages.
	// Hour 5 of day 1: expect 10 burst lines all at "2024-01-13T05:00:00Z".
	burstTS := "2024-01-13T05:00:00Z INFO burst"
	count := 0
	for _, line := range all {
		if strings.HasPrefix(line, burstTS) {
			count++
		}
	}
	if count != 10 {
		t.Errorf("expected 10 burst lines for hour 5 of day 1, found %d", count)
	}

	// Verify they are in the correct order (burst 1 through burst 10).
	burstLines := make([]string, 0, 10)
	for _, line := range all {
		if strings.HasPrefix(line, burstTS) {
			burstLines = append(burstLines, line)
		}
	}
	for i, line := range burstLines {
		want := fmt.Sprintf("2024-01-13T05:00:00Z INFO burst %d from %s", i+1, pod)
		if line != want {
			t.Errorf("burst[%d]: got %q, want %q", i, line, want)
		}
	}
}

// ── GetDeploymentLogs paging ──────────────────────────────────────────────────

// TestGetDeploymentLogs_3Day_LoadLastPageShowsMostRecent verifies that
// LoadLastPage=true for a deployment returns pageSize lines, all from the
// newest day (2024-01-15).
func TestGetDeploymentLogs_3Day_LoadLastPageShowsMostRecent(t *testing.T) {
	// Pod name must satisfy the Kubernetes naming heuristic:
	// <deployment>-<rsHash>-<podHash>
	const (
		deployment = "myworker"
		pod        = "myworker-6abc1-def2345"
	)
	dir := t.TempDir()
	writeLogFile(t, dir, "default", pod, generate3DayLogLines(pod))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:    "default",
		Deployment:   deployment,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("GetDeploymentLogs: %v", err)
	}
	if len(resp.Lines) != pagingPageSize {
		t.Fatalf("expected %d lines, got %d", pagingPageSize, len(resp.Lines))
	}
	for _, line := range resp.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Errorf("expected all last-page lines to be from 2024-01-15, got: %q", line)
			break
		}
	}
	for _, line := range resp.Lines {
		if strings.Contains(line, "2024-01-13") || strings.Contains(line, "2024-01-14") {
			t.Errorf("expected no old-day lines on last page, got: %q", line)
			break
		}
	}
	// Lines must be in ascending timestamp order (the deployment merge-sorts).
	for i := 1; i < len(resp.Lines); i++ {
		tsPrev := resp.Lines[i-1][:len("2024-01-15T00:00:00Z")]
		tsCurr := resp.Lines[i][:len("2024-01-15T00:00:00Z")]
		if tsCurr < tsPrev {
			t.Errorf("lines not in ascending order at [%d]: %q then %q", i, resp.Lines[i-1], resp.Lines[i])
			break
		}
	}
}

// TestGetDeploymentLogs_3Day_BackwardPaginationReachesOldestLogs starts from
// the most recent page and pages backwards until prevPageToken is exhausted.
// The final page must contain day-1 content, with the very first line being
// the oldest log entry in the fixture.
func TestGetDeploymentLogs_3Day_BackwardPaginationReachesOldestLogs(t *testing.T) {
	const (
		deployment = "myworker"
		pod        = "myworker-6abc1-def2345"
	)
	dir := t.TempDir()
	writeLogFile(t, dir, "default", pod, generate3DayLogLines(pod))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	initial, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:    "default",
		Deployment:   deployment,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("initial GetDeploymentLogs (LoadLastPage): %v", err)
	}
	for _, line := range initial.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Fatalf("initial page: expected all lines from 2024-01-15, got %q", line)
		}
	}

	const maxPages = 25
	prevToken := initial.PrevPageToken
	var finalResp *pb.GetDeploymentLogsResponse
	for i := 0; i < maxPages; i++ {
		if prevToken == "" {
			break
		}
		r, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
			Namespace:  "default",
			Deployment: deployment,
			PageSize:   pagingPageSize,
			PageToken:  prevToken,
		})
		if err != nil {
			t.Fatalf("GetDeploymentLogs backward page %d: %v", i+1, err)
		}
		finalResp = r
		prevToken = r.PrevPageToken
	}

	if finalResp == nil {
		t.Fatal("expected at least one backward page")
	}
	if len(finalResp.Lines) == 0 {
		t.Fatal("oldest page returned no lines")
	}
	if finalResp.PrevPageToken != "" {
		t.Errorf("expected no prevPageToken on the oldest page, got %q", finalResp.PrevPageToken)
	}
	// All lines on the oldest page must be from day 1.
	for _, line := range finalResp.Lines {
		if !strings.Contains(line, "2024-01-13") {
			t.Errorf("oldest page: expected only 2024-01-13 lines, got %q", line)
			break
		}
	}
	// The first line of the oldest page must be the very first log entry.
	wantFirst := fmt.Sprintf("2024-01-13T00:00:00Z INFO log entry 1 from %s", pod)
	if finalResp.Lines[0] != wantFirst {
		t.Errorf("oldest page first line: got %q, want %q", finalResp.Lines[0], wantFirst)
	}
}

// TestGetDeploymentLogs_3Day_MultiPod_LoadLastPageShowsMostRecent verifies
// that with two pods contributing to the same deployment (4320 lines total),
// the initial load returns lines from the newest day only and merges them in
// timestamp order.
func TestGetDeploymentLogs_3Day_MultiPod_LoadLastPageShowsMostRecent(t *testing.T) {
	const deployment = "myworker"
	// Pod names must satisfy the Kubernetes naming heuristic.
	const podA = "myworker-6abc1-aaa11111"
	const podB = "myworker-6abc1-bbb22222"
	dir := t.TempDir()
	writeLogFile(t, dir, "default", podA, generate3DayLogLines(podA))
	writeLogFile(t, dir, "default", podB, generate3DayLogLines(podB))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})
	resp, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:    "default",
		Deployment:   deployment,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("GetDeploymentLogs: %v", err)
	}
	if len(resp.Lines) != pagingPageSize {
		t.Fatalf("expected %d lines, got %d", pagingPageSize, len(resp.Lines))
	}
	for _, line := range resp.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Errorf("expected all last-page lines from 2024-01-15, got: %q", line)
			break
		}
	}
	// Lines must be in ascending timestamp order.
	for i := 1; i < len(resp.Lines); i++ {
		tsPrev := resp.Lines[i-1][:len("2024-01-15T00:00:00Z")]
		tsCurr := resp.Lines[i][:len("2024-01-15T00:00:00Z")]
		if tsCurr < tsPrev {
			t.Errorf("lines not in ascending order at [%d]: %q then %q", i, resp.Lines[i-1], resp.Lines[i])
			break
		}
	}
}

// TestGetDeploymentLogs_3Day_MultiPod_BackwardPaginationReachesOldestLogs
// verifies that with two pods (4320 merged lines), paging all the way back
// reaches day-1 content, and the oldest line in the final page comes from day 1.
func TestGetDeploymentLogs_3Day_MultiPod_BackwardPaginationReachesOldestLogs(t *testing.T) {
	const deployment = "myworker"
	const podA = "myworker-6abc1-aaa11111"
	const podB = "myworker-6abc1-bbb22222"
	dir := t.TempDir()
	writeLogFile(t, dir, "default", podA, generate3DayLogLines(podA))
	writeLogFile(t, dir, "default", podB, generate3DayLogLines(podB))

	svc := NewLogService(dir, &fakeChecker{}, &fakeChecker{}, noopDeploymentMapper{})

	initial, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
		Namespace:    "default",
		Deployment:   deployment,
		PageSize:     pagingPageSize,
		LoadLastPage: true,
	})
	if err != nil {
		t.Fatalf("initial GetDeploymentLogs: %v", err)
	}
	for _, line := range initial.Lines {
		if !strings.Contains(line, "2024-01-15") {
			t.Fatalf("initial page: expected all lines from 2024-01-15, got %q", line)
		}
	}

	// Page backwards to the oldest content.
	const maxPages = 40
	prevToken := initial.PrevPageToken
	var finalResp *pb.GetDeploymentLogsResponse
	for i := 0; i < maxPages; i++ {
		if prevToken == "" {
			break
		}
		r, err := svc.GetDeploymentLogs(context.Background(), &pb.GetDeploymentLogsRequest{
			Namespace:  "default",
			Deployment: deployment,
			PageSize:   pagingPageSize,
			PageToken:  prevToken,
		})
		if err != nil {
			t.Fatalf("GetDeploymentLogs backward page %d: %v", i+1, err)
		}
		finalResp = r
		prevToken = r.PrevPageToken
	}

	if finalResp == nil {
		t.Fatal("expected at least one backward page")
	}
	if len(finalResp.Lines) == 0 {
		t.Fatal("oldest page returned no lines")
	}
	if finalResp.PrevPageToken != "" {
		t.Errorf("expected no prevPageToken on the oldest page, got %q", finalResp.PrevPageToken)
	}
	// All lines on the oldest page must be from day 1.
	for _, line := range finalResp.Lines {
		if !strings.Contains(line, "2024-01-13") {
			t.Errorf("oldest page: expected only 2024-01-13 lines, got %q", line)
			break
		}
	}
	// The first line of the oldest page must be the very first log entry from
	// the alphabetically-first pod (podA is sorted before podB, so its line
	// gets the lower insertion index in the merge heap and sorts first).
	if !strings.Contains(finalResp.Lines[0], "log entry 1 from") ||
		!strings.Contains(finalResp.Lines[0], "2024-01-13") {
		t.Errorf("oldest page first line: got %q, expected log entry 1 from 2024-01-13", finalResp.Lines[0])
	}
}
