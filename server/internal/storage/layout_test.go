package storage

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestNewSegmentWriter_ConcurrentContainersAllRecordedInPodMeta guards against
// the meta.json lost-update race: multiple containers of the same pod start
// their writers at roughly the same time (the normal case once the collector
// streams every container instead of just one), and every one of them must
// end up in Containers — none silently dropped by an interleaved
// read-modify-write.
func TestNewSegmentWriter_ConcurrentContainersAllRecordedInPodMeta(t *testing.T) {
	dir := t.TempDir()
	const n = 20
	containers := make([]string, n)
	for i := range containers {
		containers[i] = fmt.Sprintf("container-%02d", i)
	}

	var wg sync.WaitGroup
	writers := make([]*SegmentWriter, n)
	errs := make([]error, n)
	wg.Add(n)
	for i, container := range containers {
		go func(i int, container string) {
			defer wg.Done()
			w, err := NewSegmentWriter(dir, "ns", "pod", container)
			writers[i] = w
			errs[i] = err
		}(i, container)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("NewSegmentWriter(%s): %v", containers[i], err)
		}
		defer writers[i].Close()
	}

	meta, err := ReadPodMeta(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	got := append([]string(nil), meta.Containers...)
	sort.Strings(got)
	want := append([]string(nil), containers...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("meta.Containers = %v (len %d), want all %d containers: %v", got, len(got), len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("meta.Containers = %v, want %v", got, want)
		}
	}
}

func TestRecordContainerSeen_PreservesEarliestFirstSeen(t *testing.T) {
	dir := t.TempDir()
	early := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	if err := recordContainerSeen(dir, "ns", "pod", "app", early); err != nil {
		t.Fatalf("recordContainerSeen (early): %v", err)
	}
	if err := recordContainerSeen(dir, "ns", "pod", "sidecar", late); err != nil {
		t.Fatalf("recordContainerSeen (late): %v", err)
	}

	meta, err := ReadPodMeta(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if !meta.FirstSeen.Equal(early) {
		t.Errorf("FirstSeen = %v, want %v (should not move forward)", meta.FirstSeen, early)
	}
	if !meta.LastSeen.Equal(late) {
		t.Errorf("LastSeen = %v, want %v", meta.LastSeen, late)
	}
}

func TestRecordOwner_RoundTrips(t *testing.T) {
	dir := t.TempDir()

	if err := RecordOwner(dir, "ns", "pod", "Deployment", "web-app", ""); err != nil {
		t.Fatalf("RecordOwner: %v", err)
	}

	meta, err := ReadPodMeta(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if meta.OwnerKind != "Deployment" || meta.OwnerName != "web-app" {
		t.Errorf("owner = (%q, %q), want (Deployment, web-app)", meta.OwnerKind, meta.OwnerName)
	}
	if meta.CronJobName != "" {
		t.Errorf("CronJobName = %q, want empty", meta.CronJobName)
	}
}

func TestRecordOwner_SetsCronJobName(t *testing.T) {
	dir := t.TempDir()

	if err := RecordOwner(dir, "ns", "pod", "Job", "backup-2893471000", "backup"); err != nil {
		t.Fatalf("RecordOwner: %v", err)
	}

	meta, err := ReadPodMeta(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if meta.OwnerKind != "Job" || meta.OwnerName != "backup-2893471000" || meta.CronJobName != "backup" {
		t.Errorf("meta = %+v, want OwnerKind=Job OwnerName=backup-2893471000 CronJobName=backup", meta)
	}
}

func TestRecordOwner_DoesNotClobberContainersOrTimestamps(t *testing.T) {
	dir := t.TempDir()
	seenAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if err := recordContainerSeen(dir, "ns", "pod", "app", seenAt); err != nil {
		t.Fatalf("recordContainerSeen: %v", err)
	}
	if err := RecordOwner(dir, "ns", "pod", "Deployment", "web-app", ""); err != nil {
		t.Fatalf("RecordOwner: %v", err)
	}

	meta, err := ReadPodMeta(dir, "ns", "pod")
	if err != nil {
		t.Fatalf("ReadPodMeta: %v", err)
	}
	if len(meta.Containers) != 1 || meta.Containers[0] != "app" {
		t.Errorf("Containers = %v, want [app] (RecordOwner must not clobber it)", meta.Containers)
	}
	if !meta.FirstSeen.Equal(seenAt) {
		t.Errorf("FirstSeen = %v, want %v (RecordOwner must not clobber it)", meta.FirstSeen, seenAt)
	}
}
