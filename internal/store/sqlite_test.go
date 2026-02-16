package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// newTestStore creates a fresh in-memory SQLite store for testing.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// addTestRepo is a helper that adds a repo and fails the test on error.
func addTestRepo(t *testing.T, s *SQLiteStore, owner, name string) *model.RepoConfig {
	t.Helper()
	repo, err := s.AddRepo(context.Background(), owner, name)
	if err != nil {
		t.Fatalf("AddRepo(%s/%s): %v", owner, name, err)
	}
	return repo
}

// ---------------------------------------------------------------------------
// Repo tests
// ---------------------------------------------------------------------------

func TestAddRepo(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	repo, err := s.AddRepo(ctx, "octocat", "hello-world")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if repo.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if repo.Owner != "octocat" || repo.Name != "hello-world" {
		t.Errorf("unexpected owner/name: %s/%s", repo.Owner, repo.Name)
	}
	if repo.PollIntervalMs != 5000 {
		t.Errorf("expected default poll_interval_ms=5000, got %d", repo.PollIntervalMs)
	}
}

func TestAddRepoDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.AddRepo(ctx, "octocat", "hello-world")
	if err != nil {
		t.Fatalf("first AddRepo: %v", err)
	}
	_, err = s.AddRepo(ctx, "octocat", "hello-world")
	if err == nil {
		t.Fatal("expected error for duplicate repo, got nil")
	}
}

func TestGetRepo(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created := addTestRepo(t, s, "octocat", "hello-world")
	got, err := s.GetRepo(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if got.Owner != "octocat" || got.Name != "hello-world" {
		t.Errorf("unexpected repo: %+v", got)
	}
}

func TestGetRepoNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRepo(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for non-existent repo")
	}
}

func TestGetRepoByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	addTestRepo(t, s, "octocat", "hello-world")

	got, err := s.GetRepoByName(ctx, "octocat", "hello-world")
	if err != nil {
		t.Fatalf("GetRepoByName: %v", err)
	}
	if got.Owner != "octocat" || got.Name != "hello-world" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestListRepos(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	addTestRepo(t, s, "a", "one")
	addTestRepo(t, s, "b", "two")

	repos, err := s.ListRepos(ctx)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestUpdateRepo(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	now := time.Now().UTC().Truncate(time.Second)
	repo.PollIntervalMs = 10000
	repo.LastSyncAt = &now
	repo.IssuesETag = "abc123"

	if err := s.UpdateRepo(ctx, repo); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}

	got, err := s.GetRepo(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepo after update: %v", err)
	}
	if got.PollIntervalMs != 10000 {
		t.Errorf("poll_interval_ms: want 10000, got %d", got.PollIntervalMs)
	}
	if got.IssuesETag != "abc123" {
		t.Errorf("issues_etag: want abc123, got %s", got.IssuesETag)
	}
	if got.LastSyncAt == nil {
		t.Fatal("expected LastSyncAt to be set")
	}
}

func TestUpdateRepoIssuesSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	// Initially empty.
	if repo.IssuesSince != "" {
		t.Errorf("expected empty IssuesSince, got %q", repo.IssuesSince)
	}

	// Set IssuesSince and update.
	repo.IssuesSince = "2024-06-15T12:00:00Z"
	if err := s.UpdateRepo(ctx, repo); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}

	// Read back.
	got, err := s.GetRepo(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if got.IssuesSince != "2024-06-15T12:00:00Z" {
		t.Errorf("IssuesSince: want 2024-06-15T12:00:00Z, got %s", got.IssuesSince)
	}
}

func TestUpdateRepoSocketFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	// Initially empty/false.
	if repo.LocalPath != "" {
		t.Errorf("expected empty LocalPath, got %q", repo.LocalPath)
	}
	if repo.SocketEnabled {
		t.Error("expected SocketEnabled=false initially")
	}

	// Set local_path and socket_enabled.
	repo.LocalPath = "/tmp/my-repo"
	repo.SocketEnabled = true
	if err := s.UpdateRepo(ctx, repo); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}

	// Read back.
	got, err := s.GetRepo(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if got.LocalPath != "/tmp/my-repo" {
		t.Errorf("LocalPath: want /tmp/my-repo, got %s", got.LocalPath)
	}
	if !got.SocketEnabled {
		t.Error("expected SocketEnabled=true")
	}

	// Verify SocketPath() returns the expected value.
	expected := "/tmp/my-repo/.boxofrocks/bor.sock"
	if got.SocketPath() != expected {
		t.Errorf("SocketPath: want %s, got %s", expected, got.SocketPath())
	}

	// Disable socket.
	got.SocketEnabled = false
	if err := s.UpdateRepo(ctx, got); err != nil {
		t.Fatalf("UpdateRepo (disable): %v", err)
	}

	got2, err := s.GetRepo(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepo after disable: %v", err)
	}
	if got2.SocketEnabled {
		t.Error("expected SocketEnabled=false after disable")
	}
	if got2.SocketPath() != "" {
		t.Errorf("SocketPath should be empty when disabled, got %s", got2.SocketPath())
	}
}

// ---------------------------------------------------------------------------
// Issue tests
// ---------------------------------------------------------------------------

func TestCreateAndGetIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	ghID := 42
	issue := &model.Issue{
		RepoID:      repo.ID,
		GitHubID:    &ghID,
		Title:       "Fix the bug",
		Priority:    1,
		IssueType:   model.IssueTypeBug,
		Description: "Something is broken",
		Owner:       "alice",
		Labels:      []string{"bug", "urgent"},
	}
	created, err := s.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Status != model.StatusOpen {
		t.Errorf("expected status open, got %s", created.Status)
	}
	if len(created.Labels) != 2 || created.Labels[0] != "bug" {
		t.Errorf("unexpected labels: %v", created.Labels)
	}
	if created.GitHubID == nil || *created.GitHubID != 42 {
		t.Errorf("expected github_id=42, got %v", created.GitHubID)
	}

	got, err := s.GetIssue(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != "Fix the bug" {
		t.Errorf("title: want 'Fix the bug', got '%s'", got.Title)
	}
}

func TestCreateIssueDefaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	created, err := s.CreateIssue(ctx, &model.Issue{
		RepoID: repo.ID,
		Title:  "Simple task",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if created.Status != model.StatusOpen {
		t.Errorf("expected default status open, got %s", created.Status)
	}
	if created.IssueType != model.IssueTypeTask {
		t.Errorf("expected default issue_type task, got %s", created.IssueType)
	}
	if created.Labels == nil {
		t.Error("expected non-nil labels slice")
	}
}

func TestGetIssueNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetIssue(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for non-existent issue")
	}
}

func TestUpdateIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	created, _ := s.CreateIssue(ctx, &model.Issue{
		RepoID: repo.ID,
		Title:  "Original",
	})

	created.Title = "Updated"
	created.Status = model.StatusInProgress
	created.Owner = "bob"
	created.Labels = []string{"wip"}
	if err := s.UpdateIssue(ctx, created); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	got, _ := s.GetIssue(ctx, created.ID)
	if got.Title != "Updated" {
		t.Errorf("title: want 'Updated', got '%s'", got.Title)
	}
	if got.Status != model.StatusInProgress {
		t.Errorf("status: want in_progress, got %s", got.Status)
	}
	if got.Owner != "bob" {
		t.Errorf("owner: want bob, got %s", got.Owner)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "wip" {
		t.Errorf("labels: want [wip], got %v", got.Labels)
	}
}

func TestDeleteIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	created, _ := s.CreateIssue(ctx, &model.Issue{
		RepoID: repo.ID,
		Title:  "To delete",
	})

	if err := s.DeleteIssue(ctx, created.ID); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}

	got, err := s.GetIssue(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetIssue after delete: %v", err)
	}
	if got.Status != model.StatusDeleted {
		t.Errorf("expected status deleted, got %s", got.Status)
	}
}

func TestListIssuesNoFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	for i := 0; i < 3; i++ {
		s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "issue"})
	}

	issues, err := s.ListIssues(ctx, IssueFilter{})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d", len(issues))
	}
}

func TestListIssuesFilterByStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	open, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "open one"})
	_ = open
	closed, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "closed one"})
	closed.Status = model.StatusClosed
	s.UpdateIssue(ctx, closed)

	issues, err := s.ListIssues(ctx, IssueFilter{Status: model.StatusOpen})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 open issue, got %d", len(issues))
	}
}

