package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// Client is an HTTP client wrapper for communicating with the daemon.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a new Client targeting the given daemon host.
func NewClient(host string) *Client {
	return &Client{
		baseURL: host,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Do executes an HTTP request to the daemon and returns the response.
// If body is non-nil it is JSON-encoded.
func (c *Client) Do(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("daemon not running at %s; start with: bor daemon start", c.baseURL)
		}
		return nil, fmt.Errorf("request failed (is the daemon running?): %w", err)
	}
	return resp, nil
}

// decodeOrError reads the response body. If the status is not in the 2xx range
// it tries to parse an error message from the JSON body.
func decodeOrError(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, string(data))
	}

	if v != nil {
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// CreateRepo registers a new repository with the daemon.
func (c *Client) CreateRepo(owner, name string) error {
	body := map[string]string{
		"owner": owner,
		"name":  name,
	}
	resp, err := c.Do("POST", "/repos", body)
	if err != nil {
		return err
	}
	return decodeOrError(resp, nil)
}

// ListRepos returns all registered repositories.
func (c *Client) ListRepos() ([]*model.RepoConfig, error) {
	resp, err := c.Do("GET", "/repos", nil)
	if err != nil {
		return nil, err
	}
	var repos []*model.RepoConfig
	if err := decodeOrError(resp, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// CreateIssueRequest holds the parameters for creating an issue.
type CreateIssueRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	IssueType   string   `json:"issue_type,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// CreateIssue creates a new issue in the given repo.
func (c *Client) CreateIssue(repo string, req CreateIssueRequest) (*model.Issue, error) {
	path := "/issues"
	if repo != "" {
		path += "?repo=" + repo
	}
	resp, err := c.Do("POST", path, req)
	if err != nil {
		return nil, err
	}
	var issue model.Issue
	if err := decodeOrError(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// ListOpts holds query parameters for listing issues.
type ListOpts struct {
	Status   string
	Priority string
	All      bool
}

// ListIssues returns issues for the given repo, filtered by opts.
func (c *Client) ListIssues(repo string, opts ListOpts) ([]*model.Issue, error) {
	path := "/issues?"
	params := ""
	if repo != "" {
		params += "repo=" + repo + "&"
	}
	if opts.Status != "" {
		params += "status=" + opts.Status + "&"
	}
	if opts.Priority != "" {
		params += "priority=" + opts.Priority + "&"
	}
	if opts.All {
		params += "all=true&"
	}
	path += params

	resp, err := c.Do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var issues []*model.Issue
	if err := decodeOrError(resp, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// GetIssue retrieves a single issue by ID.
func (c *Client) GetIssue(id int) (*model.Issue, error) {
	path := fmt.Sprintf("/issues/%d", id)
	resp, err := c.Do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var issue model.Issue
	if err := decodeOrError(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// UpdateIssue updates fields on an existing issue.
func (c *Client) UpdateIssue(id int, fields map[string]interface{}) (*model.Issue, error) {
	path := fmt.Sprintf("/issues/%d", id)
	resp, err := c.Do("PATCH", path, fields)
	if err != nil {
		return nil, err
	}
	var issue model.Issue
	if err := decodeOrError(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// DeleteIssue soft-deletes an issue by ID.
func (c *Client) DeleteIssue(id int) error {
	path := fmt.Sprintf("/issues/%d", id)
	resp, err := c.Do("DELETE", path, nil)
	if err != nil {
		return err
	}
	return decodeOrError(resp, nil)
}

// AssignIssue assigns an issue to the given owner.
func (c *Client) AssignIssue(id int, owner string) (*model.Issue, error) {
	path := fmt.Sprintf("/issues/%d/assign", id)
	body := map[string]string{"owner": owner}
	resp, err := c.Do("POST", path, body)
	if err != nil {
		return nil, err
	}
	var issue model.Issue
	if err := decodeOrError(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// NextIssue retrieves the highest-priority open issue for the given repo.
func (c *Client) NextIssue(repo string) (*model.Issue, error) {
	path := "/issues/next"
	if repo != "" {
		path += "?repo=" + repo
	}
	resp, err := c.Do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var issue model.Issue
	if err := decodeOrError(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// Health pings the daemon health endpoint.
func (c *Client) Health() (map[string]interface{}, error) {
	resp, err := c.Do("GET", "/health", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := decodeOrError(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ForceSync triggers a sync for the given repo.
func (c *Client) ForceSync(repo string) error {
	path := "/sync"
	if repo != "" {
		path += "?repo=" + repo
	}
	resp, err := c.Do("POST", path, nil)
	if err != nil {
		return err
	}
	return decodeOrError(resp, nil)
}
