package storage

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/OsoianMarcel/asana-extractor/models"
)

func TestStorageInitialize(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Check that base directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("base directory not created")
	}
}

func TestStorageInitializeWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	workspaceGID := "workspace123"
	err = s.InitializeWorkspace(workspaceGID)
	if err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}

	// Check that workspace directories were created
	workspaceDir := filepath.Join(tmpDir, workspaceGID)
	usersDir := filepath.Join(workspaceDir, "users")
	projectsDir := filepath.Join(workspaceDir, "projects")

	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		t.Errorf("workspace directory not created")
	}

	if _, err := os.Stat(usersDir); os.IsNotExist(err) {
		t.Errorf("users directory not created")
	}

	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		t.Errorf("projects directory not created")
	}
}

func TestStorageSaveUser(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	workspaceGID := "workspace123"
	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	err = s.InitializeWorkspace(workspaceGID)
	if err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}

	user := &models.User{
		GID:   "12345",
		Name:  "Test User",
		Email: "test@example.com",
	}

	err = s.SaveUser(workspaceGID, user)
	if err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	// Check file was created
	expectedFile := filepath.Join(tmpDir, workspaceGID, "users", "user_12345.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("user file not created at %s", expectedFile)
	}

	// Check file contents
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("user file is empty")
	}

	// Verify it's valid JSON by checking for expected content
	if !contains(string(data), "12345") {
		t.Errorf("file doesn't contain expected GID")
	}
}

func TestStorageSaveProject(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	workspaceGID := "workspace123"
	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	err = s.InitializeWorkspace(workspaceGID)
	if err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}

	project := &models.Project{
		GID:  "99999",
		Name: "Test Project",
	}

	err = s.SaveProject(workspaceGID, project)
	if err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}

	// Check file was created
	expectedFile := filepath.Join(tmpDir, workspaceGID, "projects", "project_99999.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("project file not created at %s", expectedFile)
	}
}

func TestStorageSaveUsers(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	workspaceGID := "workspace123"
	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	err = s.InitializeWorkspace(workspaceGID)
	if err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}

	users := []models.User{
		{GID: "1", Name: "User 1", Email: "user1@example.com"},
		{GID: "2", Name: "User 2", Email: "user2@example.com"},
		{GID: "3", Name: "User 3", Email: "user3@example.com"},
	}

	err = s.SaveUsers(workspaceGID, users)
	if err != nil {
		t.Fatalf("SaveUsers failed: %v", err)
	}

	// Check all files were created
	for _, user := range users {
		expectedFile := filepath.Join(tmpDir, workspaceGID, "users", "user_"+user.GID+".json")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("user file not created for GID %s", user.GID)
		}
	}
}

func TestStorageSaveProjects(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewStorage(tmpDir, logger)

	workspaceGID := "workspace123"
	err := s.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	err = s.InitializeWorkspace(workspaceGID)
	if err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}

	projects := []models.Project{
		{GID: "proj1", Name: "Project 1"},
		{GID: "proj2", Name: "Project 2"},
	}

	err = s.SaveProjects(workspaceGID, projects)
	if err != nil {
		t.Fatalf("SaveProjects failed: %v", err)
	}

	// Check all files were created
	for _, project := range projects {
		expectedFile := filepath.Join(tmpDir, workspaceGID, "projects", "project_"+project.GID+".json")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("project file not created for GID %s", project.GID)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
