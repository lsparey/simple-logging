package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("GRPC_WEB_PORT", "")
	t.Setenv("RETENTION_DAYS", "")
	t.Setenv("RETENTION_CHECK_INTERVAL", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LogsRoot != dir {
		t.Errorf("LogsRoot: got %q, want %q", cfg.LogsRoot, dir)
	}
	if cfg.GRPCWebPort != 8080 {
		t.Errorf("GRPCWebPort: got %d, want 8080", cfg.GRPCWebPort)
	}
	if cfg.RetentionDays != 30 {
		t.Errorf("RetentionDays: got %d, want 30", cfg.RetentionDays)
	}
	if cfg.RetentionCheckInterval != 24*time.Hour {
		t.Errorf("RetentionCheckInterval: got %v, want 24h", cfg.RetentionCheckInterval)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want info", cfg.LogLevel)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("GRPC_WEB_PORT", "9090")
	t.Setenv("RETENTION_DAYS", "7")
	t.Setenv("RETENTION_CHECK_INTERVAL", "12h")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GRPCWebPort != 9090 {
		t.Errorf("GRPCWebPort: got %d, want 9090", cfg.GRPCWebPort)
	}
	if cfg.RetentionDays != 7 {
		t.Errorf("RetentionDays: got %d, want 7", cfg.RetentionDays)
	}
	if cfg.RetentionCheckInterval != 12*time.Hour {
		t.Errorf("RetentionCheckInterval: got %v, want 12h", cfg.RetentionCheckInterval)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want debug", cfg.LogLevel)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"0", "99999", "abc", "-1"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("LOGS_ROOT", dir)
			t.Setenv("GRPC_WEB_PORT", v)
			t.Setenv("RETENTION_DAYS", "")
			t.Setenv("RETENTION_CHECK_INTERVAL", "")
			if _, err := Load(); err == nil {
				t.Errorf("expected error for GRPC_WEB_PORT=%q", v)
			}
		})
	}
}

func TestLoad_InvalidRetentionDays(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"0", "-1", "abc"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("LOGS_ROOT", dir)
			t.Setenv("GRPC_WEB_PORT", "")
			t.Setenv("RETENTION_DAYS", v)
			t.Setenv("RETENTION_CHECK_INTERVAL", "")
			if _, err := Load(); err == nil {
				t.Errorf("expected error for RETENTION_DAYS=%q", v)
			}
		})
	}
}

func TestLoad_InvalidRetentionCheckInterval(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"-1h", "notaduration"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("LOGS_ROOT", dir)
			t.Setenv("GRPC_WEB_PORT", "")
			t.Setenv("RETENTION_DAYS", "")
			t.Setenv("RETENTION_CHECK_INTERVAL", v)
			if _, err := Load(); err == nil {
				t.Errorf("expected error for RETENTION_CHECK_INTERVAL=%q", v)
			}
		})
	}
}

func TestLoad_LogsRootCreated(t *testing.T) {
	newDir := t.TempDir() + "/sub/dir"
	t.Setenv("LOGS_ROOT", newDir)
	t.Setenv("GRPC_WEB_PORT", "")
	t.Setenv("RETENTION_DAYS", "")
	t.Setenv("RETENTION_CHECK_INTERVAL", "")

	if _, err := Load(); err != nil {
		t.Fatalf("expected LOGS_ROOT to be created automatically: %v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("expected directory %q to exist: %v", newDir, err)
	}
}
