package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and runs
// migrations. Use ":memory:" for an in-memory database.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode and foreign keys for better concurrency and integrity.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Repos
// ---------------------------------------------------------------------------

func (s *SQLiteStore) AddRepo(ctx context.Context, owner, name string) (*model.RepoConfig, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO repos (owner, name) VALUES (?, ?)`, owner, name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("repo %s/%s already exists", owner, name)
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetRepo(ctx, int(id))
}

func (s *SQLiteStore) GetRepo(ctx context.Context, id int) (*model.RepoConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, owner, name, poll_interval_ms, last_sync_at, issues_etag, issues_since, trusted_authors_only, local_path, socket_enabled, queue_enabled, created_at
		 FROM repos WHERE id = ?`, id)
	return scanRepo(row)
}

func (s *SQLiteStore) GetRepoByName(ctx context.Context, owner, name string) (*model.RepoConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, owner, name, poll_interval_ms, last_sync_at, issues_etag, issues_since, trusted_authors_only, local_path, socket_enabled, queue_enabled, created_at
		 FROM repos WHERE owner = ? AND name = ?`, owner, name)
	return scanRepo(row)
}

func (s *SQLiteStore) ListRepos(ctx context.Context) ([]*model.RepoConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner, name, poll_interval_ms, last_sync_at, issues_etag, issues_since, trusted_authors_only, local_path, socket_enabled, queue_enabled, created_at
		 FROM repos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*model.RepoConfig
	for rows.Next() {
		r, err := scanRepoRow(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *SQLiteStore) UpdateRepo(ctx context.Context, repo *model.RepoConfig) error {
	var lastSync *string
	if repo.LastSyncAt != nil {
		t := repo.LastSyncAt.Format(time.RFC3339)
		lastSync = &t
	}
	trustedInt := 0
	if repo.TrustedAuthorsOnly {
		trustedInt = 1
	}
	socketInt := 0
	if repo.SocketEnabled {
		socketInt = 1
	}
	queueInt := 0
	if repo.QueueEnabled {
		queueInt = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE repos SET owner=?, name=?, poll_interval_ms=?, last_sync_at=?, issues_etag=?, issues_since=?, trusted_authors_only=?, local_path=?, socket_enabled=?, queue_enabled=?
		 WHERE id=?`,
		repo.Owner, repo.Name, repo.PollIntervalMs, lastSync, repo.IssuesETag, repo.IssuesSince, trustedInt, repo.LocalPath, socketInt, queueInt, repo.ID)
	return err
}

// ---------------------------------------------------------------------------
// Issues
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateIssue(ctx context.Context, issue *model.Issue) (*model.Issue, error) {
	now := time.Now().UTC()
	if issue.CreatedAt.IsZero() {
		issue.CreatedAt = now
	}
	if issue.UpdatedAt.IsZero() {
		issue.UpdatedAt = now
	}
	if issue.Status == "" {
		issue.Status = model.StatusOpen
	}
	if issue.IssueType == "" {
		issue.IssueType = model.IssueTypeTask
	}
	if issue.Labels == nil {
		issue.Labels = []string{}
	}
	if issue.Comments == nil {
		issue.Comments = []model.Comment{}
	}

	labelsJSON, err := json.Marshal(issue.Labels)
	if err != nil {
		return nil, fmt.Errorf("marshal labels: %w", err)
	}
	commentsJSON, err := json.Marshal(issue.Comments)
	if err != nil {
		return nil, fmt.Errorf("marshal comments: %w", err)
	}

	var githubID *int
	if issue.GitHubID != nil {
		githubID = issue.GitHubID
	}
	var closedAt *string
	if issue.ClosedAt != nil {
		t := issue.ClosedAt.Format(time.RFC3339)
		closedAt = &t
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO issues (repo_id, github_id, title, status, priority, issue_type, description, owner, labels, created_at, updated_at, closed_at, comments)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.RepoID, githubID, issue.Title, string(issue.Status), issue.Priority,
		string(issue.IssueType), issue.Description, issue.Owner,
		string(labelsJSON),
		issue.CreatedAt.Format(time.RFC3339), issue.UpdatedAt.Format(time.RFC3339),
		closedAt, string(commentsJSON))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetIssue(ctx, int(id))
}

