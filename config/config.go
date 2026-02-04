package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config represents the application configuration.
type Config struct {
	AsanaToken         string
	ExtractionInterval time.Duration
	OutputDir          string
	WorkerCount        int
	LogLevel           slog.Level
}

// Load loads configuration from environment variables and CLI flags.
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg := &Config{}

	// Parse CLI flags
	interval := flag.String("interval", "", "Extraction interval (5m or 30s)")
	outputDir := flag.String("output-dir", "", "Output directory for JSON files")
	workerCount := flag.Int("workers", 0, "Number of concurrent workers (0 = auto-calculate)")
	logLevel := flag.String("log-level", "", "Logging level (debug, info, warn, error)")
	flag.Parse()

	// Load Asana token from environment (required)
	cfg.AsanaToken = os.Getenv("ASANA_TOKEN")
	if cfg.AsanaToken == "" {
		return nil, fmt.Errorf("ASANA_TOKEN environment variable is required")
	}

	// Load extraction interval
	intervalStr := *interval
	if intervalStr == "" {
		intervalStr = os.Getenv("EXTRACTION_INTERVAL")
		if intervalStr == "" {
			intervalStr = "5m"
		}
	}
	duration, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid extraction interval %q: %w", intervalStr, err)
	}
	// Validate interval is either 5m or 30s
	if duration != 5*time.Minute && duration != 30*time.Second {
		return nil, fmt.Errorf("extraction interval must be 5m or 30s, got %v", duration)
	}
	cfg.ExtractionInterval = duration

	// Load output directory
	cfg.OutputDir = *outputDir
	if cfg.OutputDir == "" {
		cfg.OutputDir = os.Getenv("OUTPUT_DIR")
		if cfg.OutputDir == "" {
			cfg.OutputDir = "./data"
		}
	}

	// Load worker count
	cfg.WorkerCount = *workerCount
	if cfg.WorkerCount <= 0 {
		workerCountStr := os.Getenv("WORKER_COUNT")
		if workerCountStr != "" {
			// Parse from env
			_, err := fmt.Sscanf(workerCountStr, "%d", &cfg.WorkerCount)
			if err != nil || cfg.WorkerCount <= 0 {
				// Use default auto-calculated value
				cfg.WorkerCount = calculateDefaultWorkerCount()
			}
		} else {
			cfg.WorkerCount = calculateDefaultWorkerCount()
		}
	}

	// Load log level
	logLevelStr := *logLevel
	if logLevelStr == "" {
		logLevelStr = os.Getenv("LOG_LEVEL")
		if logLevelStr == "" {
			logLevelStr = "info"
		}
	}
	err = cfg.LogLevel.UnmarshalText([]byte(logLevelStr))
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", logLevelStr, err)
	}

	return cfg, nil
}

// calculateDefaultWorkerCount calculates the default worker pool size based on
// API rate limits. The logic assumes one API request per item extraction.
// Asana allows 50 concurrent GET requests, so we use a conservative default.
func calculateDefaultWorkerCount() int {
	// Conservative default: 30 workers (well under 50 concurrent GET limit)
	// This leaves headroom for pagination and other requests
	return 30
}