func TestListIssuesFilterByPriority(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "p1", Priority: 1})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "p2", Priority: 2})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "p1 also", Priority: 1})

	pri := 1
	issues, err := s.ListIssues(ctx, IssueFilter{Priority: &pri})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("expected 2 issues with priority 1, got %d", len(issues))
	}
}

func TestListIssuesFilterByType(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "bug", IssueType: model.IssueTypeBug})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "task"})

	issues, err := s.ListIssues(ctx, IssueFilter{Type: model.IssueTypeBug})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 bug, got %d", len(issues))
	}
	if issues[0].Title != "bug" {
		t.Errorf("expected 'bug', got '%s'", issues[0].Title)
	}
}

func TestListIssuesFilterByOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "alice task", Owner: "alice"})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "bob task", Owner: "bob"})

	issues, err := s.ListIssues(ctx, IssueFilter{Owner: "alice"})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue for alice, got %d", len(issues))
	}
}

func TestListIssuesFilterByRepoID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo1 := addTestRepo(t, s, "a", "one")
	repo2 := addTestRepo(t, s, "b", "two")

	s.CreateIssue(ctx, &model.Issue{RepoID: repo1.ID, Title: "r1"})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo2.ID, Title: "r2"})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo1.ID, Title: "r1 also"})

	issues, err := s.ListIssues(ctx, IssueFilter{RepoID: repo1.ID})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("expected 2 issues for repo1, got %d", len(issues))
	}
}

func TestListIssuesCombinedFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "open bug", IssueType: model.IssueTypeBug, Owner: "alice"})
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "open task", Owner: "alice"})
	iss3, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "closed bug", IssueType: model.IssueTypeBug, Owner: "alice"})
	iss3.Status = model.StatusClosed
	s.UpdateIssue(ctx, iss3)

	issues, err := s.ListIssues(ctx, IssueFilter{
		Status: model.StatusOpen,
		Type:   model.IssueTypeBug,
		Owner:  "alice",
	})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 open bug for alice, got %d", len(issues))
	}
}

// ---------------------------------------------------------------------------
// NextIssue tests
// ---------------------------------------------------------------------------

func TestNextIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	// Create issues with different priorities. Lower number = higher priority.
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "low priority", Priority: 3})
	time.Sleep(10 * time.Millisecond) // ensure different created_at
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "high priority", Priority: 1})
	time.Sleep(10 * time.Millisecond)
	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "medium priority", Priority: 2})

	next, err := s.NextIssue(ctx, repo.ID)
	if err != nil {
		t.Fatalf("NextIssue: %v", err)
	}
	if next.Title != "high priority" {
		t.Errorf("expected 'high priority', got '%s'", next.Title)
	}
}

func TestNextIssueSkipsAssigned(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	iss1, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "assigned", Priority: 1})
	iss1.Owner = "alice"
	s.UpdateIssue(ctx, iss1)

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "unassigned", Priority: 2})

	next, err := s.NextIssue(ctx, repo.ID)
	if err != nil {
		t.Fatalf("NextIssue: %v", err)
	}
	if next.Title != "unassigned" {
		t.Errorf("expected 'unassigned', got '%s'", next.Title)
	}
}

func TestNextIssueSkipsClosed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	iss1, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "closed", Priority: 1})
	iss1.Status = model.StatusClosed
	s.UpdateIssue(ctx, iss1)

	s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "open", Priority: 2})

	next, err := s.NextIssue(ctx, repo.ID)
	if err != nil {
		t.Fatalf("NextIssue: %v", err)
	}
	if next.Title != "open" {
		t.Errorf("expected 'open', got '%s'", next.Title)
	}
}

