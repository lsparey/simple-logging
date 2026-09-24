package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/internal/metrics"
)

// RetentionManager periodically deletes log segments older than the
// configured retention window, and removes empty container/pod/namespace
// directories left behind. It also sweeps any legacy v0.11 <pod>.log files by
// mtime, which only exist when the startup migration was disabled.
type RetentionManager struct {
	logsRoot       string
	retentionDays  int
	checkInterval  time.Duration
	log            *zap.Logger
	compactIndexes func() error
	metrics        *metrics.Metrics
	isActive       func(namespace, pod string) bool
}

// SetActiveChecker tells retention which pods are still being collected.
// A running pod whose segments have all expired (it has been quiet for the
// whole retention window) keeps its meta.json, which is only written when
// its stream starts and records the pod's owning workload.
func (r *RetentionManager) SetActiveChecker(isActive func(namespace, pod string) bool) {
	r.isActive = isActive
}

// SetMetrics makes retention count the segments it deletes in m.
func (r *RetentionManager) SetMetrics(m *metrics.Metrics) {
	r.metrics = m
}

// SetIndexCompactor registers the index cleanup run after stale log segments
// are removed. It is optional so retention remains usable without indexes.
func (r *RetentionManager) SetIndexCompactor(compact func() error) {
	r.compactIndexes = compact
}

// NewRetentionManager creates a RetentionManager.
func NewRetentionManager(logsRoot string, retentionDays int, checkInterval time.Duration, log *zap.Logger) *RetentionManager {
	return &RetentionManager{
		logsRoot:      logsRoot,
		retentionDays: retentionDays,
		checkInterval: checkInterval,
		log:           log,
	}
}

// dailySweepDelay is how long after each UTC midnight the extra daily sweep
// runs: late enough that the previous day's segments are closed, early
// enough that the day that just expired is deleted promptly.
const dailySweepDelay = 5 * time.Minute

// Run starts the retention loop. It performs an initial sweep immediately,
// then repeats on every checkInterval tick and also shortly after every UTC
// midnight. The midnight sweep is what bounds the retention overshoot: a
// day's segment expires at a midnight, and without it a 24h interval that
// happened to start just after midnight would leave the segment in place for
// almost another day. It blocks until ctx is cancelled.
func (r *RetentionManager) Run(ctx context.Context) {
	r.log.Info("retention manager starting",
		zap.Int("retention_days", r.retentionDays),
		zap.Duration("check_interval", r.checkInterval),
	)

	r.sweep()

	ticker := time.NewTicker(r.checkInterval)
	defer ticker.Stop()
	daily := time.NewTimer(time.Until(nextDailySweep(time.Now())))
	defer daily.Stop()

	for {
		select {
		case <-ticker.C:
			r.sweep()
		case <-daily.C:
			r.sweep()
			daily.Reset(time.Until(nextDailySweep(time.Now())))
		case <-ctx.Done():
			r.log.Info("retention manager stopped")
			return
		}
	}
}

// nextDailySweep returns the first UTC midnight plus dailySweepDelay that is
// after now.
func nextDailySweep(now time.Time) time.Time {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(dailySweepDelay)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// sweep deletes expired log segments (and any expired legacy log files),
// then compacts indexes if anything was deleted.
func (r *RetentionManager) sweep() {
	today := time.Now().UTC()
	cutoffDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -r.retentionDays)

	deleted := r.sweepSegments(cutoffDate)
	deleted += r.sweepLegacyFiles()
	r.metrics.SegmentsDeleted(metrics.ReasonRetention, deleted)

	if deleted > 0 && r.compactIndexes != nil {
		if err := r.compactIndexes(); err != nil {
			r.log.Error("failed to compact indexes after retention", zap.Error(err))
		}
	}
}

// sweepSegments walks <logsRoot>/<ns>/<pod>/<container>/*.log and deletes any
// segment whose date is strictly before cutoffDate, then removes any
// container/pod/namespace directory left empty as a result.
func (r *RetentionManager) sweepSegments(cutoffDate time.Time) int {
	r.log.Debug("retention sweep started", zap.Time("cutoff_date", cutoffDate))

	deleted := 0
	namespaces, err := os.ReadDir(r.logsRoot)
	if err != nil {
		r.log.Error("failed to read logs root", zap.String("path", r.logsRoot), zap.Error(err))
		return 0
	}

	for _, nsEntry := range namespaces {
		if !nsEntry.IsDir() || strings.HasPrefix(nsEntry.Name(), ".") {
			continue
		}
		nsDir := filepath.Join(r.logsRoot, nsEntry.Name())

		pods, err := os.ReadDir(nsDir)
		if err != nil {
			r.log.Error("failed to read namespace dir", zap.String("path", nsDir), zap.Error(err))
			continue
		}
		for _, podEntry := range pods {
			if !podEntry.IsDir() {
				continue // legacy <pod>.log file, handled by sweepLegacyFiles
			}
			podDir := filepath.Join(nsDir, podEntry.Name())
			active := r.isActive != nil && r.isActive(nsEntry.Name(), podEntry.Name())
			deleted += r.sweepPodDirLocked(podDir, cutoffDate, active)
		}

		removeIfEmpty(nsDir, r.log)
	}

	r.log.Debug("retention sweep complete", zap.Int("segments_deleted", deleted))
	return deleted
}

