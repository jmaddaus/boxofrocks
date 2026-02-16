package store

import (
	"context"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// IssueFilter holds optional filter criteria for listing issues.
type IssueFilter struct {
	RepoID   int
	Status   model.Status
	Priority *int
	Type     model.IssueType
	Owner    string
}

// Store defines the persistence interface for the agent tracker.
type Store interface {
	// Repos
	AddRepo(ctx context.Context, owner, name string) (*model.RepoConfig, error)
	GetRepo(ctx context.Context, id int) (*model.RepoConfig, error)
	GetRepoByName(ctx context.Context, owner, name string) (*model.RepoConfig, error)
	ListRepos(ctx context.Context) ([]*model.RepoConfig, error)
	UpdateRepo(ctx context.Context, repo *model.RepoConfig) error

	// Local paths (worktree support)
	AddLocalPath(ctx context.Context, repoID int, localPath string, socket, queue bool) (*model.LocalPathConfig, error)
	RemoveLocalPath(ctx context.Context, repoID int, localPath string) error
	ListLocalPaths(ctx context.Context, repoID int) ([]model.LocalPathConfig, error)

	// Issues
	CreateIssue(ctx context.Context, issue *model.Issue) (*model.Issue, error)
	GetIssue(ctx context.Context, id int) (*model.Issue, error)
	ListIssues(ctx context.Context, filter IssueFilter) ([]*model.Issue, error)
	UpdateIssue(ctx context.Context, issue *model.Issue) error
	DeleteIssue(ctx context.Context, id int) error
	NextIssue(ctx context.Context, repoID int) (*model.Issue, error)

	// Events
	AppendEvent(ctx context.Context, event *model.Event) (*model.Event, error)
	ListEvents(ctx context.Context, repoID, issueID int) ([]*model.Event, error)
	PendingEvents(ctx context.Context, repoID int) ([]*model.Event, error)
	MarkEventSynced(ctx context.Context, eventID int, githubCommentID int) error

	// Sync state
	GetIssueSyncState(ctx context.Context, repoID, githubIssueNumber int) (lastCommentID int, lastCommentAt string, err error)
	SetIssueSyncState(ctx context.Context, repoID, githubIssueNumber, lastCommentID int, lastCommentAt string) error

	Close() error
}