func (s *SQLiteStore) GetIssue(ctx context.Context, id int) (*model.Issue, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, github_id, title, status, priority, issue_type, description, owner, labels, created_at, updated_at, closed_at, comments
		 FROM issues WHERE id = ?`, id)
	return scanIssue(row)
}

func (s *SQLiteStore) ListIssues(ctx context.Context, filter IssueFilter) ([]*model.Issue, error) {
	query := `SELECT id, repo_id, github_id, title, status, priority, issue_type, description, owner, labels, created_at, updated_at, closed_at, comments FROM issues WHERE 1=1`
	var args []interface{}

	if filter.RepoID != 0 {
		query += " AND repo_id = ?"
		args = append(args, filter.RepoID)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, string(filter.Status))
	}
	if filter.Priority != nil {
		query += " AND priority = ?"
		args = append(args, *filter.Priority)
	}
	if filter.Type != "" {
		query += " AND issue_type = ?"
		args = append(args, string(filter.Type))
	}
	if filter.Owner != "" {
		query += " AND owner = ?"
		args = append(args, filter.Owner)
	}

	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []*model.Issue
	for rows.Next() {
		iss, err := scanIssueRow(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, iss)
	}
	return issues, rows.Err()
}

func (s *SQLiteStore) UpdateIssue(ctx context.Context, issue *model.Issue) error {
	issue.UpdatedAt = time.Now().UTC()
	if issue.Labels == nil {
		issue.Labels = []string{}
	}
	if issue.Comments == nil {
		issue.Comments = []model.Comment{}
	}
	labelsJSON, err := json.Marshal(issue.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	commentsJSON, err := json.Marshal(issue.Comments)
	if err != nil {
		return fmt.Errorf("marshal comments: %w", err)
	}
	var closedAt *string
	if issue.ClosedAt != nil {
		t := issue.ClosedAt.Format(time.RFC3339)
		closedAt = &t
	}
	var githubID *int
	if issue.GitHubID != nil {
		githubID = issue.GitHubID
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE issues SET repo_id=?, github_id=?, title=?, status=?, priority=?, issue_type=?, description=?, owner=?, labels=?, updated_at=?, closed_at=?, comments=?
		 WHERE id=?`,
		issue.RepoID, githubID, issue.Title, string(issue.Status), issue.Priority,
		string(issue.IssueType), issue.Description, issue.Owner,
		string(labelsJSON),
		issue.UpdatedAt.Format(time.RFC3339), closedAt,
		string(commentsJSON),
		issue.ID)
	return err
}

func (s *SQLiteStore) DeleteIssue(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE issues SET status = ?, updated_at = ? WHERE id = ?`,
		string(model.StatusDeleted), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *SQLiteStore) NextIssue(ctx context.Context, repoID int) (*model.Issue, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, github_id, title, status, priority, issue_type, description, owner, labels, created_at, updated_at, closed_at, comments
		 FROM issues
		 WHERE repo_id = ? AND status = 'open' AND owner = ''
		 ORDER BY priority ASC, created_at ASC
		 LIMIT 1`, repoID)
	return scanIssue(row)
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

func (s *SQLiteStore) AppendEvent(ctx context.Context, event *model.Event) (*model.Event, error) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	var githubCommentID *int
	if event.GitHubCommentID != nil {
		githubCommentID = event.GitHubCommentID
	}
	var githubIssueNumber *int
	if event.GitHubIssueNumber != nil {
		githubIssueNumber = event.GitHubIssueNumber
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events (repo_id, github_comment_id, issue_id, github_issue_number, timestamp, action, payload, agent, synced)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RepoID, githubCommentID, event.IssueID, githubIssueNumber,
		event.Timestamp.Format(time.RFC3339), string(event.Action), event.Payload,
		event.Agent, event.Synced)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.getEvent(ctx, int(id))
}

func (s *SQLiteStore) getEvent(ctx context.Context, id int) (*model.Event, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, github_comment_id, issue_id, github_issue_number, timestamp, action, payload, agent, synced
		 FROM events WHERE id = ?`, id)
	return scanEvent(row)
}

func (s *SQLiteStore) ListEvents(ctx context.Context, repoID, issueID int) ([]*model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, github_comment_id, issue_id, github_issue_number, timestamp, action, payload, agent, synced
		 FROM events WHERE repo_id = ? AND issue_id = ? ORDER BY id`,
		repoID, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.Event
	for rows.Next() {
		e, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) PendingEvents(ctx context.Context, repoID int) ([]*model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, github_comment_id, issue_id, github_issue_number, timestamp, action, payload, agent, synced
		 FROM events WHERE repo_id = ? AND synced = 0 ORDER BY id`,
		repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.Event
	for rows.Next() {
		e, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) MarkEventSynced(ctx context.Context, eventID int, githubCommentID int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE events SET synced = 1, github_comment_id = ? WHERE id = ?`,
		githubCommentID, eventID)
	return err
}