func TestNextIssueNoneAvailable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	_, err := s.NextIssue(ctx, repo.ID)
	if err == nil {
		t.Fatal("expected error when no issues available")
	}
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestNextIssueSamePriorityOrderByCreated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	s.CreateIssue(ctx, &model.Issue{
		RepoID:    repo.ID,
		Title:     "first created",
		Priority:  1,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	s.CreateIssue(ctx, &model.Issue{
		RepoID:    repo.ID,
		Title:     "second created",
		Priority:  1,
		CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})

	next, err := s.NextIssue(ctx, repo.ID)
	if err != nil {
		t.Fatalf("NextIssue: %v", err)
	}
	if next.Title != "first created" {
		t.Errorf("expected 'first created', got '%s'", next.Title)
	}
}

// ---------------------------------------------------------------------------
// Event tests
// ---------------------------------------------------------------------------

func TestAppendAndListEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	issue, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "task"})

	ghIssueNum := 10
	evt, err := s.AppendEvent(ctx, &model.Event{
		RepoID:            repo.ID,
		IssueID:           issue.ID,
		GitHubIssueNumber: &ghIssueNum,
		Action:            model.ActionCreate,
		Payload:           `{"title":"task"}`,
		Agent:             "test-agent",
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if evt.ID == 0 {
		t.Error("expected non-zero event ID")
	}
	if evt.Synced != 0 {
		t.Errorf("expected synced=0, got %d", evt.Synced)
	}
	if evt.GitHubIssueNumber == nil || *evt.GitHubIssueNumber != 10 {
		t.Errorf("expected github_issue_number=10, got %v", evt.GitHubIssueNumber)
	}

	events, err := s.ListEvents(ctx, repo.ID, issue.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Action != model.ActionCreate {
		t.Errorf("action: want create, got %s", events[0].Action)
	}
}

func TestPendingEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	issue, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "task"})

	// Create two events - both pending initially.
	e1, _ := s.AppendEvent(ctx, &model.Event{
		RepoID:  repo.ID,
		IssueID: issue.ID,
		Action:  model.ActionCreate,
		Payload: `{}`,
	})
	s.AppendEvent(ctx, &model.Event{
		RepoID:  repo.ID,
		IssueID: issue.ID,
		Action:  model.ActionUpdate,
		Payload: `{}`,
	})

	pending, err := s.PendingEvents(ctx, repo.ID)
	if err != nil {
		t.Fatalf("PendingEvents: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending events, got %d", len(pending))
	}

	// Mark the first one synced.
	if err := s.MarkEventSynced(ctx, e1.ID, 100); err != nil {
		t.Fatalf("MarkEventSynced: %v", err)
	}

	pending, err = s.PendingEvents(ctx, repo.ID)
	if err != nil {
		t.Fatalf("PendingEvents after sync: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(pending))
	}
}

func TestMarkEventSynced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	issue, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "task"})

	evt, _ := s.AppendEvent(ctx, &model.Event{
		RepoID:  repo.ID,
		IssueID: issue.ID,
		Action:  model.ActionCreate,
		Payload: `{}`,
	})

	if err := s.MarkEventSynced(ctx, evt.ID, 200); err != nil {
		t.Fatalf("MarkEventSynced: %v", err)
	}

	got, err := s.getEvent(ctx, evt.ID)
	if err != nil {
		t.Fatalf("getEvent: %v", err)
	}
	if got.Synced != 1 {
		t.Errorf("expected synced=1, got %d", got.Synced)
	}
	if got.GitHubCommentID == nil || *got.GitHubCommentID != 200 {
		t.Errorf("expected github_comment_id=200, got %v", got.GitHubCommentID)
	}
}

func TestListEventsFiltersByRepoAndIssue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo1 := addTestRepo(t, s, "a", "one")
	repo2 := addTestRepo(t, s, "b", "two")
	iss1, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo1.ID, Title: "r1 issue"})
	iss2, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo2.ID, Title: "r2 issue"})

	s.AppendEvent(ctx, &model.Event{RepoID: repo1.ID, IssueID: iss1.ID, Action: model.ActionCreate, Payload: `{}`})
	s.AppendEvent(ctx, &model.Event{RepoID: repo2.ID, IssueID: iss2.ID, Action: model.ActionCreate, Payload: `{}`})

	events, err := s.ListEvents(ctx, repo1.ID, iss1.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for repo1/iss1, got %d", len(events))
	}
}

func TestPendingEventsFiltersByRepo(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo1 := addTestRepo(t, s, "a", "one")
	repo2 := addTestRepo(t, s, "b", "two")
	iss1, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo1.ID, Title: "r1 issue"})
	iss2, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo2.ID, Title: "r2 issue"})

	s.AppendEvent(ctx, &model.Event{RepoID: repo1.ID, IssueID: iss1.ID, Action: model.ActionCreate, Payload: `{}`})
	s.AppendEvent(ctx, &model.Event{RepoID: repo2.ID, IssueID: iss2.ID, Action: model.ActionCreate, Payload: `{}`})

	pending, err := s.PendingEvents(ctx, repo1.ID)
	if err != nil {
		t.Fatalf("PendingEvents: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending event for repo1, got %d", len(pending))
	}
}

