package model

import (
	"path/filepath"
	"time"
)

type RepoConfig struct {
	ID                 int        `json:"id"`
	Owner              string     `json:"owner"`
	Name               string     `json:"name"`
	PollIntervalMs     int        `json:"poll_interval_ms"`
	LastSyncAt         *time.Time `json:"last_sync_at,omitempty"`
	IssuesETag         string     `json:"issues_etag"`
	IssuesSince        string     `json:"issues_since"`
	TrustedAuthorsOnly bool       `json:"trusted_authors_only"`
	LocalPath          string     `json:"local_path,omitempty"`
	SocketEnabled      bool       `json:"socket_enabled"`
	QueueEnabled       bool       `json:"queue_enabled"`
	CreatedAt          time.Time  `json:"created_at"`
}

// FullName returns "owner/name".
func (r *RepoConfig) FullName() string {
	return r.Owner + "/" + r.Name
}

// SocketPath returns the path to the Unix domain socket for this repo,
// or "" if socket is not enabled or local path is not set.
func (r *RepoConfig) SocketPath() string {
	if !r.SocketEnabled || r.LocalPath == "" {
		return ""
	}
	return filepath.Join(r.LocalPath, ".boxofrocks", "bor.sock")
}

// QueueDir returns the path to the file-based queue directory for this repo,
// or "" if queue is not enabled or local path is not set.
func (r *RepoConfig) QueueDir() string {
	if !r.QueueEnabled || r.LocalPath == "" {
		return ""
	}
	return filepath.Join(r.LocalPath, ".boxofrocks", "queue")
}
