package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("PORT", "")
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
	if cfg.Port != 8080 {
		t.Errorf("Port: got %d, want 8080", cfg.Port)
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
	if cfg.DiskHighWaterPercent != 90 {
		t.Errorf("DiskHighWaterPercent: got %d, want 90", cfg.DiskHighWaterPercent)
	}
	if cfg.DiskLowWaterPercent != 80 {
		t.Errorf("DiskLowWaterPercent: got %d, want 80", cfg.DiskLowWaterPercent)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("PORT", "9090")
	t.Setenv("RETENTION_DAYS", "7")
	t.Setenv("RETENTION_CHECK_INTERVAL", "12h")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port: got %d, want 9090", cfg.Port)
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
			t.Setenv("PORT", v)
			t.Setenv("RETENTION_DAYS", "")
			t.Setenv("RETENTION_CHECK_INTERVAL", "")
			if _, err := Load(); err == nil {
				t.Errorf("expected error for PORT=%q", v)
			}
		})
	}
}

func TestLoad_InvalidRetentionDays(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"0", "-1", "abc"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("LOGS_ROOT", dir)
			t.Setenv("PORT", "")
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
			t.Setenv("PORT", "")
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
	t.Setenv("PORT", "")
	t.Setenv("RETENTION_DAYS", "")
	t.Setenv("RETENTION_CHECK_INTERVAL", "")

	if _, err := Load(); err != nil {
		t.Fatalf("expected LOGS_ROOT to be created automatically: %v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("expected directory %q to exist: %v", newDir, err)
	}
}

func TestCollectionMode(t *testing.T) {
	cases := []struct {
		name         string
		nodeLogsRoot string
		nodeName     string
		want         string
	}{
		{"api when no node logs root", "", "", ModeAPI},
		{"api ignores node name without node logs root", "", "node-a", ModeAPI},
		{"fileTail when node logs root only", "/var/log/pods", "", ModeFileTail},
		{"hybrid when node logs root and node name", "/var/log/pods", "node-a", ModeHybrid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{NodeLogsRoot: tc.nodeLogsRoot, NodeName: tc.nodeName}
			if got := cfg.CollectionMode(); got != tc.want {
				t.Errorf("CollectionMode: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_MigrateLegacyDefaultsTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("MIGRATE_LEGACY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.MigrateLegacy {
		t.Error("expected MigrateLegacy to default to true")
	}
}

func TestLoad_MigrateLegacyDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	for _, v := range []string{"false", "0"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("MIGRATE_LEGACY", v)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.MigrateLegacy {
				t.Errorf("expected MigrateLegacy=false for MIGRATE_LEGACY=%q", v)
			}
		})
	}
}

func TestLoad_DiskWaterMarkOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("DISK_HIGH_WATER_PERCENT", "95")
	t.Setenv("DISK_LOW_WATER_PERCENT", "70")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DiskHighWaterPercent != 95 {
		t.Errorf("DiskHighWaterPercent: got %d, want 95", cfg.DiskHighWaterPercent)
	}
	if cfg.DiskLowWaterPercent != 70 {
		t.Errorf("DiskLowWaterPercent: got %d, want 70", cfg.DiskLowWaterPercent)
	}
}

func TestLoad_InvalidDiskWaterMarkPercent(t *testing.T) {
	dir := t.TempDir()
	for _, env := range []string{"DISK_HIGH_WATER_PERCENT", "DISK_LOW_WATER_PERCENT"} {
		for _, v := range []string{"0", "101", "-1", "abc"} {
			t.Run(env+"="+v, func(t *testing.T) {
				t.Setenv("LOGS_ROOT", dir)
				t.Setenv(env, v)
				if _, err := Load(); err == nil {
					t.Errorf("expected error for %s=%q", env, v)
				}
			})
		}
	}
}

func TestLoad_DiskLowWaterMarkMustBeBelowHigh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("DISK_HIGH_WATER_PERCENT", "80")
	t.Setenv("DISK_LOW_WATER_PERCENT", "80")

	if _, err := Load(); err == nil {
		t.Error("expected error when DISK_LOW_WATER_PERCENT equals DISK_HIGH_WATER_PERCENT")
	}

	t.Setenv("DISK_LOW_WATER_PERCENT", "85")
	if _, err := Load(); err == nil {
		t.Error("expected error when DISK_LOW_WATER_PERCENT exceeds DISK_HIGH_WATER_PERCENT")
	}
}

func TestLoad_NodeNameFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOGS_ROOT", dir)
	t.Setenv("PORT", "")
	t.Setenv("RETENTION_DAYS", "")
	t.Setenv("RETENTION_CHECK_INTERVAL", "")
	t.Setenv("NODE_LOGS_ROOT", "/var/log/pods")
	t.Setenv("NODE_NAME", "worker-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NodeName != "worker-1" {
		t.Errorf("NodeName: got %q, want worker-1", cfg.NodeName)
	}
	if got := cfg.CollectionMode(); got != ModeHybrid {
		t.Errorf("CollectionMode: got %q, want %q", got, ModeHybrid)
	}
}

func TestLoad_CORSAndAPIURLDefaultOff(t *testing.T) {
	t.Setenv("LOGS_ROOT", t.TempDir())
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	t.Setenv("API_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Errorf("CORSAllowedOrigins: got %v, want none", cfg.CORSAllowedOrigins)
	}
	if cfg.APIURL != "" {
		t.Errorf("APIURL: got %q, want empty", cfg.APIURL)
	}
}

func TestLoad_CORSAllowedOriginsParsed(t *testing.T) {
	t.Setenv("LOGS_ROOT", t.TempDir())
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://a.example.com, ,https://b.example.com ")
	t.Setenv("API_URL", "https://api.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(cfg.CORSAllowedOrigins) != len(want) || cfg.CORSAllowedOrigins[0] != want[0] || cfg.CORSAllowedOrigins[1] != want[1] {
		t.Errorf("CORSAllowedOrigins: got %v, want %v", cfg.CORSAllowedOrigins, want)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("APIURL: got %q, want https://api.example.com", cfg.APIURL)
	}
}
