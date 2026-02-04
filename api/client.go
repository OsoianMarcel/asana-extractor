package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/OsoianMarcel/asana-extractor/limiter"
	"github.com/OsoianMarcel/asana-extractor/models"
)

const (
	asanaBaseURL = "https://app.asana.com/api/1.0"
	pageSize     = 100
)

// Client represents an Asana API client.
type Client struct {
	token      string
	httpClient *http.Client
	limiter    *limiter.Limiter
	logger     *slog.Logger
}

// NewClient creates a new Asana API client.
func NewClient(token string, rateLimiter *limiter.Limiter, logger *slog.Logger) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		limiter: rateLimiter,
		logger:  logger,
	}
}

// waitForRateLimit waits for the rate limiter to allow a GET request.
func (c *Client) waitForRateLimit(ctx context.Context) error {
	if c.limiter == nil {
		return nil
	}
	return c.limiter.WaitForGet(ctx)
}

// releaseRateLimit releases the concurrent request slot.
func (c *Client) releaseRateLimit() {
	if c.limiter == nil {
		return
	}
	c.limiter.ReleaseGet()
}

// saveRateLimiterState persists the rate limiter state to disk.
func (c *Client) saveRateLimiterState() {
	if c.limiter == nil {
		return
	}
	if err := c.limiter.SaveState(); err != nil {
		c.logger.Warn("failed to save rate limiter state", "error", err)
	}
}

// GetWorkspaces fetches all workspaces accessible to the authenticated user.
func (c *Client) GetWorkspaces(ctx context.Context) ([]models.Workspace, error) {
	c.logger.Info("fetching workspaces from Asana API")

	// Wait for rate limit
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	defer c.releaseRateLimit()

	url := fmt.Sprintf("%s/workspaces?limit=%d", asanaBaseURL, pageSize)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Save rate limiter state after each request
	c.saveRateLimiterState()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			Message:    "rate limit exceeded",
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var workspaceResp models.WorkspaceResponse
	err = json.Unmarshal(body, &workspaceResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workspace response: %w", err)
	}

	c.logger.Info("successfully fetched workspaces", "count", len(workspaceResp.Data))
	return workspaceResp.Data, nil
}

// GetUsers fetches all users from the specified workspace.
func (c *Client) GetUsers(ctx context.Context, workspaceID string) ([]models.User, error) {
	c.logger.Info("fetching users from Asana API", "workspace", workspaceID)

	var users []models.User
	url := fmt.Sprintf("%s/users?workspace=%s&limit=%d", asanaBaseURL, workspaceID, pageSize)

	for url != "" {
		data, nextURL, err := c.fetchPage(ctx, url)
		if err != nil {
			c.logger.Error("failed to fetch users page", "error", err, "url", url)
			return nil, err
		}

		for _, item := range data {
			var user models.User
			itemJSON, err := json.Marshal(item)
			if err != nil {
				c.logger.Warn("failed to marshal user item", "error", err)
				continue
			}

			err = json.Unmarshal(itemJSON, &user)
			if err != nil {
				c.logger.Warn("failed to unmarshal user", "error", err)
				continue
			}

			users = append(users, user)
		}

		url = nextURL
	}

	c.logger.Info("successfully fetched users", "count", len(users))
	return users, nil
}

// GetProjects fetches all projects from the specified workspace.
func (c *Client) GetProjects(ctx context.Context, workspaceID string) ([]models.Project, error) {
	c.logger.Info("fetching projects from Asana API", "workspace", workspaceID)

	var projects []models.Project
	url := fmt.Sprintf("%s/projects?workspace=%s&limit=%d", asanaBaseURL, workspaceID, pageSize)

	for url != "" {
		data, nextURL, err := c.fetchPage(ctx, url)
		if err != nil {
			c.logger.Error("failed to fetch projects page", "error", err, "url", url)
			return nil, err
		}

		for _, item := range data {
			var project models.Project
			itemJSON, err := json.Marshal(item)
			if err != nil {
				c.logger.Warn("failed to marshal project item", "error", err)
				continue
			}

			err = json.Unmarshal(itemJSON, &project)
			if err != nil {
				c.logger.Warn("failed to unmarshal project", "error", err)
				continue
			}

			projects = append(projects, project)
		}

		url = nextURL
	}

	c.logger.Info("successfully fetched projects", "count", len(projects))
	return projects, nil
}

