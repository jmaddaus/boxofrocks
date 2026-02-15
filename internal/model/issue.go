package model

import "time"

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
	StatusDeleted    Status = "deleted"
)

type IssueType string

const (
	IssueTypeTask    IssueType = "task"
	IssueTypeBug     IssueType = "bug"
	IssueTypeFeature IssueType = "feature"
)

type Issue struct {
	ID          int        `json:"id"`
	RepoID      int        `json:"repo_id"`
	GitHubID    *int       `json:"github_id,omitempty"`
	Title       string     `json:"title"`
	Status      Status     `json:"status"`
	Priority    int        `json:"priority"`
	IssueType   IssueType  `json:"issue_type"`
	Description string     `json:"description"`
	Owner       string     `json:"owner"`
	Labels      []string   `json:"labels"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}
