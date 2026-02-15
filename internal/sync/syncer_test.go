package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
)

// ---------------------------------------------------------------------------
// Mock GitHub Client
// ---------------------------------------------------------------------------

type mockGitHubClient struct {
	mu sync.Mutex

	// Issues stored per "owner/repo".
	issues map[string][]*github.GitHubIssue

	// Comments stored per "owner/repo/number".
	comments map[string][]*github.GitHubComment

	// Track calls for assertions.
	createdIssues   []createdIssueRecord
	createdComments []createdCommentRecord

	nextIssueNumber int
	nextCommentID   int
	rateLimitVal    github.RateLimit
}

type createdIssueRecord struct {
	Owner, Repo, Title, Body string
	Labels                   []string
}

type createdCommentRecord struct {
	Owner, Repo string
	Number      int
	Body        string
}

func newMockGitHubClient() *mockGitHubClient {
	return &mockGitHubClient{
		issues:          make(map[string][]*github.GitHubIssue),
		comments:        make(map[string][]*github.GitHubComment),
		nextIssueNumber: 100,
		nextCommentID:   1000,
		rateLimitVal: github.RateLimit{
			Remaining: 5000,
			Reset:     time.Now().Add(1 * time.Hour),
		},
	}
}

func (m *mockGitHubClient) repoKey(owner, repo string) string {
	return owner + "/" + repo
}

func (m *mockGitHubClient) commentKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s/%d", owner, repo, number)
}

func (m *mockGitHubClient) ListIssues(ctx context.Context, owner, repo string, opts github.ListOpts) ([]*github.GitHubIssue, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.repoKey(owner, repo)
	issues := m.issues[key]

	// Filter by label if requested.
	if opts.Labels != "" {
		var filtered []*github.GitHubIssue
		for _, iss := range issues {
			for _, l := range iss.Labels {
				if l.Name == opts.Labels {
					filtered = append(filtered, iss)
					break
				}
			}
		}
		issues = filtered
	}

	return issues, "new-etag", nil
}

func (m *mockGitHubClient) CreateIssue(ctx context.Context, owner, repo, title, body string, labels []string) (*github.GitHubIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextIssueNumber++
	num := m.nextIssueNumber

	ghLabels := make([]github.GitHubLabel, len(labels))
	for i, l := range labels {
		ghLabels[i] = github.GitHubLabel{Name: l}
	}

	issue := &github.GitHubIssue{
		Number:    num,
		Title:     title,
		Body:      body,
		State:     "open",
		Labels:    ghLabels,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	key := m.repoKey(owner, repo)
	m.issues[key] = append(m.issues[key], issue)

	m.createdIssues = append(m.createdIssues, createdIssueRecord{
		Owner: owner, Repo: repo, Title: title, Body: body, Labels: labels,
	})

	return issue, nil
}

func (m *mockGitHubClient) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.repoKey(owner, repo)
	for _, iss := range m.issues[key] {
		if iss.Number == number {
			iss.Body = body
			return nil
		}
	}
	return fmt.Errorf("issue %d not found", number)
}

func (m *mockGitHubClient) ListComments(ctx context.Context, owner, repo string, number int, opts github.ListOpts) ([]*github.GitHubComment, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.commentKey(owner, repo, number)
	comments := m.comments[key]

	// Filter by since if provided.
	if opts.Since != "" {
		sinceTime, err := time.Parse(time.RFC3339, opts.Since)
		if err == nil {
			var filtered []*github.GitHubComment
			for _, c := range comments {
				if !c.CreatedAt.Before(sinceTime) {
					filtered = append(filtered, c)
				}
			}
			comments = filtered
		}
	}

	return comments, "comment-etag", nil
}