// GetTasksForProject fetches all tasks from the specified project.
func (c *Client) GetTasksForProject(ctx context.Context, projectGID string) ([]models.Task, error) {
	c.logger.Debug("fetching tasks for project", "project", projectGID)

	var tasks []models.Task
	url := fmt.Sprintf("%s/projects/%s/tasks?limit=%d", asanaBaseURL, projectGID, pageSize)

	for url != "" {
		data, nextURL, err := c.fetchPageWithTaskFields(ctx, url)
		if err != nil {
			c.logger.Error("failed to fetch tasks page", "error", err, "url", url)
			return nil, err
		}

		for _, item := range data {
			var task models.Task
			itemJSON, err := json.Marshal(item)
			if err != nil {
				c.logger.Warn("failed to marshal task item", "error", err)
				continue
			}

			err = json.Unmarshal(itemJSON, &task)
			if err != nil {
				c.logger.Warn("failed to unmarshal task", "error", err)
				continue
			}

			tasks = append(tasks, task)
		}

		url = nextURL
	}

	c.logger.Debug("successfully fetched tasks for project", "project", projectGID, "count", len(tasks))
	return tasks, nil
}

// fetchPage fetches a single page from the API with pagination support.
// Returns the data, the next page URL (if any), and any error.
func (c *Client) fetchPage(ctx context.Context, url string) ([]map[string]interface{}, string, error) {
	// Wait for rate limit before making request
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, "", err
	}
	defer c.releaseRateLimit()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuthHeader(req)
	c.addOptFields(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Save rate limiter state after each request
	c.saveRateLimiterState()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, "", &RateLimitError{
			RetryAfter: retryAfter,
			Message:    "rate limit exceeded",
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp models.APIResponse
	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	nextURL := ""
	if apiResp.NextPage != nil {
		nextURL = fmt.Sprintf("%s%s", asanaBaseURL, apiResp.NextPage.Path)
	}

	return apiResp.Data, nextURL, nil
}

// fetchPageWithTaskFields fetches a single page from the API with task-specific fields.
func (c *Client) fetchPageWithTaskFields(ctx context.Context, url string) ([]map[string]interface{}, string, error) {
	// Wait for rate limit before making request
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, "", err
	}
	defer c.releaseRateLimit()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuthHeader(req)
	c.addTaskOptFields(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Save rate limiter state after each request
	c.saveRateLimiterState()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, "", &RateLimitError{
			RetryAfter: retryAfter,
			Message:    "rate limit exceeded",
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp models.APIResponse
	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	nextURL := ""
	if apiResp.NextPage != nil {
		nextURL = fmt.Sprintf("%s%s", asanaBaseURL, apiResp.NextPage.Path)
	}

	return apiResp.Data, nextURL, nil
}

// addAuthHeader adds the Bearer token authorization header to the request.
func (c *Client) addAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
}

// addOptFields adds the opt_fields parameter to optimize the response payload.
func (c *Client) addOptFields(req *http.Request) {
	q := req.URL.Query()
	q.Set("opt_fields", "gid,name,email,photo,workspaces,resource_type,created_at,modified_at")
	req.URL.RawQuery = q.Encode()
}

// addTaskOptFields adds the opt_fields parameter for task requests.
func (c *Client) addTaskOptFields(req *http.Request) {
	q := req.URL.Query()
	q.Set("opt_fields", "gid,name,resource_type,resource_subtype,assignee,assignee.name,completed,completed_at,due_on,due_at,start_on,notes,num_subtasks,parent,parent.name,projects,projects.name,tags,tags.name,followers,followers.name,custom_fields,created_at,modified_at,permalink_url")
	req.URL.RawQuery = q.Encode()
}

// parseRetryAfter parses the Retry-After header value.
// It can be either a number of seconds or an HTTP-date.
func parseRetryAfter(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 60 * time.Second
	}

	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := http.ParseTime(retryAfter); err == nil {
		return time.Until(t)
	}

	return 60 * time.Second
}

// RateLimitError represents a rate limit error from the API.
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s (retry after %v)", e.Message, e.RetryAfter)
}

// IsRateLimitError checks if an error is a rate limit error.
func IsRateLimitError(err error) (*RateLimitError, bool) {
	if err == nil {
		return nil, false
	}
	rle, ok := err.(*RateLimitError)
	return rle, ok
}
