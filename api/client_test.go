package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		minTime time.Duration
		maxTime time.Duration
	}{
		{
			name:    "seconds format",
			input:   "30",
			minTime: 29 * time.Second,
			maxTime: 31 * time.Second,
		},
		{
			name:    "empty string defaults to 60",
			input:   "",
			minTime: 59 * time.Second,
			maxTime: 61 * time.Second,
		},
		{
			name:    "invalid format defaults to 60",
			input:   "invalid",
			minTime: 59 * time.Second,
			maxTime: 61 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfter(tt.input)
			if result < tt.minTime || result > tt.maxTime {
				t.Errorf("parseRetryAfter(%q) = %v, want between %v and %v", tt.input, result, tt.minTime, tt.maxTime)
			}
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	rle := &RateLimitError{
		RetryAfter: 30 * time.Second,
		Message:    "rate limited",
	}

	rateLimitErr, ok := IsRateLimitError(rle)
	if !ok {
		t.Errorf("IsRateLimitError should return true for RateLimitError")
	}
	if rateLimitErr.RetryAfter != 30*time.Second {
		t.Errorf("IsRateLimitError returned wrong RetryAfter: %v", rateLimitErr.RetryAfter)
	}

	_, ok = IsRateLimitError(nil)
	if ok {
		t.Errorf("IsRateLimitError should return false for nil")
	}
}

func TestNewClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)

	if client.token != "test-token" {
		t.Errorf("NewClient didn't set token correctly")
	}
	if client.httpClient == nil {
		t.Errorf("NewClient didn't initialize httpClient")
	}
	if client.logger == nil {
		t.Errorf("NewClient didn't set logger")
	}
	if client.baseURL != asanaBaseURL {
		t.Errorf("NewClient didn't set default baseURL")
	}
}

func TestGetWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/workspaces" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or incorrect Authorization header")
		}

		// Return mock response
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"gid": "123", "name": "Workspace 1", "resource_type": "workspace"},
				{"gid": "456", "name": "Workspace 2", "resource_type": "workspace"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)
	client.SetBaseURL(server.URL)

	workspaces, err := client.GetWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("GetWorkspaces failed: %v", err)
	}

	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}
	if workspaces[0].GID != "123" || workspaces[0].Name != "Workspace 1" {
		t.Errorf("unexpected workspace data: %+v", workspaces[0])
	}
}

func TestGetUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("workspace") != "ws123" {
			t.Errorf("missing workspace query param")
		}

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"gid": "user1", "name": "Alice", "email": "alice@example.com"},
				{"gid": "user2", "name": "Bob", "email": "bob@example.com"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)
	client.SetBaseURL(server.URL)

	users, err := client.GetUsers(context.Background(), "ws123")
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
	if users[0].GID != "user1" || users[0].Name != "Alice" {
		t.Errorf("unexpected user data: %+v", users[0])
	}
}

func TestGetProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("workspace") != "ws123" {
			t.Errorf("missing workspace query param")
		}

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"gid": "proj1", "name": "Project Alpha"},
				{"gid": "proj2", "name": "Project Beta"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)
	client.SetBaseURL(server.URL)

	projects, err := client.GetProjects(context.Background(), "ws123")
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}

	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].GID != "proj1" || projects[0].Name != "Project Alpha" {
		t.Errorf("unexpected project data: %+v", projects[0])
	}
}

func TestGetTasksForProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj123/tasks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"gid": "task1", "name": "Task One", "completed": false},
				{"gid": "task2", "name": "Task Two", "completed": true},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)
	client.SetBaseURL(server.URL)

	tasks, err := client.GetTasksForProject(context.Background(), "proj123")
	if err != nil {
		t.Fatalf("GetTasksForProject failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].GID != "task1" || tasks[0].Name != "Task One" {
		t.Errorf("unexpected task data: %+v", tasks[0])
	}
}

func TestRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("test-token", nil, logger)
	client.SetBaseURL(server.URL)

	_, err := client.GetWorkspaces(context.Background())
	if err == nil {
		t.Fatal("expected error for rate limit response")
	}

	rle, ok := IsRateLimitError(err)
	if !ok {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rle.RetryAfter != 30*time.Second {
		t.Errorf("expected RetryAfter 30s, got %v", rle.RetryAfter)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors":[{"message":"Not Authorized"}]}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient("bad-token", nil, logger)
	client.SetBaseURL(server.URL)

	_, err := client.GetWorkspaces(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized response")
	}
}