// sweepPodDirLocked deletes expired segments under one pod directory and
// removes now-empty container directories, and meta.json and the pod
// directory itself if no container has any segments left and the pod isn't
// active.
func (r *RetentionManager) sweepPodDirLocked(podDir string, cutoffDate time.Time, active bool) int {
	deleted := 0
	containers, err := os.ReadDir(podDir)
	if err != nil {
		r.log.Error("failed to read pod dir", zap.String("path", podDir), zap.Error(err))
		return 0
	}

	anyContainerLeft := false
	for _, containerEntry := range containers {
		if !containerEntry.IsDir() {
			continue // meta.json
		}
		containerDir := filepath.Join(podDir, containerEntry.Name())

		segments, err := os.ReadDir(containerDir)
		if err != nil {
			r.log.Error("failed to read container dir", zap.String("path", containerDir), zap.Error(err))
			anyContainerLeft = true
			continue
		}

		remaining := 0
		for _, segEntry := range segments {
			if segEntry.IsDir() {
				continue
			}
			date, err := SegmentDate(segEntry.Name())
			if err != nil {
				continue
			}
			if !date.Before(cutoffDate) {
				remaining++
				continue
			}
			segPath := filepath.Join(containerDir, segEntry.Name())
			if err := os.Remove(segPath); err != nil {
				r.log.Error("failed to delete log segment", zap.String("path", segPath), zap.Error(err))
				remaining++
				continue
			}
			r.log.Info("deleted expired log segment", zap.String("path", segPath), zap.Time("segment_date", date))
			deleted++
		}

		if remaining == 0 {
			if err := os.Remove(containerDir); err != nil && !os.IsNotExist(err) {
				r.log.Warn("failed to remove empty container dir", zap.String("path", containerDir), zap.Error(err))
				anyContainerLeft = true
			}
		} else {
			anyContainerLeft = true
		}
	}

	if !anyContainerLeft && !active {
		_ = os.Remove(filepath.Join(podDir, metaFileName))
		if err := os.Remove(podDir); err != nil && !os.IsNotExist(err) {
			r.log.Warn("failed to remove empty pod dir", zap.String("path", podDir), zap.Error(err))
		}
	}
	return deleted
}

// sweepLegacyFiles deletes any remaining v0.11 <ns>/<pod>.log files whose
// mtime is outside the retention window. These only exist when the startup
// migration was disabled (MIGRATE_LEGACY=false); they carry no segment date
// to key off, so mtime is the only signal available, matching the pre-Phase-1
// behaviour exactly for anyone using that escape hatch.
func (r *RetentionManager) sweepLegacyFiles() int {
	cutoff := time.Now().Add(-time.Duration(r.retentionDays) * 24 * time.Hour)
	deleted := 0

	namespaces, err := os.ReadDir(r.logsRoot)
	if err != nil {
		return 0
	}
	for _, nsEntry := range namespaces {
		if !nsEntry.IsDir() || strings.HasPrefix(nsEntry.Name(), ".") {
			continue
		}
		nsDir := filepath.Join(r.logsRoot, nsEntry.Name())
		entries, err := os.ReadDir(nsDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if !info.ModTime().Before(cutoff) {
				continue
			}
			path := filepath.Join(nsDir, e.Name())
			if err := os.Remove(path); err != nil {
				r.log.Error("failed to delete legacy log file", zap.String("path", path), zap.Error(err))
				continue
			}
			r.log.Info("deleted expired legacy log file", zap.String("path", path), zap.Time("last_modified", info.ModTime()))
			deleted++
		}
	}
	return deleted
}

func removeIfEmpty(dir string, log *zap.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		return
	}
	if err := os.Remove(dir); err != nil {
		log.Warn("failed to remove empty directory", zap.String("path", dir), zap.Error(err))
		return
	}
	log.Info("removed empty directory", zap.String("path", dir))
}
