package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/OsoianMarcel/asana-extractor/api"
	"github.com/OsoianMarcel/asana-extractor/config"
	"github.com/OsoianMarcel/asana-extractor/limiter"
	"github.com/OsoianMarcel/asana-extractor/scheduler"
	"github.com/OsoianMarcel/asana-extractor/storage"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("asana-extractor starting",
		"interval", cfg.ExtractionInterval,
		"output_dir", cfg.OutputDir,
		"workers", cfg.WorkerCount,
		"log_level", cfg.LogLevel,
	)

	// Create rate limiter with state persistence
	// Using 150 as default (free tier). TODO: Make this configurable
	rateLimiterStatePath := filepath.Join(cfg.OutputDir, "rate_limiter_state.json")
	rateLimiter := limiter.NewLimiter(150, rateLimiterStatePath, logger)

	// Create API client with rate limiter
	apiClient := api.NewClient(cfg.AsanaToken, rateLimiter, logger)

	// Fetch all workspaces
	ctx := context.Background()
	workspaces, err := apiClient.GetWorkspaces(ctx)
	if err != nil {
		logger.Error("failed to fetch workspaces", "error", err)
		os.Exit(1)
	}

	if len(workspaces) == 0 {
		logger.Error("no workspaces found for the authenticated user")
		os.Exit(1)
	}

	logger.Info("found workspaces", "count", len(workspaces))
	for _, ws := range workspaces {
		logger.Info("workspace", "gid", ws.GID, "name", ws.Name)
	}

	// Initialize storage
	stor := storage.NewStorage(cfg.OutputDir, logger)
	if err := stor.Initialize(); err != nil {
		logger.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}

	// Initialize storage directories for each workspace
	for _, ws := range workspaces {
		if err := stor.InitializeWorkspace(ws.GID); err != nil {
			logger.Error("failed to initialize workspace storage", "workspace", ws.GID, "error", err)
			os.Exit(1)
		}
	}

	// Create scheduler
	sched := scheduler.NewScheduler(apiClient, stor, rateLimiter, workspaces, cfg.WorkerCount, cfg.ExtractionInterval, logger)

	// Start scheduler
	if err := sched.Start(); err != nil {
		logger.Error("failed to start scheduler", "error", err)
		os.Exit(1)
	}

	logger.Info("scheduler started successfully")

	// Set up signal handlers for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	logger.Info("received signal, starting graceful shutdown", "signal", sig)

	// Stop scheduler
	sched.Stop()

	logger.Info("asana-extractor stopped gracefully")
}
