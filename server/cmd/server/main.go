package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on http.DefaultServeMux
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/internal/api"
	"github.com/lsparey/simple-logging/internal/auth"
	"github.com/lsparey/simple-logging/internal/collector"
	"github.com/lsparey/simple-logging/internal/config"
	"github.com/lsparey/simple-logging/internal/indexes"
	"github.com/lsparey/simple-logging/internal/k8s"
	"github.com/lsparey/simple-logging/internal/metrics"
	"github.com/lsparey/simple-logging/internal/storage"
	"github.com/lsparey/simple-logging/internal/ui"
)

// version is set at image build time with -ldflags. Local builds use "dev".
var version = "dev"

// diskGuardCheckInterval is how often the disk guard checks LOGS_ROOT usage.
// Unlike retention (which defaults to once a day), a full disk is a fast-
// moving problem, so this isn't tied to RetentionCheckInterval or exposed as
// its own setting.
const diskGuardCheckInterval = time.Minute

func main() {
	// Bootstrap a temporary logger for startup errors before the real one is ready.
	tmpLog, _ := zap.NewProduction()

	cfg, err := config.Load()
	if err != nil {
		tmpLog.Fatal("invalid configuration", zap.Error(err))
	}

	log, err := buildLogger(cfg.LogLevel)
	if err != nil {
		tmpLog.Fatal("failed to build logger", zap.Error(err))
	}
	defer log.Sync() //nolint:errcheck

	log.Info("simple-logging starting",
		zap.String("version", version),
		zap.String("logs_root", cfg.LogsRoot),
		zap.Int("port", cfg.Port),
		zap.Int("retention_days", cfg.RetentionDays),
		zap.Duration("retention_check_interval", cfg.RetentionCheckInterval),
		zap.String("collection_mode", cfg.CollectionMode()),
		zap.String("node_logs_root", cfg.NodeLogsRoot),
		zap.String("node_name", cfg.NodeName),
		zap.Int("disk_high_water_percent", cfg.DiskHighWaterPercent),
		zap.Int("disk_low_water_percent", cfg.DiskLowWaterPercent),
		zap.Bool("metrics_enabled", cfg.MetricsEnabled),
		zap.Bool("basic_auth_enabled", cfg.AuthHTPasswdFile != ""),
	)

	if cfg.PPROFPort > 0 {
		pprofAddr := fmt.Sprintf("localhost:%d", cfg.PPROFPort)
		log.Info("pprof server enabled", zap.String("addr", pprofAddr))
		go func() {
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Error("pprof server exited", zap.Error(err))
			}
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Self-metrics ──────────────────────────────────────────────────────────
	// Always recorded (the UI's data dashboard reads them via GetStats), but
	// only served at /metrics when METRICS_ENABLED is set.
	m := metrics.New(func() (float64, error) { return storage.DiskUsageRatio(cfg.LogsRoot) })

	serverOpts := api.ServerOptions{
		Port:               cfg.Port,
		UI:                 ui.FS(),
		APIURL:             cfg.APIURL,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	}
	if cfg.MetricsEnabled {
		serverOpts.Metrics = m.Handler()
	}
	if cfg.AuthHTPasswdFile != "" {
		basic, err := auth.LoadHTPasswd(cfg.AuthHTPasswdFile)
		if err != nil {
			log.Fatal("failed to load basic auth htpasswd file", zap.Error(err))
		}
		log.Info("basic auth enabled", zap.Int("users", basic.Users()))
		serverOpts.Auth = basic
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	// Listens straight away so /healthz answers (and the liveness probe
	// passes) during a long migration; /readyz and the API answer 503 until
	// SetService is called below.
	srv := api.NewServer(serverOpts, log)
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Start() }()

	// ── Storage permissions ───────────────────────────────────────────────────
	if err := storage.CheckWritable(cfg.LogsRoot); err != nil {
		log.Fatal("LOGS_ROOT has entries this process cannot write, so logs would be silently lost. "+
			"If they were written by an older image that ran as root, chown them to this user, "+
			"e.g. with the Helm chart's volumePermissions.enabled=true for one upgrade",
			zap.Error(err))
	}

	// ── Storage layout migration ──────────────────────────────────────────────
	// Runs synchronously before anything else starts (collector, watcher,
	// API) so the readiness probe stays failing for the duration, and so
	// nothing reads the logs root mid-migration.
	if cfg.MigrateLegacy {
		migrationStart := time.Now()
		if err := storage.MigrateLegacyLayout(cfg.LogsRoot, log); err != nil {
			log.Fatal("legacy log layout migration failed", zap.Error(err))
		}
		log.Info("legacy log layout migration complete", zap.Duration("took", time.Since(migrationStart)))
	} else {
		log.Info("legacy log layout migration disabled (MIGRATE_LEGACY=false)")
	}

	// ── Phase 4: Kubernetes client & pod watcher ──────────────────────────────
	cs, err := k8s.NewClientset()
	if err != nil {
		log.Fatal("failed to create kubernetes clientset", zap.Error(err))
	}

	// ── Phase 5/6: Log Collector, Indexes & FileWriter ───────────────────────
	indexManager := indexes.NewManager(cfg.LogsRoot)
	coll := collector.NewWithIndexes(cs, cfg.LogsRoot, cfg.NodeLogsRoot, log, indexManager,
		collector.WithNodeName(cfg.NodeName), collector.WithMetrics(m))

	watcher, err := k8s.NewPodWatcher(cs, k8s.PodEventHandler{
		OnAdd:    coll.OnAdd,
		OnDelete: coll.OnDelete,
	}, 0, log)
	if err != nil {
		log.Fatal("failed to create pod watcher", zap.Error(err))
	}

	go watcher.Start(ctx)

	syncCtx, cancelSync := context.WithTimeout(ctx, 30*time.Second)
	defer cancelSync()
	if err := watcher.WaitForCacheSync(syncCtx); err != nil {
		log.Fatal("pod cache sync failed", zap.Error(err))
	}
	log.Info("pod cache synced")

	// ── Phase 7: Retention Manager ─────────────────────────────────
	retention := storage.NewRetentionManager(cfg.LogsRoot, cfg.RetentionDays, cfg.RetentionCheckInterval, log)
	retention.SetIndexCompactor(indexManager.Compact)
	retention.SetMetrics(m)
	retention.SetActiveChecker(coll.IsActive)
	go retention.Run(ctx)

	// Disk guard: a safety net for when retention alone doesn't keep LOGS_ROOT
	// usage down, checked far more often than retention runs since a full
	// disk can happen much faster than a day.
	diskGuard := storage.NewDiskGuard(cfg.LogsRoot, cfg.DiskHighWaterPercent, cfg.DiskLowWaterPercent, diskGuardCheckInterval, log)
	diskGuard.SetMetrics(m)
	diskGuard.SetIndexCompactor(indexManager.Compact)
	go diskGuard.Run(ctx)

	// ── LogService API ────────────────────────────────────────────────────────
	svc := api.NewLogServiceWithIndexes(cfg.LogsRoot, coll, coll, indexManager)
	svc.SetDiskWaterMarks(cfg.DiskHighWaterPercent, cfg.DiskLowWaterPercent)
	svc.SetMetrics(m)
	srv.SetService(svc)

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received, stopping")
	case err := <-serverErr:
		// Start only returns early on a listen failure (e.g. port in use).
		log.Fatal("server exited unexpectedly", zap.Error(err))
	}

	// Stop serving first so no new streams start, then stop collecting. The
	// pod watcher, retention and disk guard all stop with ctx, which is
	// already cancelled.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	srv.Shutdown(shutdownCtx)
	if err := <-serverErr; err != nil {
		log.Error("server exited with error", zap.Error(err))
	}

	// Close waits for every stream goroutine to exit, and each one closes its
	// segment writer on the way out, so all collected lines are on disk.
	coll.Close()
	log.Info("collector stopped, all log writers closed")
}

func buildLogger(level string) (*zap.Logger, error) {
	var zapLevel zap.AtomicLevel
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("unknown log level %q: %w", level, err)
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zapLevel
	return cfg.Build()
}
