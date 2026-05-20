package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/lsparey/simple-logging/internal/api"
	"github.com/lsparey/simple-logging/internal/collector"
	"github.com/lsparey/simple-logging/internal/config"
	"github.com/lsparey/simple-logging/internal/k8s"
	"github.com/lsparey/simple-logging/internal/storage"
)

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
		zap.String("logs_root", cfg.LogsRoot),
		zap.Int("grpc_web_port", cfg.GRPCWebPort),
		zap.Int("retention_days", cfg.RetentionDays),
		zap.Duration("retention_check_interval", cfg.RetentionCheckInterval),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Phase 4: Kubernetes client & pod watcher ──────────────────────────────
	cs, err := k8s.NewClientset()
	if err != nil {
		log.Fatal("failed to create kubernetes clientset", zap.Error(err))
	}

	// ── Phase 5/6: Log Collector & FileWriter ────────────────────────────────
	coll := collector.New(cs, cfg.LogsRoot, log)

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
	go retention.Run(ctx)

	// ── Phase 8/9: gRPC Service & gRPC-Web Server ───────────────────
	svc := api.NewLogService(cfg.LogsRoot, coll)
	srv := api.NewServer(cfg.GRPCWebPort, svc, cfg.RESTDebugEnabled, log)

	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Start() }()

	<-ctx.Done()
	log.Info("shutdown signal received, stopping")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	srv.Shutdown(shutdownCtx)

	if err := <-serverErr; err != nil {
		log.Error("server exited with error", zap.Error(err))
	}
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
