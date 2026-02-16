package model

import (
	"path/filepath"
	"time"
)

// LocalPathConfig represents a single local directory registered to a repo.
// Each worktree gets its own entry with independent socket/queue flags.
type LocalPathConfig struct {
	ID            int    `json:"id"`
	RepoID        int    `json:"repo_id"`
	LocalPath     string `json:"local_path"`
	SocketEnabled bool   `json:"socket_enabled"`
	QueueEnabled  bool   `json:"queue_enabled"`
}

// SocketPath returns the path to the Unix domain socket for this local path,
// or "" if socket is not enabled or local path is not set.
func (lp *LocalPathConfig) SocketPath() string {
	if !lp.SocketEnabled || lp.LocalPath == "" {
		return ""
	}
	return filepath.Join(lp.LocalPath, ".boxofrocks", "bor.sock")
}

// QueueDir returns the path to the file-based queue directory for this local path,
// or "" if queue is not enabled or local path is not set.
func (lp *LocalPathConfig) QueueDir() string {
	if !lp.QueueEnabled || lp.LocalPath == "" {
		return ""
	}
	return filepath.Join(lp.LocalPath, ".boxofrocks", "queue")
}

type RepoConfig struct {
	ID                 int               `json:"id"`
	Owner              string            `json:"owner"`
	Name               string            `json:"name"`
	PollIntervalMs     int               `json:"poll_interval_ms"`
	LastSyncAt         *time.Time        `json:"last_sync_at,omitempty"`
	IssuesETag         string            `json:"issues_etag"`
	IssuesSince        string            `json:"issues_since"`
	TrustedAuthorsOnly bool              `json:"trusted_authors_only"`
	LocalPath          string            `json:"local_path,omitempty"`
	SocketEnabled      bool              `json:"socket_enabled"`
	QueueEnabled       bool              `json:"queue_enabled"`
	CreatedAt          time.Time         `json:"created_at"`
	LocalPaths         []LocalPathConfig `json:"local_paths,omitempty"`
}

// FullName returns "owner/name".
func (r *RepoConfig) FullName() string {
	return r.Owner + "/" + r.Name
}

// SocketPath returns the path to the Unix domain socket for this repo,
// or "" if socket is not enabled or local path is not set.
// Uses the first local path entry for backward compatibility.
func (r *RepoConfig) SocketPath() string {
	if !r.SocketEnabled || r.LocalPath == "" {
		return ""
	}
	return filepath.Join(r.LocalPath, ".boxofrocks", "bor.sock")
}

// QueueDir returns the path to the file-based queue directory for this repo,
// or "" if queue is not enabled or local path is not set.
// Uses the first local path entry for backward compatibility.
func (r *RepoConfig) QueueDir() string {
	if !r.QueueEnabled || r.LocalPath == "" {
		return ""
	}
	return filepath.Join(r.LocalPath, ".boxofrocks", "queue")
}

// AllSocketPaths returns socket paths for all registered local paths that have sockets enabled.
func (r *RepoConfig) AllSocketPaths() []string {
	var paths []string
	for _, lp := range r.LocalPaths {
		if sp := lp.SocketPath(); sp != "" {
			paths = append(paths, sp)
		}
	}
	return paths
}

// AllQueueDirs returns queue directories for all registered local paths that have queues enabled.
func (r *RepoConfig) AllQueueDirs() []string {
	var dirs []string
	for _, lp := range r.LocalPaths {
		if qd := lp.QueueDir(); qd != "" {
			dirs = append(dirs, qd)
		}
	}
	return dirs
}