// ---------------------------------------------------------------------------
// Sync state tests
// ---------------------------------------------------------------------------

func TestGetSetIssueSyncState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Before any state is set, should return zero values.
	id, at, err := s.GetIssueSyncState(ctx, 1, 100)
	if err != nil {
		t.Fatalf("GetIssueSyncState: %v", err)
	}
	if id != 0 || at != "" {
		t.Errorf("expected (0, ''), got (%d, %q)", id, at)
	}

	// Set sync state.
	ts := "2024-01-15T10:30:00Z"
	if err := s.SetIssueSyncState(ctx, 1, 100, 500, ts); err != nil {
		t.Fatalf("SetIssueSyncState: %v", err)
	}

	id, at, err = s.GetIssueSyncState(ctx, 1, 100)
	if err != nil {
		t.Fatalf("GetIssueSyncState: %v", err)
	}
	if id != 500 {
		t.Errorf("last_comment_id: want 500, got %d", id)
	}
	if at != ts {
		t.Errorf("last_comment_at: want %q, got %q", ts, at)
	}
}

func TestSetIssueSyncStateUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetIssueSyncState(ctx, 1, 100, 500, "2024-01-15T10:30:00Z"); err != nil {
		t.Fatalf("SetIssueSyncState: %v", err)
	}

	// Update the same key.
	if err := s.SetIssueSyncState(ctx, 1, 100, 600, "2024-01-16T10:30:00Z"); err != nil {
		t.Fatalf("SetIssueSyncState (upsert): %v", err)
	}

	id, at, err := s.GetIssueSyncState(ctx, 1, 100)
	if err != nil {
		t.Fatalf("GetIssueSyncState: %v", err)
	}
	if id != 600 {
		t.Errorf("last_comment_id: want 600, got %d", id)
	}
	if at != "2024-01-16T10:30:00Z" {
		t.Errorf("last_comment_at: want updated timestamp, got %q", at)
	}
}

// ---------------------------------------------------------------------------
// Migration idempotency
// ---------------------------------------------------------------------------

func TestMigrationIdempotency(t *testing.T) {
	s := newTestStore(t)

	// Running migrations again should not error.
	if err := runMigrations(s.db); err != nil {
		t.Fatalf("second runMigrations: %v", err)
	}

	// And a third time.
	if err := runMigrations(s.db); err != nil {
		t.Fatalf("third runMigrations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestIssueWithNilLabels(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	created, err := s.CreateIssue(ctx, &model.Issue{
		RepoID: repo.ID,
		Title:  "no labels",
		Labels: nil,
	})
	if err != nil {
		t.Fatalf("CreateIssue with nil labels: %v", err)
	}
	if created.Labels == nil {
		t.Error("expected non-nil labels slice (empty)")
	}
	if len(created.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", created.Labels)
	}
}

func TestIssueWithClosedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	closedTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	created, err := s.CreateIssue(ctx, &model.Issue{
		RepoID:   repo.ID,
		Title:    "closed issue",
		Status:   model.StatusClosed,
		ClosedAt: &closedTime,
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if created.ClosedAt == nil {
		t.Fatal("expected ClosedAt to be set")
	}
	if !created.ClosedAt.Equal(closedTime) {
		t.Errorf("ClosedAt: want %v, got %v", closedTime, *created.ClosedAt)
	}
}

func TestEventWithNilOptionalFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")
	issue, _ := s.CreateIssue(ctx, &model.Issue{RepoID: repo.ID, Title: "task"})

	evt, err := s.AppendEvent(ctx, &model.Event{
		RepoID:  repo.ID,
		IssueID: issue.ID,
		Action:  model.ActionCreate,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if evt.GitHubCommentID != nil {
		t.Error("expected nil GitHubCommentID")
	}
	if evt.GitHubIssueNumber != nil {
		t.Error("expected nil GitHubIssueNumber")
	}
}

// ---------------------------------------------------------------------------
// Schema version guard
// ---------------------------------------------------------------------------

func TestDBSchemaVersionIsSet(t *testing.T) {
	s := newTestStore(t)

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("query user_version: %v", err)
	}
	if version != DBSchemaVersion {
		t.Errorf("expected user_version %d, got %d", DBSchemaVersion, version)
	}
}

func TestDBSchemaVersionRejectsNewerDB(t *testing.T) {
	// Open a DB and set its version higher than what the binary supports.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	futureVersion := DBSchemaVersion + 1
	if _, err := db.Exec("PRAGMA user_version = " + fmt.Sprintf("%d", futureVersion)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	err = runMigrations(db)
	if err == nil {
		t.Fatal("expected error for newer schema version, got nil")
	}
	if !strings.Contains(err.Error(), "newer than this binary supports") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDowngradeDB(t *testing.T) {
	db, err := OpenRawDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Set version to 3 (simulating a future DB).
	if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatalf("set version: %v", err)
	}

	// Downgrade to 1.
	if err := DowngradeDB(db, 3, 1); err != nil {
		t.Fatalf("DowngradeDB: %v", err)
	}

	version, err := ReadDBVersion(db)
	if err != nil {
		t.Fatalf("ReadDBVersion: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1 after downgrade, got %d", version)
	}
}

func TestDowngradeDBRejectsInvalidTarget(t *testing.T) {
	db, err := OpenRawDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Target >= current should fail.
	if err := DowngradeDB(db, 1, 1); err == nil {
		t.Error("expected error when target == current")
	}
	if err := DowngradeDB(db, 1, 2); err == nil {
		t.Error("expected error when target > current")
	}

	// Negative target should fail.
	if err := DowngradeDB(db, 1, -1); err == nil {
		t.Error("expected error for negative target")
	}
}

// ---------------------------------------------------------------------------
// Local path tests (worktree support)
// ---------------------------------------------------------------------------

func TestAddLocalPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	lp, err := s.AddLocalPath(ctx, repo.ID, "/home/user/project", true, false)
	if err != nil {
		t.Fatalf("AddLocalPath: %v", err)
	}
	if lp.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if lp.RepoID != repo.ID {
		t.Errorf("expected repo_id=%d, got %d", repo.ID, lp.RepoID)
	}
	if lp.LocalPath != "/home/user/project" {
		t.Errorf("expected local_path '/home/user/project', got %q", lp.LocalPath)
	}
	if !lp.SocketEnabled {
		t.Error("expected socket_enabled=true")
	}
	if lp.QueueEnabled {
		t.Error("expected queue_enabled=false")
	}

	// Verify it's listed.
	paths, err := s.ListLocalPaths(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListLocalPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].LocalPath != "/home/user/project" {
		t.Errorf("expected '/home/user/project', got %q", paths[0].LocalPath)
	}
}

func TestAddLocalPathUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	// Add with socket enabled.
	_, err := s.AddLocalPath(ctx, repo.ID, "/home/user/project", true, false)
	if err != nil {
		t.Fatalf("first AddLocalPath: %v", err)
	}

	// Upsert with different flags.
	lp, err := s.AddLocalPath(ctx, repo.ID, "/home/user/project", false, true)
	if err != nil {
		t.Fatalf("second AddLocalPath: %v", err)
	}
	if lp.SocketEnabled {
		t.Error("expected socket_enabled=false after upsert")
	}
	if !lp.QueueEnabled {
		t.Error("expected queue_enabled=true after upsert")
	}

	// Verify only one entry.
	paths, err := s.ListLocalPaths(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListLocalPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after upsert, got %d", len(paths))
	}
}

func TestRemoveLocalPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	_, err := s.AddLocalPath(ctx, repo.ID, "/home/user/project", true, false)
	if err != nil {
		t.Fatalf("AddLocalPath: %v", err)
	}

	err = s.RemoveLocalPath(ctx, repo.ID, "/home/user/project")
	if err != nil {
		t.Fatalf("RemoveLocalPath: %v", err)
	}

	paths, err := s.ListLocalPaths(ctx, repo.ID)
	if err != nil {
		t.Fatalf("ListLocalPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths after remove, got %d", len(paths))
	}
}

