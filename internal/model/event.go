package model

import "time"

type Action string

const (
	ActionCreate       Action = "create"
	ActionStatusChange Action = "status_change"
	ActionAssign       Action = "assign"
	ActionClose        Action = "close"
	ActionUpdate       Action = "update"
	ActionDelete       Action = "delete"
	ActionReopen       Action = "reopen"
	ActionComment      Action = "comment"
)

type Event struct {
	ID                int       `json:"id,omitempty"`
	RepoID            int       `json:"repo_id"`
	GitHubCommentID   *int      `json:"github_comment_id,omitempty"`
	IssueID           int       `json:"issue_id"`
	GitHubIssueNumber *int      `json:"github_issue_number,omitempty"`
	Timestamp         time.Time `json:"timestamp"`
	Action            Action    `json:"action"`
	Payload           string    `json:"payload"`
	Agent             string    `json:"agent,omitempty"`
	Synced            int       `json:"synced"`
}

// EventPayload is the structured data within an event's payload JSON.
type EventPayload struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Status      Status   `json:"status,omitempty"`
	FromStatus  Status   `json:"from_status,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	IssueType   string   `json:"issue_type,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Comment     string   `json:"comment,omitempty"`
}
