package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for simple-logging.
type Config struct {
	// LogsRoot is the root directory where pod log files are stored (PVC mount path).
	LogsRoot string

	// GRPCWebPort is the port the gRPC-Web HTTP server listens on.
	GRPCWebPort int

	// RetentionDays is how many days a log file is kept after its last write.
	RetentionDays int

	// RetentionCheckInterval is how often the retention manager runs.
	RetentionCheckInterval time.Duration

	// LogLevel controls the application's structured log verbosity.
	LogLevel string

	// RESTDebugEnabled enables plain JSON REST endpoints at /debug/* for
	// testing purposes. Defaults to true; set REST_DEBUG=false to disable.
	RESTDebugEnabled bool

	// PPROFPort is the port for the Go pprof HTTP server (localhost only).
	// 0 means disabled (the default). Set PPROF_PORT to enable.
	PPROFPort int

	// NodeLogsRoot is the host path where the CRI writes pod log files,
	// typically /var/log/pods when mounted as a hostPath volume.
	// When set, the collector tails files directly (no Kubernetes log API).
	NodeLogsRoot string
}

// Load reads configuration from environment variables, applying defaults where
// a variable is not set, and returns an error if any value is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		LogsRoot:               getEnv("LOGS_ROOT", "/var/pod-logs"),
		GRPCWebPort:            8080,
		RetentionDays:          30,
		RetentionCheckInterval: 24 * time.Hour,
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		RESTDebugEnabled:       true, // enabled by default; set REST_DEBUG=false to disable
	}

	if raw := os.Getenv("GRPC_WEB_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid GRPC_WEB_PORT %q: must be an integer between 1 and 65535", raw)
		}
		cfg.GRPCWebPort = port
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

	if raw := os.Getenv("REST_DEBUG"); raw == "false" || raw == "0" {
		cfg.RESTDebugEnabled = false
	}

	if raw := os.Getenv("PPROF_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid PPROF_PORT %q: must be an integer between 1 and 65535", raw)
		}
		cfg.PPROFPort = port
	}

	if raw := os.Getenv("NODE_LOGS_ROOT"); raw != "" {
		cfg.NodeLogsRoot = raw
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
