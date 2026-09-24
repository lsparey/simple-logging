package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for simple-logging.
type Config struct {
	// LogsRoot is the root directory where pod log files are stored (PVC mount path).
	LogsRoot string

	// Port is the single HTTP port serving the UI, the API, /download,
	// /healthz and /readyz.
	Port int

	// APIURL is the API base URL handed to the frontend via /config.js.
	// Empty (the default) means the frontend's own origin, which is right
	// whenever the UI is served by this binary.
	APIURL string

	// CORSAllowedOrigins lists the browser origins allowed to call the API
	// cross-origin, parsed from the comma-separated CORS_ALLOWED_ORIGINS.
	// Empty (the default) disables CORS.
	CORSAllowedOrigins []string

	// RetentionDays is how many whole UTC days of log segments are kept
	// before today's; older days' segments are deleted.
	RetentionDays int

	// RetentionCheckInterval is how often the retention manager runs.
	RetentionCheckInterval time.Duration

	// LogLevel controls the application's structured log verbosity.
	LogLevel string

	// MigrateLegacy controls whether the v0.11 single-file-per-pod log layout
	// is migrated to the segmented layout at startup. Defaults to true; set
	// MIGRATE_LEGACY=false to leave legacy files untouched (they are then
	// ignored by the API and swept by mtime, as before Phase 1).
	MigrateLegacy bool

	// PPROFPort is the port for the Go pprof HTTP server (localhost only).
	// 0 means disabled (the default). Set PPROF_PORT to enable.
	PPROFPort int

	// NodeLogsRoot is the host path where the CRI writes pod log files,
	// typically /var/log/pods when mounted as a hostPath volume.
	// When set, the collector tails files directly (no Kubernetes log API).
	NodeLogsRoot string

	// NodeName is the name of the Kubernetes node this instance is scheduled
	// on, normally injected via the Downward API (spec.nodeName). When set
	// together with NodeLogsRoot the collector runs in hybrid mode: pods on
	// this node are tailed from the host filesystem and pods on every other
	// node are streamed through the Kubernetes log API, so a single replica
	// can cover a multi-node cluster.
	NodeName string

	// DiskHighWaterPercent is the LOGS_ROOT usage percentage at or above which
	// the disk guard starts deleting the globally oldest log segments. This is
	// a safety net for when retention alone doesn't keep up; it should rarely
	// trigger in normal operation.
	DiskHighWaterPercent int

	// DiskLowWaterPercent is the LOGS_ROOT usage percentage the disk guard
	// deletes segments down to once triggered by DiskHighWaterPercent.
	DiskLowWaterPercent int

	// MetricsEnabled serves Prometheus metrics at /metrics. Off by default;
	// set METRICS_ENABLED=true. The counters are kept either way, since the
	// GetStats RPC reports them to the UI.
	MetricsEnabled bool

	// AuthHTPasswdFile is the path to an htpasswd file of bcrypt hashes.
	// When set, every request except /healthz, /readyz and /metrics needs
	// HTTP basic credentials from it. Empty (the default) disables the
	// built-in auth.
	AuthHTPasswdFile string
}

// Collection modes reported by Config.CollectionMode.
const (
	// ModeAPI streams every pod through the Kubernetes log API.
	ModeAPI = "api"
	// ModeFileTail tails every pod from the node filesystem (single node only).
	ModeFileTail = "fileTail"
	// ModeHybrid tails local pods from the filesystem and streams remote pods
	// through the Kubernetes log API.
	ModeHybrid = "hybrid"
)

// CollectionMode reports how pod logs are collected, derived from
// NodeLogsRoot and NodeName.
func (c *Config) CollectionMode() string {
	switch {
	case c.NodeLogsRoot == "":
		return ModeAPI
	case c.NodeName == "":
		return ModeFileTail
	default:
		return ModeHybrid
	}
}

// Load reads configuration from environment variables, applying defaults where
// a variable is not set, and returns an error if any value is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		LogsRoot:               getEnv("LOGS_ROOT", "/var/pod-logs"),
		Port:                   8080,
		APIURL:                 os.Getenv("API_URL"),
		RetentionDays:          30,
		RetentionCheckInterval: 24 * time.Hour,
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		MigrateLegacy:          true,
		DiskHighWaterPercent:   90,
		DiskLowWaterPercent:    80,
	}

	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid PORT %q: must be an integer between 1 and 65535", raw)
		}
		cfg.Port = port
	}

	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, origin)
		}
	}

	if raw := os.Getenv("RETENTION_DAYS"); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 {
			return nil, fmt.Errorf("invalid RETENTION_DAYS %q: must be a positive integer", raw)
		}
		cfg.RetentionDays = days
	}

	if raw := os.Getenv("RETENTION_CHECK_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid RETENTION_CHECK_INTERVAL %q: must be a positive duration (e.g. 24h)", raw)
		}
		cfg.RetentionCheckInterval = d
	}

	if raw := os.Getenv("MIGRATE_LEGACY"); raw == "false" || raw == "0" {
		cfg.MigrateLegacy = false
	}

	if raw := os.Getenv("PPROF_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid PPROF_PORT %q: must be an integer between 1 and 65535", raw)
		}
		cfg.PPROFPort = port
	}

	if raw := os.Getenv("METRICS_ENABLED"); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid METRICS_ENABLED %q: must be true or false", raw)
		}
		cfg.MetricsEnabled = enabled
	}

	cfg.AuthHTPasswdFile = os.Getenv("AUTH_HTPASSWD_FILE")

	if raw := os.Getenv("NODE_LOGS_ROOT"); raw != "" {
		cfg.NodeLogsRoot = raw
	}

	if raw := os.Getenv("NODE_NAME"); raw != "" {
		cfg.NodeName = raw
	}

	if raw := os.Getenv("DISK_HIGH_WATER_PERCENT"); raw != "" {
		percent, err := strconv.Atoi(raw)
		if err != nil || percent < 1 || percent > 100 {
			return nil, fmt.Errorf("invalid DISK_HIGH_WATER_PERCENT %q: must be an integer between 1 and 100", raw)
		}
		cfg.DiskHighWaterPercent = percent
	}

	if raw := os.Getenv("DISK_LOW_WATER_PERCENT"); raw != "" {
		percent, err := strconv.Atoi(raw)
		if err != nil || percent < 1 || percent > 100 {
			return nil, fmt.Errorf("invalid DISK_LOW_WATER_PERCENT %q: must be an integer between 1 and 100", raw)
		}
		cfg.DiskLowWaterPercent = percent
	}

	if cfg.DiskLowWaterPercent >= cfg.DiskHighWaterPercent {
		return nil, fmt.Errorf("DISK_LOW_WATER_PERCENT (%d) must be lower than DISK_HIGH_WATER_PERCENT (%d)",
			cfg.DiskLowWaterPercent, cfg.DiskHighWaterPercent)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	info, err := os.Stat(c.LogsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			// Attempt to create the directory so the app can start even if the
			// PVC is pre-provisioned but the sub-path hasn't been initialised.
			if mkErr := os.MkdirAll(c.LogsRoot, 0755); mkErr != nil {
				return fmt.Errorf("LOGS_ROOT %q does not exist and could not be created: %w", c.LogsRoot, mkErr)
			}
			return nil
		}
		return fmt.Errorf("cannot stat LOGS_ROOT %q: %w", c.LogsRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("LOGS_ROOT %q exists but is not a directory", c.LogsRoot)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
