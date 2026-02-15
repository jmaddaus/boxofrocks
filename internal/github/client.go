package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	userAgent      = "agent-tracker/1.0"
	acceptHeader   = "application/vnd.github+json"
)

// GitHubIssue represents a GitHub issue from the REST API.
type GitHubIssue struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     string        `json:"state"`
	Labels    []GitHubLabel `json:"labels"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// GitHubLabel represents a label on a GitHub issue.
type GitHubLabel struct {
	Name string `json:"name"`
}

// GitHubComment represents a comment on a GitHub issue.
type GitHubComment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ListOpts holds optional parameters for list operations.
type ListOpts struct {
	ETag    string
	Since   string
	PerPage int
	Labels  string // comma-separated label filter
}

// RateLimit holds the current rate limit status from GitHub API.
type RateLimit struct {
	Remaining int
	Reset     time.Time
}

// Client defines the interface for interacting with the GitHub REST API.
type Client interface {
	ListIssues(ctx context.Context, owner, repo string, opts ListOpts) ([]*GitHubIssue, string, error)
	GetIssue(ctx context.Context, owner, repo string, number int) (*GitHubIssue, error)
	CreateIssue(ctx context.Context, owner, repo, title, body string, labels []string) (*GitHubIssue, error)
	UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error
	ListComments(ctx context.Context, owner, repo string, number int, opts ListOpts) ([]*GitHubComment, string, error)
	CreateComment(ctx context.Context, owner, repo string, number int, body string) (*GitHubComment, error)
	CreateLabel(ctx context.Context, owner, repo, name, color, description string) error
	GetRateLimit() RateLimit
}

// clientImpl is the concrete implementation of Client.
type clientImpl struct {
	token      string
	httpClient *http.Client
	baseURL    string

	mu        sync.RWMutex
	rateLimit RateLimit
}

// NewClient creates a new GitHub API client with the given token.
func NewClient(token string) Client {
	return &clientImpl{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

// NewClientWithHTTP creates a new GitHub API client with a custom http.Client (useful for testing).
func NewClientWithHTTP(token string, httpClient *http.Client) Client {
	return &clientImpl{
		token:      token,
		httpClient: httpClient,
		baseURL:    defaultBaseURL,
	}
}

// newClientWithBaseURL is an internal constructor for testing with httptest servers.
func newClientWithBaseURL(token string, httpClient *http.Client, baseURL string) *clientImpl {
	return &clientImpl{
		token:      token,
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

func (c *clientImpl) newRequest(ctx context.Context, method, url string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *clientImpl) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	c.updateRateLimit(resp)
	return resp, nil
}

func (c *clientImpl) updateRateLimit(resp *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		if remaining, err := strconv.Atoi(v); err == nil {
			c.rateLimit.Remaining = remaining
		}
	}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.rateLimit.Reset = time.Unix(ts, 0)
		}
	}
}

// GetRateLimit returns the most recently observed rate limit status.
func (c *clientImpl) GetRateLimit() RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit
}

// linkNextRe matches Link header entries with rel="next".
var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// parseLinkNext extracts the "next" URL from a Link header value.
func parseLinkNext(linkHeader string) string {
	matches := linkNextRe.FindStringSubmatch(linkHeader)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ListIssues fetches issues from a GitHub repository. Returns issues, the response ETag, and any error.
// If opts.ETag is provided and the server responds with 304, returns nil issues and the same ETag.
func (c *clientImpl) ListIssues(ctx context.Context, owner, repo string, opts ListOpts) ([]*GitHubIssue, string, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 100
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues?per_page=%d&state=all", c.baseURL, owner, repo, perPage)
	if opts.Since != "" {
		url += "&since=" + opts.Since
	}
	if opts.Labels != "" {
		url += "&labels=" + opts.Labels
	}

	var allIssues []*GitHubIssue
	var etag string
	firstPage := true

	for url != "" {
		req, err := c.newRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}

		if firstPage && opts.ETag != "" {
			req.Header.Set("If-None-Match", opts.ETag)
		}

		resp, err := c.do(req)
		if err != nil {
			return nil, "", fmt.Errorf("list issues: %w", err)
		}

		if firstPage {
			etag = resp.Header.Get("ETag")
			if etag == "" {
				etag = opts.ETag
			}
		}

		if resp.StatusCode == http.StatusNotModified {
			resp.Body.Close()
			return nil, opts.ETag, nil
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, "", fmt.Errorf("list issues: unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var issues []*GitHubIssue
		if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
			resp.Body.Close()
			return nil, "", fmt.Errorf("list issues: decode response: %w", err)
		}
		resp.Body.Close()

		allIssues = append(allIssues, issues...)

		// Follow pagination
		url = parseLinkNext(resp.Header.Get("Link"))
		firstPage = false
	}

	return allIssues, etag, nil
}

// GetIssue fetches a single issue by number from a GitHub repository.
func (c *clientImpl) GetIssue(ctx context.Context, owner, repo string, number int) (*GitHubIssue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.baseURL, owner, repo, number)

	req, err := c.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get issue: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var issue GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("get issue: decode response: %w", err)
	}

	return &issue, nil
}

// CreateIssue creates a new issue in the specified repository.
func (c *clientImpl) CreateIssue(ctx context.Context, owner, repo, title, body string, labels []string) (*GitHubIssue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues", c.baseURL, owner, repo)

	payload := map[string]interface{}{
		"title": title,
		"body":  body,
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}

	req, err := c.newRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create issue: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var issue GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("create issue: decode response: %w", err)
	}

	return &issue, nil
}

// UpdateIssueBody updates just the body of an existing issue.
func (c *clientImpl) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.baseURL, owner, repo, number)

	payload := map[string]string{
		"body": body,
	}

	req, err := c.newRequest(ctx, http.MethodPatch, url, payload)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("update issue body: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update issue body: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// Drain and discard the body
	io.Copy(io.Discard, resp.Body)
	return nil
}

// ListComments fetches comments for a specific issue. Returns comments, the response ETag, and any error.
func (c *clientImpl) ListComments(ctx context.Context, owner, repo string, number int, opts ListOpts) ([]*GitHubComment, string, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 100
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=%d", c.baseURL, owner, repo, number, perPage)
	if opts.Since != "" {
		url += "&since=" + opts.Since
	}

	var allComments []*GitHubComment
	var etag string
	firstPage := true

	for url != "" {
		req, err := c.newRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}

		if firstPage && opts.ETag != "" {
			req.Header.Set("If-None-Match", opts.ETag)
		}

		resp, err := c.do(req)
		if err != nil {
			return nil, "", fmt.Errorf("list comments: %w", err)
		}

		if firstPage {
			etag = resp.Header.Get("ETag")
			if etag == "" {
				etag = opts.ETag
			}
		}

		if resp.StatusCode == http.StatusNotModified {
			resp.Body.Close()
			return nil, opts.ETag, nil
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, "", fmt.Errorf("list comments: unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var comments []*GitHubComment
		if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
			resp.Body.Close()
			return nil, "", fmt.Errorf("list comments: decode response: %w", err)
		}
		resp.Body.Close()

		allComments = append(allComments, comments...)

		url = parseLinkNext(resp.Header.Get("Link"))
		firstPage = false
	}

	return allComments, etag, nil
}

// CreateComment posts a new comment on the specified issue.
func (c *clientImpl) CreateComment(ctx context.Context, owner, repo string, number int, body string) (*GitHubComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, number)

	payload := map[string]string{
		"body": body,
	}

	req, err := c.newRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create comment: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var comment GitHubComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("create comment: decode response: %w", err)
	}

	return &comment, nil
}

// CreateLabel creates a label in the specified repository.
// If the label already exists (422), it is not treated as an error.
func (c *clientImpl) CreateLabel(ctx context.Context, owner, repo, name, color, description string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/labels", c.baseURL, owner, repo)

	// Strip leading '#' from color if present
	color = strings.TrimPrefix(color, "#")

	payload := map[string]string{
		"name":        name,
		"color":       color,
		"description": description,
	}

	req, err := c.newRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("create label: %w", err)
	}
	defer resp.Body.Close()

	// 201 Created or 422 Unprocessable Entity (already exists) are both OK
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusUnprocessableEntity {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("create label: unexpected status %d: %s", resp.StatusCode, string(respBody))
}
