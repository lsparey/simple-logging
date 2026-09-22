package storage

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// DiskGuard is a safety net against LOGS_ROOT filling up. Retention is the
// primary mechanism for keeping disk usage bounded; DiskGuard only acts if
// usage still crosses highWaterPercent (e.g. an unusually verbose burst of
// logging outpaces the configured retention window), deleting the globally
// oldest segments — regardless of namespace, pod or container — until usage
// drops back below lowWaterPercent.
type DiskGuard struct {
	logsRoot         string
	highWaterPercent int
	lowWaterPercent  int
	checkInterval    time.Duration
	log              *zap.Logger

	// usedPercent is diskUsedPercent by default; overridable in tests.
	usedPercent func(path string) (int, error)
}

// NewDiskGuard creates a DiskGuard for logsRoot.
func NewDiskGuard(logsRoot string, highWaterPercent, lowWaterPercent int, checkInterval time.Duration, log *zap.Logger) *DiskGuard {
	return &DiskGuard{
		logsRoot:         logsRoot,
		highWaterPercent: highWaterPercent,
		lowWaterPercent:  lowWaterPercent,
		checkInterval:    checkInterval,
		log:              log,
		usedPercent:      diskUsedPercent,
	}
}

// Run starts the disk-guard loop, checking usage on every checkInterval tick.
// It blocks until ctx is cancelled.
func (g *DiskGuard) Run(ctx context.Context) {
	g.log.Info("disk guard starting",
		zap.Int("high_water_percent", g.highWaterPercent),
		zap.Int("low_water_percent", g.lowWaterPercent),
		zap.Duration("check_interval", g.checkInterval),
	)

	ticker := time.NewTicker(g.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.check()
		case <-ctx.Done():
			g.log.Info("disk guard stopped")
			return
		}
	}
}

// check deletes the globally oldest segments if usage is at or above
// highWaterPercent, stopping once usage drops below lowWaterPercent or there
// is nothing left to delete.
func (g *DiskGuard) check() {
	usedPercent, err := g.usedPercent(g.logsRoot)
	if err != nil {
		g.log.Error("failed to check disk usage", zap.String("path", g.logsRoot), zap.Error(err))
		return
	}
	if usedPercent < g.highWaterPercent {
		return
	}

	g.log.Warn("disk usage at or above high water mark, deleting oldest log segments",
		zap.Int("used_percent", usedPercent),
		zap.Int("high_water_percent", g.highWaterPercent),
	)

	segments, err := allSegmentsByAge(g.logsRoot)
	if err != nil {
		g.log.Error("failed to list segments for disk guard", zap.Error(err))
		return
	}

	deleted := 0
	for _, seg := range segments {
		if usedPercent < g.lowWaterPercent {
			break
		}
		if err := os.Remove(seg.path); err != nil {
			g.log.Error("failed to delete log segment", zap.String("path", seg.path), zap.Error(err))
			continue
		}
		g.log.Warn("deleted log segment to relieve disk pressure",
			zap.String("path", seg.path),
			zap.Time("segment_date", seg.date),
		)
		deleted++
		cleanupEmptyDirsAbove(seg.path, g.log)

		usedPercent, err = g.usedPercent(g.logsRoot)
		if err != nil {
			g.log.Error("failed to recheck disk usage", zap.Error(err))
			return
		}
	}

	if usedPercent >= g.lowWaterPercent {
		g.log.Warn("disk usage remains above low water mark",
			zap.Int("used_percent", usedPercent),
			zap.Int("low_water_percent", g.lowWaterPercent),
			zap.Int("segments_deleted", deleted),
		)
	}
}

type agedSegment struct {
	path string
	date time.Time
}

// allSegmentsByAge returns every log segment under logsRoot across every
// namespace, pod and container, sorted oldest first.
func allSegmentsByAge(logsRoot string) ([]agedSegment, error) {
	var segments []agedSegment

	namespaceEntries, err := os.ReadDir(logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, nsEntry := range namespaceEntries {
		if !nsEntry.IsDir() || strings.HasPrefix(nsEntry.Name(), ".") {
			continue
		}
		namespace := nsEntry.Name()

		pods, err := ListPodDirs(logsRoot, namespace)
		if err != nil {
			return nil, err
		}
		for _, pod := range pods {
			containers, err := ListContainers(logsRoot, namespace, pod)
			if err != nil {
				return nil, err
			}
			for _, container := range containers {
				dates, err := ListSegments(logsRoot, namespace, pod, container)
				if err != nil {
					return nil, err
				}
				for _, dateStr := range dates {
					date, err := time.ParseInLocation(segmentDateForm, dateStr, time.UTC)
					if err != nil {
						continue
					}
					segments = append(segments, agedSegment{
						path: filepath.Join(ContainerDir(logsRoot, namespace, pod, container), dateStr+".log"),
						date: date,
					})
				}
			}
		}
	}

	sort.Slice(segments, func(i, j int) bool { return segments[i].date.Before(segments[j].date) })
	return segments, nil
}

// cleanupEmptyDirsAbove removes the container/pod/namespace directories above
// a just-deleted segment if they are now empty, mirroring the cleanup
// RetentionManager does after its own sweep.
func cleanupEmptyDirsAbove(segmentPath string, log *zap.Logger) {
	containerDir := filepath.Dir(segmentPath)
	podDir := filepath.Dir(containerDir)
	nsDir := filepath.Dir(podDir)

	removeIfEmpty(containerDir, log)

	if entries, err := os.ReadDir(podDir); err == nil && len(entries) > 0 {
		onlyMeta := true
		for _, e := range entries {
			if e.Name() != metaFileName {
				onlyMeta = false
				break
			}
		}
		if onlyMeta {
			_ = os.Remove(filepath.Join(podDir, metaFileName))
		}
	}
	removeIfEmpty(podDir, log)
	removeIfEmpty(nsDir, log)
}

// diskUsedPercent reports the percentage of logsRoot's filesystem currently
// in use, using the same available-space accounting as `df` (i.e. blocks
// reserved for privileged processes count as used).
func diskUsedPercent(path string) (int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	if total == 0 {
		return 0, nil
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	used := total - available
	return int(used * 100 / total), nil
}