// ---------------------------------------------------------------------------
// Sync state
// ---------------------------------------------------------------------------

func (s *SQLiteStore) GetIssueSyncState(ctx context.Context, repoID, githubIssueNumber int) (int, string, error) {
	var lastCommentID int
	var lastCommentAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT last_comment_id, last_comment_at FROM issue_sync_state
		 WHERE repo_id = ? AND github_issue_number = ?`,
		repoID, githubIssueNumber).Scan(&lastCommentID, &lastCommentAt)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	return lastCommentID, lastCommentAt.String, nil
}

func (s *SQLiteStore) SetIssueSyncState(ctx context.Context, repoID, githubIssueNumber, lastCommentID int, lastCommentAt string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO issue_sync_state (repo_id, github_issue_number, last_comment_id, last_comment_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(repo_id, github_issue_number)
		 DO UPDATE SET last_comment_id = excluded.last_comment_id, last_comment_at = excluded.last_comment_at`,
		repoID, githubIssueNumber, lastCommentID, lastCommentAt)
	return err
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanRepo(row scanner) (*model.RepoConfig, error) {
	var r model.RepoConfig
	var lastSync sql.NullString
	var trustedInt int
	var socketInt int
	var queueInt int
	var createdAt string
	err := row.Scan(&r.ID, &r.Owner, &r.Name, &r.PollIntervalMs, &lastSync, &r.IssuesETag, &r.IssuesSince, &trustedInt, &r.LocalPath, &socketInt, &queueInt, &createdAt)
	if err != nil {
		return nil, err
	}
	r.TrustedAuthorsOnly = trustedInt != 0
	r.SocketEnabled = socketInt != 0
	r.QueueEnabled = queueInt != 0
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if r.CreatedAt.IsZero() {
		// Fallback: the SQLite default uses datetime('now') which is "2006-01-02 15:04:05"
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	if lastSync.Valid {
		t, _ := time.Parse(time.RFC3339, lastSync.String)
		if t.IsZero() {
			t, _ = time.Parse("2006-01-02 15:04:05", lastSync.String)
		}
		if !t.IsZero() {
			r.LastSyncAt = &t
		}
	}
	return &r, nil
}

func scanRepoRow(rows *sql.Rows) (*model.RepoConfig, error) {
	return scanRepo(rows)
}

func scanIssue(row scanner) (*model.Issue, error) {
	var iss model.Issue
	var githubID sql.NullInt64
	var labelsJSON string
	var commentsJSON string
	var createdAt, updatedAt string
	var closedAt sql.NullString

	err := row.Scan(&iss.ID, &iss.RepoID, &githubID, &iss.Title,
		&iss.Status, &iss.Priority, &iss.IssueType,
		&iss.Description, &iss.Owner, &labelsJSON,
		&createdAt, &updatedAt, &closedAt, &commentsJSON)
	if err != nil {
		return nil, err
	}

	if githubID.Valid {
		v := int(githubID.Int64)
		iss.GitHubID = &v
	}
	if err := json.Unmarshal([]byte(labelsJSON), &iss.Labels); err != nil {
		iss.Labels = []string{}
	}
	if err := json.Unmarshal([]byte(commentsJSON), &iss.Comments); err != nil {
		iss.Comments = []model.Comment{}
	}
	iss.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	iss.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339, closedAt.String)
		if !t.IsZero() {
			iss.ClosedAt = &t
		}
	}
	return &iss, nil
}

func scanIssueRow(rows *sql.Rows) (*model.Issue, error) {
	return scanIssue(rows)
}

func scanEvent(row scanner) (*model.Event, error) {
	var e model.Event
	var githubCommentID sql.NullInt64
	var githubIssueNumber sql.NullInt64
	var ts string

	err := row.Scan(&e.ID, &e.RepoID, &githubCommentID, &e.IssueID,
		&githubIssueNumber, &ts, &e.Action, &e.Payload, &e.Agent, &e.Synced)
	if err != nil {
		return nil, err
	}

	if githubCommentID.Valid {
		v := int(githubCommentID.Int64)
		e.GitHubCommentID = &v
	}
	if githubIssueNumber.Valid {
		v := int(githubIssueNumber.Int64)
		e.GitHubIssueNumber = &v
	}
	e.Timestamp, _ = time.Parse(time.RFC3339, ts)
	return &e, nil
}

func scanEventRow(rows *sql.Rows) (*model.Event, error) {
	return scanEvent(rows)
}