func (m *mockGitHubClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (*github.GitHubComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextCommentID++
	comment := &github.GitHubComment{
		ID:        m.nextCommentID,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}

	key := m.commentKey(owner, repo, number)
	m.comments[key] = append(m.comments[key], comment)

	m.createdComments = append(m.createdComments, createdCommentRecord{
		Owner: owner, Repo: repo, Number: number, Body: body,
	})

	return comment, nil
}

func (m *mockGitHubClient) GetIssue(ctx context.Context, owner, repo string, number int) (*github.GitHubIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.repoKey(owner, repo)
	for _, iss := range m.issues[key] {
		if iss.Number == number {
			return iss, nil
		}
	}
	return nil, fmt.Errorf("issue %d not found", number)
}

func (m *mockGitHubClient) CreateLabel(ctx context.Context, owner, repo, name, color, description string) error {
	return nil
}

func (m *mockGitHubClient) GetRateLimit() github.RateLimit {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rateLimitVal
}

// addGitHubIssue adds a pre-existing issue to the mock (simulating web-created issues).
func (m *mockGitHubClient) addGitHubIssue(owner, repo string, issue *github.GitHubIssue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.repoKey(owner, repo)
	m.issues[key] = append(m.issues[key], issue)
}

// addGitHubComment adds a pre-existing comment to the mock.
func (m *mockGitHubClient) addGitHubComment(owner, repo string, number int, comment *github.GitHubComment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.commentKey(owner, repo, number)
	m.comments[key] = append(m.comments[key], comment)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func setupTest(t *testing.T) (store.Store, *mockGitHubClient, *model.RepoConfig) {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	gh := newMockGitHubClient()

	ctx := context.Background()
	repo, err := s.AddRepo(ctx, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("failed to add repo: %v", err)
	}

	return s, gh, repo
}

func makeCreatePayload(title, desc string) string {
	p := model.EventPayload{
		Title:       title,
		Description: desc,
	}
	data, _ := json.Marshal(p)
	return string(data)
}

func makeStatusChangePayload(status model.Status) string {
	p := model.EventPayload{
		Status: status,
	}
	data, _ := json.Marshal(p)
	return string(data)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPushOutbound_CommentPosted(t *testing.T) {
	s, gh, repo := setupTest(t)
	ctx := context.Background()

	// Create a local issue with a GitHub ID already set.
	ghID := 42
	issue := &model.Issue{
		RepoID:    repo.ID,
		GitHubID:  &ghID,
		Title:     "Test Issue",
		Status:    model.StatusOpen,
		IssueType: model.IssueTypeTask,
		Labels:    []string{},
	}
	created, err := s.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Create a pending event (status change).
	ev := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: time.Now().UTC(),
		Action:    model.ActionStatusChange,
		Payload:   makeStatusChangePayload(model.StatusInProgress),
		Agent:     "test",
		Synced:    0,
	}
	appended, err := s.AppendEvent(ctx, ev)
	if err != nil {
		t.Fatalf("append event: %v", err)
	}

	// Run push.
	sm := NewSyncManager(s, gh)
	rs := newRepoSyncer(repo, s, gh, sm, 5*time.Second)
	if err := rs.pushOutbound(ctx); err != nil {
		t.Fatalf("pushOutbound: %v", err)
	}

	// Verify comment was posted.
	if len(gh.createdComments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(gh.createdComments))
	}
	if gh.createdComments[0].Number != 42 {
		t.Errorf("expected comment on issue #42, got #%d", gh.createdComments[0].Number)
	}

	// Verify event is now synced.
	synced, err := s.ListEvents(ctx, repo.ID, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	found := false
	for _, e := range synced {
		if e.ID == appended.ID {
			if e.Synced != 1 {
				t.Errorf("expected event to be synced, got synced=%d", e.Synced)
			}
			if e.GitHubCommentID == nil {
				t.Error("expected github_comment_id to be set")
			}
			found = true
		}
	}
	if !found {
		t.Error("did not find the appended event")
	}
}

func TestPushOutbound_CreateIssue(t *testing.T) {
	s, gh, repo := setupTest(t)
	ctx := context.Background()

	// Create a local issue WITHOUT a GitHub ID.
	issue := &model.Issue{
		RepoID:      repo.ID,
		Title:       "New Issue",
		Description: "description",
		Status:      model.StatusOpen,
		IssueType:   model.IssueTypeTask,
		Labels:      []string{"bug"},
	}
	created, err := s.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Create a pending "create" event.
	ev := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: time.Now().UTC(),
		Action:    model.ActionCreate,
		Payload:   makeCreatePayload("New Issue", "description"),
		Agent:     "test",
		Synced:    0,
	}
	appended, err := s.AppendEvent(ctx, ev)
	if err != nil {
		t.Fatalf("append event: %v", err)
	}

	// Run push.
	sm := NewSyncManager(s, gh)
	rs := newRepoSyncer(repo, s, gh, sm, 5*time.Second)
	if err := rs.pushOutbound(ctx); err != nil {
		t.Fatalf("pushOutbound: %v", err)
	}

	// Verify GitHub issue was created.
	if len(gh.createdIssues) != 1 {
		t.Fatalf("expected 1 issue created, got %d", len(gh.createdIssues))
	}
	if gh.createdIssues[0].Title != "New Issue" {
		t.Errorf("expected title 'New Issue', got '%s'", gh.createdIssues[0].Title)
	}

	// Verify local issue now has GitHubID.
	updated, err := s.GetIssue(ctx, created.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if updated.GitHubID == nil {
		t.Fatal("expected GitHubID to be set")
	}

	// Verify the create event is synced.
	syncedEv, _ := s.ListEvents(ctx, repo.ID, created.ID)
	for _, e := range syncedEv {
		if e.ID == appended.ID {
			if e.Synced != 1 {
				t.Errorf("expected create event to be synced")
			}
		}
	}

	// Verify comment was posted (the create event comment).
	if len(gh.createdComments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(gh.createdComments))
	}
}

func TestPullInbound_NewComments(t *testing.T) {
	s, gh, repo := setupTest(t)
	ctx := context.Background()

	// Set up a local issue with a GitHub ID.
	ghID := 10
	issue := &model.Issue{
		RepoID:    repo.ID,
		GitHubID:  &ghID,
		Title:     "Existing Issue",
		Status:    model.StatusOpen,
		IssueType: model.IssueTypeTask,
		Labels:    []string{},
	}
	created, err := s.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Also append a create event (required for the issue to exist in the engine).
	createEv := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: time.Now().UTC().Add(-1 * time.Hour),
		Action:    model.ActionCreate,
		Payload:   makeCreatePayload("Existing Issue", ""),
		Agent:     "test",
		Synced:    1,
	}
	if _, err := s.AppendEvent(ctx, createEv); err != nil {
		t.Fatalf("append create event: %v", err)
	}

	// Add a GitHub issue and a new comment (status_change event).
	ghIssue := &github.GitHubIssue{
		Number:    10,
		Title:     "Existing Issue",
		State:     "open",
		Labels:    []github.GitHubLabel{{Name: "boxofrocks"}},
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
		UpdatedAt: time.Now().UTC(),
	}
	gh.addGitHubIssue("testowner", "testrepo", ghIssue)

	// Create an boxofrocks comment.
	statusEv := &model.Event{
		Timestamp: time.Now().UTC(),
		Action:    model.ActionStatusChange,
		Payload:   makeStatusChangePayload(model.StatusInProgress),
		Agent:     "remote-agent",
	}
	commentBody := github.FormatEventComment(statusEv)
	gh.addGitHubComment("testowner", "testrepo", 10, &github.GitHubComment{
		ID:        5001,
		Body:      commentBody,
		CreatedAt: time.Now().UTC(),
	})

	// Run pull.
	sm := NewSyncManager(s, gh)
	rs := newRepoSyncer(repo, s, gh, sm, 5*time.Second)
	if err := rs.pullInbound(ctx); err != nil {
		t.Fatalf("pullInbound: %v", err)
	}

	// Verify the local issue was updated.
	updated, err := s.GetIssue(ctx, created.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if updated.Status != model.StatusInProgress {
		t.Errorf("expected status in_progress, got %s", updated.Status)
	}

	// Verify the event was appended.
	events, err := s.ListEvents(ctx, repo.ID, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
}

func TestPullInbound_WebCreatedIssue(t *testing.T) {
	s, gh, repo := setupTest(t)
	ctx := context.Background()

	// Add a GitHub issue (created on the web) that has no local counterpart.
	ghIssue := &github.GitHubIssue{
		Number:    99,
		Title:     "Web Created Issue",
		Body:      "Created via GitHub web UI",
		State:     "open",
		Labels:    []github.GitHubLabel{{Name: "boxofrocks"}},
		CreatedAt: time.Now().UTC().Add(-30 * time.Minute),
		UpdatedAt: time.Now().UTC(),
	}
	gh.addGitHubIssue("testowner", "testrepo", ghIssue)

	// Run pull.
	sm := NewSyncManager(s, gh)
	rs := newRepoSyncer(repo, s, gh, sm, 5*time.Second)
	if err := rs.pullInbound(ctx); err != nil {
		t.Fatalf("pullInbound: %v", err)
	}

	// Verify a local issue was created.
	issues, err := s.ListIssues(ctx, store.IssueFilter{RepoID: repo.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Title != "Web Created Issue" {
		t.Errorf("expected title 'Web Created Issue', got '%s'", issues[0].Title)
	}
	if issues[0].GitHubID == nil || *issues[0].GitHubID != 99 {
		t.Errorf("expected GitHubID=99, got %v", issues[0].GitHubID)
	}

	// Verify a synthetic create event was posted as comment.
	if len(gh.createdComments) < 1 {
		t.Fatal("expected at least 1 comment posted for the synthetic create event")
	}
}

func TestPullInbound_Incremental(t *testing.T) {
	s, gh, repo := setupTest(t)
	ctx := context.Background()

	// Set up a local issue.
	ghID := 20
	issue := &model.Issue{
		RepoID:    repo.ID,
		GitHubID:  &ghID,
		Title:     "Incremental Test",
		Status:    model.StatusOpen,
		IssueType: model.IssueTypeTask,
		Labels:    []string{},
	}
	created, err := s.CreateIssue(ctx, issue)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Append a create event.
	createEv := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: time.Now().UTC().Add(-2 * time.Hour),
		Action:    model.ActionCreate,
		Payload:   makeCreatePayload("Incremental Test", ""),
		Agent:     "test",
		Synced:    1,
	}
	if _, err := s.AppendEvent(ctx, createEv); err != nil {
		t.Fatalf("append create event: %v", err)
	}

	// Set sync state: we have already processed comment 100.
	if err := s.SetIssueSyncState(ctx, repo.ID, 20, 100, time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("set sync state: %v", err)
	}

	// Add GitHub issue.
	ghIssue := &github.GitHubIssue{
		Number:    20,
		Title:     "Incremental Test",
		State:     "open",
		Labels:    []github.GitHubLabel{{Name: "boxofrocks"}},
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		UpdatedAt: time.Now().UTC(),
	}
	gh.addGitHubIssue("testowner", "testrepo", ghIssue)

	// Add an old comment (ID <= 100, should be skipped).
	oldEv := &model.Event{
		Timestamp: time.Now().UTC().Add(-1 * time.Hour),
		Action:    model.ActionStatusChange,
		Payload:   makeStatusChangePayload(model.StatusInProgress),
		Agent:     "old-agent",
	}
	gh.addGitHubComment("testowner", "testrepo", 20, &github.GitHubComment{
		ID:        100,
		Body:      github.FormatEventComment(oldEv),
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	})

	// Add a new comment (ID > 100, should be processed).
	newEv := &model.Event{
		Timestamp: time.Now().UTC(),
		Action:    model.ActionAssign,
		Payload:   `{"owner":"bob"}`,
		Agent:     "new-agent",
	}
	gh.addGitHubComment("testowner", "testrepo", 20, &github.GitHubComment{
		ID:        200,
		Body:      github.FormatEventComment(newEv),
		CreatedAt: time.Now().UTC(),
	})

	// Run pull.
	sm := NewSyncManager(s, gh)
	rs := newRepoSyncer(repo, s, gh, sm, 5*time.Second)
	if err := rs.pullInbound(ctx); err != nil {
		t.Fatalf("pullInbound: %v", err)
	}

	// Verify only the new event was applied (assign).
	updated, err := s.GetIssue(ctx, created.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	// The old status_change was skipped, so status should still be open.
	// But the assign event should have set the owner.
	if updated.Owner != "bob" {
		t.Errorf("expected owner 'bob', got '%s'", updated.Owner)
	}

	// Count events: should be 2 (the original create + the new assign).
	events, err := s.ListEvents(ctx, repo.ID, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestForceSync_TriggersImmediateCycle(t *testing.T) {
	s, gh, repo := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo); err != nil {
		t.Fatalf("add repo: %v", err)
	}

	// Give the syncer time to start.
	time.Sleep(100 * time.Millisecond)

	// Force sync should not error.
	if err := sm.ForceSync(repo.ID); err != nil {
		t.Fatalf("force sync: %v", err)
	}

	// Give time for the sync cycle to complete.
	time.Sleep(200 * time.Millisecond)

	// Status should show the repo.
	status := sm.Status()
	if _, ok := status[repo.ID]; !ok {
		t.Fatal("expected repo in status")
	}
}

func TestStatus_ReportsCorrectly(t *testing.T) {
	s, gh, repo := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo); err != nil {
		t.Fatalf("add repo: %v", err)
	}

	// Give the syncer time to run.
	time.Sleep(300 * time.Millisecond)

	status := sm.Status()
	st, ok := status[repo.ID]
	if !ok {
		t.Fatal("expected repo in status")
	}

	if st.RepoName != "testowner/testrepo" {
		t.Errorf("expected repo name 'testowner/testrepo', got '%s'", st.RepoName)
	}
	if st.LastSyncAt == nil {
		t.Error("expected LastSyncAt to be set after sync")
	}
}

func TestMultiRepo(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	gh := newMockGitHubClient()
	ctx := context.Background()

	repo1, err := s.AddRepo(ctx, "owner1", "repo1")
	if err != nil {
		t.Fatalf("add repo1: %v", err)
	}
	repo2, err := s.AddRepo(ctx, "owner2", "repo2")
	if err != nil {
		t.Fatalf("add repo2: %v", err)
	}

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo1); err != nil {
		t.Fatalf("add repo1 to sync: %v", err)
	}
	if err := sm.AddRepo(repo2); err != nil {
		t.Fatalf("add repo2 to sync: %v", err)
	}

	// Wait for initial syncs.
	time.Sleep(500 * time.Millisecond)

	status := sm.Status()
	if len(status) != 2 {
		t.Fatalf("expected 2 repos in status, got %d", len(status))
	}

	if _, ok := status[repo1.ID]; !ok {
		t.Error("repo1 not in status")
	}
	if _, ok := status[repo2.ID]; !ok {
		t.Error("repo2 not in status")
	}
}

func TestGenerateSyntheticCreate(t *testing.T) {
	ghIssue := &github.GitHubIssue{
		Number:    55,
		Title:     "Synthetic Test",
		Body:      "Some description",
		State:     "open",
		Labels:    []github.GitHubLabel{{Name: "boxofrocks"}, {Name: "bug"}},
		CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	ev := GenerateSyntheticCreate(ghIssue, 1, 42)

	if ev.Action != model.ActionCreate {
		t.Errorf("expected action 'create', got '%s'", ev.Action)
	}
	if ev.RepoID != 1 {
		t.Errorf("expected repo_id 1, got %d", ev.RepoID)
	}
	if ev.IssueID != 42 {
		t.Errorf("expected issue_id 42, got %d", ev.IssueID)
	}
	if ev.Agent != "github-sync" {
		t.Errorf("expected agent 'github-sync', got '%s'", ev.Agent)
	}
	if ev.GitHubIssueNumber == nil || *ev.GitHubIssueNumber != 55 {
		t.Errorf("expected github_issue_number 55, got %v", ev.GitHubIssueNumber)
	}

	// Verify payload.
	var payload model.EventPayload
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Title != "Synthetic Test" {
		t.Errorf("expected title 'Synthetic Test', got '%s'", payload.Title)
	}
	if payload.Description != "Some description" {
		t.Errorf("expected description 'Some description', got '%s'", payload.Description)
	}
	// Should include "bug" label but not "boxofrocks".
	foundBug := false
	for _, l := range payload.Labels {
		if l == "boxofrocks" {
			t.Error("should not include boxofrocks label in payload")
		}
		if l == "bug" {
			foundBug = true
		}
	}
	if !foundBug {
		t.Error("expected 'bug' label in payload")
	}
}

func TestRemoveRepo(t *testing.T) {
	s, gh, repo := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := sm.RemoveRepo(repo.ID); err != nil {
		t.Fatalf("remove repo: %v", err)
	}

	status := sm.Status()
	if _, ok := status[repo.ID]; ok {
		t.Error("expected repo to be removed from status")
	}

	// Removing again should error.
	if err := sm.RemoveRepo(repo.ID); err == nil {
		t.Error("expected error removing non-existent repo")
	}
}

func TestAddRepo_Duplicate(t *testing.T) {
	s, gh, repo := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo); err != nil {
		t.Fatalf("add repo: %v", err)
	}

	// Adding the same repo again should error.
	if err := sm.AddRepo(repo); err == nil {
		t.Error("expected error adding duplicate repo")
	}
}

func TestProcessNewComments(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	repo, _ := s.AddRepo(ctx, "owner", "repo")

	// Create a local issue.
	issue := &model.Issue{
		RepoID:    repo.ID,
		Title:     "Process Test",
		Status:    model.StatusOpen,
		IssueType: model.IssueTypeTask,
		Labels:    []string{},
	}
	created, _ := s.CreateIssue(ctx, issue)

	// Append a create event (needed so engine.Apply works).
	createEv := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: time.Now().UTC().Add(-1 * time.Hour),
		Action:    model.ActionCreate,
		Payload:   makeCreatePayload("Process Test", ""),
		Agent:     "test",
		Synced:    1,
	}
	s.AppendEvent(ctx, createEv)

	// Build a status change comment.
	statusEv := &model.Event{
		Timestamp: time.Now().UTC(),
		Action:    model.ActionStatusChange,
		Payload:   makeStatusChangePayload(model.StatusInProgress),
		Agent:     "test-agent",
	}
	comments := []*github.GitHubComment{
		{
			ID:        3001,
			Body:      github.FormatEventComment(statusEv),
			CreatedAt: time.Now().UTC(),
		},
	}

	updated, err := ProcessNewComments(ctx, created, comments, s, repo.ID, 42)
	if err != nil {
		t.Fatalf("ProcessNewComments: %v", err)
	}

	if updated.Status != model.StatusInProgress {
		t.Errorf("expected status in_progress, got %s", updated.Status)
	}
}

func TestForceSyncFull(t *testing.T) {
	s, gh, repo := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.AddRepo(repo); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// ForceSyncFull should not error.
	if err := sm.ForceSyncFull(repo.ID); err != nil {
		t.Fatalf("force sync full: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify it's still in status.
	status := sm.Status()
	if _, ok := status[repo.ID]; !ok {
		t.Fatal("expected repo in status after full sync")
	}
}

func TestForceSync_NonExistentRepo(t *testing.T) {
	s, gh, _ := setupTest(t)

	sm := NewSyncManager(s, gh)
	defer sm.Stop()

	if err := sm.ForceSync(999); err == nil {
		t.Error("expected error for non-existent repo")
	}
	if err := sm.ForceSyncFull(999); err == nil {
		t.Error("expected error for non-existent repo")
	}
}
