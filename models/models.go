package models

import "time"

// User represents an Asana user object from the API.
type User struct {
	GID          string                 `json:"gid"`
	Email        string                 `json:"email"`
	Name         string                 `json:"name"`
	Photo        map[string]interface{} `json:"photo"`
	Workspaces   []WorkspaceRef         `json:"workspaces"`
	ResourceType string                 `json:"resource_type"`
	Created      *time.Time             `json:"created_at"`
	Modified     *time.Time             `json:"modified_at"`
}

// Project represents an Asana project object from the API.
type Project struct {
	GID          string                 `json:"gid"`
	Name         string                 `json:"name"`
	Owner        *UserRef               `json:"owner"`
	Team         *TeamRef               `json:"team"`
	Description  string                 `json:"description"`
	Status       string                 `json:"current_status"`
	Public       bool                   `json:"public"`
	Archived     bool                   `json:"archived"`
	Members      []UserRef              `json:"members"`
	Followers    []UserRef              `json:"followers"`
	CustomFields map[string]interface{} `json:"custom_fields"`
	ResourceType string                 `json:"resource_type"`
	Created      *time.Time             `json:"created_at"`
	Modified     *time.Time             `json:"modified_at"`
}

// UserRef is a reference to a user object (used in nested structures).
type UserRef struct {
	GID   string `json:"gid"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// WorkspaceRef is a reference to a workspace object.
type WorkspaceRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

// Workspace represents a full Asana workspace object.
type Workspace struct {
	GID            string `json:"gid"`
	Name           string `json:"name"`
	ResourceType   string `json:"resource_type"`
	IsOrganization bool   `json:"is_organization"`
}

// TeamRef is a reference to a team object.
type TeamRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

// APIResponse represents a paginated API response from Asana.
type APIResponse struct {
	Data     []map[string]interface{} `json:"data"`
	NextPage *PaginationInfo          `json:"next_page"`
}

// PaginationInfo contains pagination information for API responses.
type PaginationInfo struct {
	Offset int    `json:"offset"`
	Path   string `json:"path"`
	URI    string `json:"uri"`
}

// WorkspaceResponse represents the response from the workspaces endpoint.
type WorkspaceResponse struct {
	Data []Workspace `json:"data"`
}

// Task represents an Asana task object from the API.
type Task struct {
	GID             string                   `json:"gid"`
	Name            string                   `json:"name"`
	ResourceType    string                   `json:"resource_type"`
	ResourceSubtype string                   `json:"resource_subtype"`
	Assignee        *UserRef                 `json:"assignee"`
	Completed       bool                     `json:"completed"`
	CompletedAt     *time.Time               `json:"completed_at"`
	DueOn           string                   `json:"due_on"`
	DueAt           *time.Time               `json:"due_at"`
	StartOn         string                   `json:"start_on"`
	Notes           string                   `json:"notes"`
	NumSubtasks     int                      `json:"num_subtasks"`
	Parent          *TaskRef                 `json:"parent"`
	Projects        []ProjectRef             `json:"projects"`
	Tags            []TagRef                 `json:"tags"`
	Followers       []UserRef                `json:"followers"`
	CustomFields    []map[string]interface{} `json:"custom_fields"`
	Created         *time.Time               `json:"created_at"`
	Modified        *time.Time               `json:"modified_at"`
	PermalinkURL    string                   `json:"permalink_url"`
}

// TaskRef is a reference to a task object (used in nested structures).
type TaskRef struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
}

// ProjectRef is a reference to a project object (used in nested structures).
type ProjectRef struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
}

// TagRef is a reference to a tag object.
type TagRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}