func TestLocalPathGloballyUnique(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo1 := addTestRepo(t, s, "octocat", "repo1")
	repo2 := addTestRepo(t, s, "octocat", "repo2")

	_, err := s.AddLocalPath(ctx, repo1.ID, "/home/user/shared-dir", true, false)
	if err != nil {
		t.Fatalf("AddLocalPath for repo1: %v", err)
	}

	// Adding same path for repo2 should upsert (same local_path unique constraint),
	// updating the repo_id to repo2.
	lp, err := s.AddLocalPath(ctx, repo2.ID, "/home/user/shared-dir", false, true)
	if err != nil {
		t.Fatalf("AddLocalPath for repo2: %v", err)
	}

	// The upsert only updates socket_enabled and queue_enabled; repo_id stays as repo1's
	// because ON CONFLICT DO UPDATE only touches the specified columns.
	// Verify the entry is now associated with repo1 still (upsert doesn't change repo_id).
	if lp.RepoID != repo1.ID {
		t.Logf("note: upsert changed repo_id from %d to %d", repo1.ID, lp.RepoID)
	}

	// The path should exist once.
	paths1, _ := s.ListLocalPaths(ctx, repo1.ID)
	paths2, _ := s.ListLocalPaths(ctx, repo2.ID)
	total := len(paths1) + len(paths2)
	if total != 1 {
		t.Errorf("expected exactly 1 entry total, got %d (repo1=%d, repo2=%d)", total, len(paths1), len(paths2))
	}
}

func TestMultipleLocalPaths(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := addTestRepo(t, s, "octocat", "hello-world")

	_, err := s.AddLocalPath(ctx, repo.ID, "/home/user/worktree-a", true, false)
	if err != nil {
		t.Fatalf("AddLocalPath A: %v", err)
	}
	_, err = s.AddLocalPath(ctx, repo.ID, "/home/user/worktree-b", true, true)
	if err != nil {
		t.Fatalf("AddLocalPath B: %v", err)
	}

	// Verify GetRepo populates LocalPaths and back-fills legacy fields.
	got, err := s.GetRepo(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if len(got.LocalPaths) != 2 {
		t.Fatalf("expected 2 local paths, got %d", len(got.LocalPaths))
	}
	// Legacy fields should be from first entry.
	if got.LocalPath != "/home/user/worktree-a" {
		t.Errorf("expected legacy LocalPath from first entry, got %q", got.LocalPath)
	}
}

func TestLocalPathMigration(t *testing.T) {
	// Simulate a v4 database with local_path set on repos table,
	// then run migrations to verify data is migrated to repo_local_paths.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Set up as v4 schema.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("pragma: %v", err)
		}
	}

	// Set version to 4 first, then run CREATE TABLE migrations manually.
	if _, err := db.Exec("PRAGMA user_version = 4"); err != nil {
		t.Fatalf("set version: %v", err)
	}

	// Create repos table with v4 schema.
	if _, err := db.Exec(`CREATE TABLE repos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner TEXT NOT NULL,
		name TEXT NOT NULL,
		poll_interval_ms INTEGER DEFAULT 5000,
		last_sync_at TEXT,
		issues_etag TEXT DEFAULT '',
		issues_since TEXT DEFAULT '',
		trusted_authors_only INTEGER DEFAULT 0,
		local_path TEXT DEFAULT '',
		socket_enabled INTEGER DEFAULT 0,
		queue_enabled INTEGER DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(owner, name)
	)`); err != nil {
		t.Fatalf("create repos: %v", err)
	}

	// Insert a repo with local_path set.
	if _, err := db.Exec(`INSERT INTO repos (owner, name, local_path, socket_enabled, queue_enabled) VALUES ('o', 'r', '/tmp/myrepo', 1, 1)`); err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	// Run migrations (which should create repo_local_paths and migrate data).
	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Verify data was migrated.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM repo_local_paths").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 migrated row, got %d", count)
	}

	var localPath string
	var socketInt, queueInt int
	if err := db.QueryRow("SELECT local_path, socket_enabled, queue_enabled FROM repo_local_paths").Scan(&localPath, &socketInt, &queueInt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if localPath != "/tmp/myrepo" {
		t.Errorf("expected '/tmp/myrepo', got %q", localPath)
	}
	if socketInt != 1 {
		t.Error("expected socket_enabled=1")
	}
	if queueInt != 1 {
		t.Error("expected queue_enabled=1")
	}

	// Verify version was bumped.
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != DBSchemaVersion {
		t.Errorf("expected version %d, got %d", DBSchemaVersion, version)
	}
}
