package scheduler

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/OsoianMarcel/asana-extractor/api"
	"github.com/OsoianMarcel/asana-extractor/limiter"
	"github.com/OsoianMarcel/asana-extractor/models"
	"github.com/OsoianMarcel/asana-extractor/storage"
)

func TestNewScheduler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tmpDir := t.TempDir()
	stor := storage.NewStorage(tmpDir, logger)
	rateLimiter := limiter.NewLimiter(150, "", logger)
	apiClient := api.NewClient("test-token", rateLimiter, logger)
	workspaces := []models.Workspace{
		{GID: "ws1", Name: "Workspace 1"},
	}

	sched := NewScheduler(apiClient, stor, rateLimiter, workspaces, 5, 5*time.Minute, logger)

	if sched.workerCount != 5 {
		t.Errorf("NewScheduler: workerCount = %d, want 5", sched.workerCount)
	}
	if sched.interval != 5*time.Minute {
		t.Errorf("NewScheduler: interval = %v, want 5m", sched.interval)
	}
	if len(sched.workspaces) != 1 {
		t.Errorf("NewScheduler: workspaces len = %d, want 1", len(sched.workspaces))
	}
}

func TestToUserWorkItems(t *testing.T) {
	users := []models.User{
		{GID: "1", Name: "User 1"},
		{GID: "2", Name: "User 2"},
	}

	items := toUserWorkItems("workspace123", users)

	if len(items) != len(users) {
		t.Errorf("toUserWorkItems: len = %d, want %d", len(items), len(users))
	}

	for i, item := range items {
		if item.workspaceGID != "workspace123" {
			t.Errorf("toUserWorkItems: item %d workspaceGID = %s, want workspace123", i, item.workspaceGID)
		}
		user := item.item.(*models.User)
		if user.GID != users[i].GID {
			t.Errorf("toUserWorkItems: item %d GID = %s, want %s", i, user.GID, users[i].GID)
		}
	}
}

func TestToProjectWorkItems(t *testing.T) {
	projects := []models.Project{
		{GID: "proj1", Name: "Project 1"},
		{GID: "proj2", Name: "Project 2"},
	}

	items := toProjectWorkItems("workspace456", projects)

	if len(items) != len(projects) {
		t.Errorf("toProjectWorkItems: len = %d, want %d", len(items), len(projects))
	}

	for i, item := range items {
		if item.workspaceGID != "workspace456" {
			t.Errorf("toProjectWorkItems: item %d workspaceGID = %s, want workspace456", i, item.workspaceGID)
		}
		project := item.item.(*models.Project)
		if project.GID != projects[i].GID {
			t.Errorf("toProjectWorkItems: item %d GID = %s, want %s", i, project.GID, projects[i].GID)
		}
	}
}

func TestSchedulerStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tmpDir := t.TempDir()
	stor := storage.NewStorage(tmpDir, logger)
	rateLimiter := limiter.NewLimiter(150, "", logger)
	apiClient := api.NewClient("test-token", rateLimiter, logger)
	workspaces := []models.Workspace{
		{GID: "ws1", Name: "Workspace 1"},
	}

	sched := NewScheduler(apiClient, stor, rateLimiter, workspaces, 5, 5*time.Minute, logger)

	// Start should succeed
	err := sched.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should complete without panic
	sched.Stop()

	// Verify context is cancelled
	select {
	case <-sched.ctx.Done():
		// Expected
	default:
		t.Errorf("context should be cancelled after Stop()")
	}
}
