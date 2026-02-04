package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/OsoianMarcel/asana-extractor/api"
	"github.com/OsoianMarcel/asana-extractor/limiter"
	"github.com/OsoianMarcel/asana-extractor/models"
	"github.com/OsoianMarcel/asana-extractor/storage"
)

// workItem represents a single item to be saved with its workspace context.
type workItem struct {
	workspaceGID string
	item         interface{}
}

// Scheduler manages the extraction process with a worker pool.
type Scheduler struct {
	apiClient   *api.Client
	storage     *storage.Storage
	limiter     *limiter.Limiter
	workspaces  []models.Workspace
	workerCount int
	interval    time.Duration
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	done        chan struct{}
}

// NewScheduler creates a new scheduler.
func NewScheduler(
	apiClient *api.Client,
	stor *storage.Storage,
	rateLimiter *limiter.Limiter,
	workspaces []models.Workspace,
	workerCount int,
	interval time.Duration,
	logger *slog.Logger,
) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		apiClient:   apiClient,
		storage:     stor,
		limiter:     rateLimiter,
		workspaces:  workspaces,
		workerCount: workerCount,
		interval:    interval,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
}

// Start starts the scheduler and begins periodic extraction.
func (s *Scheduler) Start() error {
	s.logger.Info("starting scheduler", "interval", s.interval, "workers", s.workerCount, "workspaces", len(s.workspaces))

	s.extract()

	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop gracefully stops the scheduler and waits for in-flight operations to complete.
func (s *Scheduler) Stop() {
	s.logger.Info("stopping scheduler")
	s.cancel()

	s.wg.Wait()

	s.logger.Info("scheduler stopped")
}

// run manages the periodic extraction loop.
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.extract()
		case <-s.ctx.Done():
			s.logger.Info("scheduler context cancelled, shutting down")
			close(s.done)
			return
		}
	}
}

// extract performs a single extraction cycle for users and projects across all workspaces.
func (s *Scheduler) extract() {
	s.logger.Info("starting extraction cycle", "workspaces", len(s.workspaces))
	startTime := time.Now()

	for _, workspace := range s.workspaces {
		s.extractWorkspace(workspace)
	}

	elapsed := time.Since(startTime)
	s.logger.Info("extraction cycle completed", "duration", elapsed)
}

// extractWorkspace extracts users and projects from a single workspace.
func (s *Scheduler) extractWorkspace(workspace models.Workspace) {
	s.logger.Info("extracting workspace", "workspace_gid", workspace.GID, "workspace_name", workspace.Name)

	users, err := s.apiClient.GetUsers(s.ctx, workspace.GID)
	if err != nil {
		s.logger.Error("failed to fetch users", "workspace", workspace.GID, "error", err)
	} else {
		s.saveItemsWithWorkerPool(toUserWorkItems(workspace.GID, users), "user")
	}

	projects, err := s.apiClient.GetProjects(s.ctx, workspace.GID)
	if err != nil {
		s.logger.Error("failed to fetch projects", "workspace", workspace.GID, "error", err)
	} else {
		s.saveItemsWithWorkerPool(toProjectWorkItems(workspace.GID, projects), "project")

		// Extract tasks for each project
		s.extractTasksForProjects(workspace.GID, projects)
	}

	s.logger.Info("workspace extraction completed", "workspace_gid", workspace.GID)
}

// extractTasksForProjects extracts tasks from all projects in a workspace.
func (s *Scheduler) extractTasksForProjects(workspaceGID string, projects []models.Project) {
	s.logger.Info("extracting tasks for projects", "workspace", workspaceGID, "project_count", len(projects))

	var allTasks []workItem

	for _, project := range projects {
		select {
		case <-s.ctx.Done():
			s.logger.Info("task extraction cancelled")
			return
		default:
		}

		tasks, err := s.apiClient.GetTasksForProject(s.ctx, project.GID)
		if err != nil {
			s.logger.Error("failed to fetch tasks for project", "workspace", workspaceGID, "project", project.GID, "error", err)
			continue
		}

		for i := range tasks {
			allTasks = append(allTasks, workItem{
				workspaceGID: workspaceGID,
				item:         &tasks[i],
			})
		}
	}

	if len(allTasks) > 0 {
		s.saveItemsWithWorkerPool(allTasks, "task")
	}

	s.logger.Info("tasks extraction completed", "workspace", workspaceGID, "total_tasks", len(allTasks))
}

// saveItemsWithWorkerPool saves items using a worker pool for concurrency.
func (s *Scheduler) saveItemsWithWorkerPool(items []workItem, itemType string) {
	if len(items) == 0 {
		s.logger.Info("no items to save", "type", itemType)
		return
	}

	s.logger.Info("starting save with worker pool", "type", itemType, "count", len(items), "workers", s.workerCount)

	workChan := make(chan workItem, len(items))
	var wg sync.WaitGroup

	for i := 0; i < s.workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			s.worker(workerID, workChan, itemType)
		}(i)
	}

	for _, item := range items {
		workChan <- item
	}
	close(workChan)

	wg.Wait()
	s.logger.Info("save completed", "type", itemType)
}

// worker processes items from the work channel.
func (s *Scheduler) worker(id int, workChan <-chan workItem, itemType string) {
	for wi := range workChan {
		select {
		case <-s.ctx.Done():
			s.logger.Debug("worker stopping due to context cancellation", "worker_id", id, "type", itemType)
			return
		default:
		}

		switch itemType {
		case "user":
			user := wi.item.(*models.User)
			if err := s.limiter.WaitForGet(s.ctx); err != nil {
				s.logger.Error("rate limiter wait failed", "worker_id", id, "error", err)
				continue
			}
			defer s.limiter.ReleaseGet()

			if err := s.storage.SaveUser(wi.workspaceGID, user); err != nil {
				s.logger.Error("failed to save user", "worker_id", id, "workspace", wi.workspaceGID, "gid", user.GID, "error", err)
			}

		case "project":
			project := wi.item.(*models.Project)
			if err := s.limiter.WaitForGet(s.ctx); err != nil {
				s.logger.Error("rate limiter wait failed", "worker_id", id, "error", err)
				continue
			}
			defer s.limiter.ReleaseGet()

			if err := s.storage.SaveProject(wi.workspaceGID, project); err != nil {
				s.logger.Error("failed to save project", "worker_id", id, "workspace", wi.workspaceGID, "gid", project.GID, "error", err)
			}

		case "task":
			task := wi.item.(*models.Task)
			if err := s.limiter.WaitForGet(s.ctx); err != nil {
				s.logger.Error("rate limiter wait failed", "worker_id", id, "error", err)
				continue
			}
			defer s.limiter.ReleaseGet()

			if err := s.storage.SaveTask(wi.workspaceGID, task); err != nil {
				s.logger.Error("failed to save task", "worker_id", id, "workspace", wi.workspaceGID, "gid", task.GID, "error", err)
			}
		}
	}
}

// toUserWorkItems converts a slice of users to a slice of work items.
func toUserWorkItems(workspaceGID string, users []models.User) []workItem {
	items := make([]workItem, len(users))
	for i := range users {
		items[i] = workItem{
			workspaceGID: workspaceGID,
			item:         &users[i],
		}
	}
	return items
}

// toProjectWorkItems converts a slice of projects to a slice of work items.
func toProjectWorkItems(workspaceGID string, projects []models.Project) []workItem {
	items := make([]workItem, len(projects))
	for i := range projects {
		items[i] = workItem{
			workspaceGID: workspaceGID,
			item:         &projects[i],
		}
	}
	return items
}
