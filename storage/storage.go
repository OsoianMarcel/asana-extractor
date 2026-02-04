package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/OsoianMarcel/asana-extractor/models"
)

// Storage handles persisting users and projects to disk.
type Storage struct {
	basePath string
	logger   *slog.Logger
}

// NewStorage creates a new storage instance.
func NewStorage(basePath string, logger *slog.Logger) *Storage {
	return &Storage{
		basePath: basePath,
		logger:   logger,
	}
}

// Initialize creates the base directory for storing data.
func (s *Storage) Initialize() error {
	s.logger.Info("initializing storage", "path", s.basePath)

	if err := os.MkdirAll(s.basePath, 0755); err != nil {
		s.logger.Error("failed to create base directory", "path", s.basePath, "error", err)
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	s.logger.Info("storage initialized successfully")
	return nil
}

// InitializeWorkspace creates necessary directories for a specific workspace.
func (s *Storage) InitializeWorkspace(workspaceGID string) error {
	workspacePath := filepath.Join(s.basePath, workspaceGID)
	s.logger.Info("initializing workspace storage", "workspace", workspaceGID, "path", workspacePath)

	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		s.logger.Error("failed to create workspace directory", "path", workspacePath, "error", err)
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	usersDir := filepath.Join(workspacePath, "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		s.logger.Error("failed to create users directory", "path", usersDir, "error", err)
		return fmt.Errorf("failed to create users directory: %w", err)
	}

	projectsDir := filepath.Join(workspacePath, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		s.logger.Error("failed to create projects directory", "path", projectsDir, "error", err)
		return fmt.Errorf("failed to create projects directory: %w", err)
	}

	tasksDir := filepath.Join(workspacePath, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		s.logger.Error("failed to create tasks directory", "path", tasksDir, "error", err)
		return fmt.Errorf("failed to create tasks directory: %w", err)
	}

	s.logger.Info("workspace storage initialized", "workspace", workspaceGID, "users_dir", usersDir, "projects_dir", projectsDir, "tasks_dir", tasksDir)
	return nil
}

// SaveUser saves a user to a JSON file within a workspace directory.
func (s *Storage) SaveUser(workspaceGID string, user *models.User) error {
	if user.GID == "" {
		s.logger.Warn("user has empty GID, skipping save")
		return nil
	}

	filePath := filepath.Join(s.basePath, workspaceGID, "users", fmt.Sprintf("user_%s.json", user.GID))
	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		s.logger.Error("failed to marshal user", "gid", user.GID, "error", err)
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		s.logger.Error("failed to write user file", "path", filePath, "error", err)
		return fmt.Errorf("failed to write user file: %w", err)
	}

	s.logger.Debug("user saved", "workspace", workspaceGID, "gid", user.GID, "file", filePath)
	return nil
}

// SaveProject saves a project to a JSON file within a workspace directory.
func (s *Storage) SaveProject(workspaceGID string, project *models.Project) error {
	if project.GID == "" {
		s.logger.Warn("project has empty GID, skipping save")
		return nil
	}

	filePath := filepath.Join(s.basePath, workspaceGID, "projects", fmt.Sprintf("project_%s.json", project.GID))
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		s.logger.Error("failed to marshal project", "gid", project.GID, "error", err)
		return fmt.Errorf("failed to marshal project: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		s.logger.Error("failed to write project file", "path", filePath, "error", err)
		return fmt.Errorf("failed to write project file: %w", err)
	}

	s.logger.Debug("project saved", "workspace", workspaceGID, "gid", project.GID, "file", filePath)
	return nil
}

// SaveUsers saves multiple users to JSON files within a workspace directory.
func (s *Storage) SaveUsers(workspaceGID string, users []models.User) error {
	s.logger.Info("saving users", "workspace", workspaceGID, "count", len(users))

	saved := 0
	failed := 0

	for i := range users {
		if err := s.SaveUser(workspaceGID, &users[i]); err != nil {
			failed++
			s.logger.Warn("failed to save user", "workspace", workspaceGID, "index", i, "gid", users[i].GID, "error", err)
		} else {
			saved++
		}
	}

	s.logger.Info("users save completed", "workspace", workspaceGID, "saved", saved, "failed", failed)
	return nil
}

// SaveProjects saves multiple projects to JSON files within a workspace directory.
func (s *Storage) SaveProjects(workspaceGID string, projects []models.Project) error {
	s.logger.Info("saving projects", "workspace", workspaceGID, "count", len(projects))

	saved := 0
	failed := 0

	for i := range projects {
		if err := s.SaveProject(workspaceGID, &projects[i]); err != nil {
			failed++
			s.logger.Warn("failed to save project", "workspace", workspaceGID, "index", i, "gid", projects[i].GID, "error", err)
		} else {
			saved++
		}
	}

	s.logger.Info("projects save completed", "workspace", workspaceGID, "saved", saved, "failed", failed)
	return nil
}

// SaveTask saves a task to a JSON file within a workspace directory.
func (s *Storage) SaveTask(workspaceGID string, task *models.Task) error {
	if task.GID == "" {
		s.logger.Warn("task has empty GID, skipping save")
		return nil
	}

	filePath := filepath.Join(s.basePath, workspaceGID, "tasks", fmt.Sprintf("task_%s.json", task.GID))
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		s.logger.Error("failed to marshal task", "gid", task.GID, "error", err)
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		s.logger.Error("failed to write task file", "path", filePath, "error", err)
		return fmt.Errorf("failed to write task file: %w", err)
	}

	s.logger.Debug("task saved", "workspace", workspaceGID, "gid", task.GID, "file", filePath)
	return nil
}

// SaveTasks saves multiple tasks to JSON files within a workspace directory.
func (s *Storage) SaveTasks(workspaceGID string, tasks []models.Task) error {
	s.logger.Info("saving tasks", "workspace", workspaceGID, "count", len(tasks))

	saved := 0
	failed := 0

	for i := range tasks {
		if err := s.SaveTask(workspaceGID, &tasks[i]); err != nil {
			failed++
			s.logger.Warn("failed to save task", "workspace", workspaceGID, "index", i, "gid", tasks[i].GID, "error", err)
		} else {
			saved++
		}
	}

	s.logger.Info("tasks save completed", "workspace", workspaceGID, "saved", saved, "failed", failed)
	return nil
}
