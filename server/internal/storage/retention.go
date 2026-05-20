package storage

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// RetentionManager periodically deletes log files that have not been written to
// within the configured retention window, and removes empty namespace directories.
type RetentionManager struct {
	logsRoot      string
	retentionDays int
	checkInterval time.Duration
	log           *zap.Logger
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

// Run starts the retention loop. It performs an initial sweep immediately, then
// repeats on every checkInterval tick. It blocks until ctx is cancelled.
func (r *RetentionManager) Run(ctx context.Context) {
	r.log.Info("retention manager starting",
		zap.Int("retention_days", r.retentionDays),
		zap.Duration("check_interval", r.checkInterval),
	)

	// Run once at startup so stale files are cleaned before the first tick.
	r.sweep()

	ticker := time.NewTicker(r.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.sweep()
		case <-ctx.Done():
			r.log.Info("retention manager stopped")
			return
		}
	}
}

// sweep walks logsRoot and deletes any .log file whose modification time is
// older than the retention window. Empty namespace directories are then removed.
func (r *RetentionManager) sweep() {
	cutoff := time.Now().Add(-time.Duration(r.retentionDays) * 24 * time.Hour)
	r.log.Debug("retention sweep started", zap.Time("cutoff", cutoff))

	// Collect namespace directories to check for emptiness after deletions.
	nsDirs := make(map[string]struct{})

	// Walk one level deep: logsRoot/<namespace>/<pod>.log
	namespaceDirs, err := os.ReadDir(r.logsRoot)
	if err != nil {
		r.log.Error("failed to read logs root", zap.String("path", r.logsRoot), zap.Error(err))
		return
	}

	deleted := 0
	for _, nsEntry := range namespaceDirs {
		if !nsEntry.IsDir() {
			continue
		}
		nsDir := filepath.Join(r.logsRoot, nsEntry.Name())
		nsDirs[nsDir] = struct{}{}

		podFiles, err := os.ReadDir(nsDir)
		if err != nil {
			r.log.Error("failed to read namespace dir", zap.String("path", nsDir), zap.Error(err))
			continue
		}

		for _, podEntry := range podFiles {
			if podEntry.IsDir() {
				continue
			}
			if filepath.Ext(podEntry.Name()) != ".log" {
				continue
			}

			logPath := filepath.Join(nsDir, podEntry.Name())
			info, err := podEntry.Info()
			if err != nil {
				r.log.Warn("failed to stat log file", zap.String("path", logPath), zap.Error(err))
				continue
			}

			if info.ModTime().Before(cutoff) {
				if err := os.Remove(logPath); err != nil {
					r.log.Error("failed to delete log file", zap.String("path", logPath), zap.Error(err))
					continue
				}
				r.log.Info("deleted expired log file",
					zap.String("path", logPath),
					zap.Time("last_modified", info.ModTime()),
				)
				deleted++
			}
		}
	}

	// Remove any namespace directories that are now empty.
	removed := 0
	for nsDir := range nsDirs {
		entries, err := os.ReadDir(nsDir)
		if err != nil {
			continue
		}
		if len(entries) == 0 {
			if err := os.Remove(nsDir); err != nil {
				r.log.Warn("failed to remove empty namespace dir", zap.String("path", nsDir), zap.Error(err))
				continue
			}
			r.log.Info("removed empty namespace directory", zap.String("path", nsDir))
			removed++
		}
	}

	r.log.Debug("retention sweep complete",
		zap.Int("files_deleted", deleted),
		zap.Int("dirs_removed", removed),
	)
}
