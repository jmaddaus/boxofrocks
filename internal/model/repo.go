package model

import "time"

type RepoConfig struct {
	ID             int        `json:"id"`
	Owner          string     `json:"owner"`
	Name           string     `json:"name"`
	PollIntervalMs int        `json:"poll_interval_ms"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	IssuesETag     string     `json:"issues_etag"`
	IssuesSince    string     `json:"issues_since"`
	CreatedAt      time.Time  `json:"created_at"`
}

// FullName returns "owner/name".
func (r *RepoConfig) FullName() string {
	return r.Owner + "/" + r.Name
}
