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
