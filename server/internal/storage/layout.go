package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	metaFileName    = "meta.json"
	segmentDateForm = "2006-01-02"
)

// PodMeta holds metadata about a pod's log storage that is not derivable from
// the directory structure alone.
type PodMeta struct {
	Namespace  string    `json:"namespace"`
	Pod        string    `json:"pod"`
	Containers []string  `json:"containers"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
}

// PodDir returns <logsRoot>/<namespace>/<pod>.
func PodDir(logsRoot, namespace, pod string) string {
	return filepath.Join(logsRoot, namespace, pod)
}

// ContainerDir returns <logsRoot>/<namespace>/<pod>/<container>.
func ContainerDir(logsRoot, namespace, pod, container string) string {
	return filepath.Join(PodDir(logsRoot, namespace, pod), container)
}

// SegmentPath returns the path of the log segment for the given container and
// UTC date.
func SegmentPath(logsRoot, namespace, pod, container string, date time.Time) string {
	return filepath.Join(ContainerDir(logsRoot, namespace, pod, container), date.UTC().Format(segmentDateForm)+".log")
}

// SegmentDate parses the UTC date encoded in a segment file name (e.g.
// "2026-09-20.log"), returning an error if name is not a recognised segment.
func SegmentDate(name string) (time.Time, error) {
	if filepath.Ext(name) != ".log" {
		return time.Time{}, fmt.Errorf("not a segment file: %q", name)
	}
	return time.ParseInLocation(segmentDateForm, strings.TrimSuffix(name, ".log"), time.UTC)
}

// ListPodDirs returns the pod names with a log directory under a namespace,
// sorted alphabetically.
func ListPodDirs(logsRoot, namespace string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(logsRoot, namespace))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pods []string
	for _, e := range entries {
		if e.IsDir() {
			pods = append(pods, e.Name())
		}
	}
	sort.Strings(pods)
	return pods, nil
}

// ListContainers returns the container names with logs stored for a pod,
// sorted alphabetically.
func ListContainers(logsRoot, namespace, pod string) ([]string, error) {
	entries, err := os.ReadDir(PodDir(logsRoot, namespace, pod))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var containers []string
	for _, e := range entries {
		if e.IsDir() {
			containers = append(containers, e.Name())
		}
	}
	sort.Strings(containers)
	return containers, nil
}

// ListSegments returns the sorted (ascending) segment dates on disk for a
// pod's container, formatted as "YYYY-MM-DD" (without the .log extension).
func ListSegments(logsRoot, namespace, pod, container string) ([]string, error) {
	entries, err := os.ReadDir(ContainerDir(logsRoot, namespace, pod, container))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var segments []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, err := SegmentDate(e.Name()); err != nil {
			continue
		}
		segments = append(segments, strings.TrimSuffix(e.Name(), ".log"))
	}
	sort.Strings(segments)
	return segments, nil
}

// ReadPodMeta reads a pod's meta.json. A missing file returns a zero-value
// PodMeta (with Namespace/Pod filled in) and no error.
func ReadPodMeta(logsRoot, namespace, pod string) (PodMeta, error) {
	data, err := os.ReadFile(filepath.Join(PodDir(logsRoot, namespace, pod), metaFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return PodMeta{Namespace: namespace, Pod: pod}, nil
		}
		return PodMeta{}, err
	}
	var meta PodMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return PodMeta{}, err
	}
	return meta, nil
}

// WritePodMeta writes a pod's meta.json, creating the pod directory if needed.
func WritePodMeta(logsRoot string, meta PodMeta) error {
	dir := PodDir(logsRoot, meta.Namespace, meta.Pod)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, metaFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, metaFileName))
}

// metaMu serializes meta.json read-modify-write cycles across the process.
// Multiple containers of the same pod start their writers concurrently, and
// meta.json has no on-disk locking of its own (plain write-temp-then-rename),
// so without this a lost update could silently drop a container from the
// Containers list or an earlier FirstSeen/LastSeen. Contention is negligible:
// this only runs at writer construction, not per line.
var metaMu sync.Mutex

// recordContainerSeen updates a pod's meta.json to note that container has
// been observed at the given time, creating the record if needed.
func recordContainerSeen(logsRoot, namespace, pod, container string, at time.Time) error {
	metaMu.Lock()
	defer metaMu.Unlock()

	meta, err := ReadPodMeta(logsRoot, namespace, pod)
	if err != nil {
		return err
	}
	meta.Namespace = namespace
	meta.Pod = pod
	if meta.FirstSeen.IsZero() {
		meta.FirstSeen = at
	}
	if at.After(meta.LastSeen) {
		meta.LastSeen = at
	}
	if !containsString(meta.Containers, container) {
		meta.Containers = append(meta.Containers, container)
		sort.Strings(meta.Containers)
	}
	return WritePodMeta(logsRoot, meta)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
