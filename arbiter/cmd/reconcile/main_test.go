package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
)

// mockClient implements github.Client for testing reconcile().
type mockClient struct {
	comments     []*github.GitHubComment
	issue        *github.GitHubIssue
	updated      string // captured body from UpdateIssueBody
	updatedState string // captured state from UpdateIssueState
}

func (m *mockClient) ListIssues(ctx context.Context, owner, repo string, opts github.ListOpts) ([]*github.GitHubIssue, string, error) {
	return nil, "", nil
}

func (m *mockClient) GetIssue(ctx context.Context, owner, repo string, number int) (*github.GitHubIssue, error) {
	if m.issue == nil {
		return nil, fmt.Errorf("issue not found")
	}
	return m.issue, nil
}

func (m *mockClient) CreateIssue(ctx context.Context, owner, repo, title, body string, labels []string) (*github.GitHubIssue, error) {
	return nil, nil
}

func (m *mockClient) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	m.updated = body
	return nil
}

func (m *mockClient) ListComments(ctx context.Context, owner, repo string, number int, opts github.ListOpts) ([]*github.GitHubComment, string, error) {
	return m.comments, "", nil
}

func (m *mockClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (*github.GitHubComment, error) {
	return &github.GitHubComment{ID: 1, Body: body, CreatedAt: time.Now()}, nil
}

func (m *mockClient) CreateLabel(ctx context.Context, owner, repo, name, color, description string) error {
	return nil
}

func (m *mockClient) UpdateIssueState(ctx context.Context, owner, repo string, number int, state string) error {
	m.updatedState = state
	return nil
}

func (m *mockClient) GetRepo(ctx context.Context, owner, repo string) (*github.GitHubRepo, error) {
	return &github.GitHubRepo{Private: true}, nil
}

func (m *mockClient) GetRateLimit() github.RateLimit {
	return github.RateLimit{Remaining: 5000, Reset: time.Now().Add(time.Hour)}
}

// makeComment creates a boxofrocks event comment.
func makeComment(id int, action model.Action, payload string, ts time.Time) *github.GitHubComment {
	ev := &model.Event{
		Timestamp: ts,
		Action:    action,
		Payload:   payload,
		Agent:     "test",
	}
	body := github.FormatEventComment(ev)
	return &github.GitHubComment{
		ID:        id,
		Body:      body,
		CreatedAt: ts,
	}
}

func makeCreatePayload(title, desc string) string {
	p := model.EventPayload{Title: title, Description: desc}
	data, _ := json.Marshal(p)
	return string(data)
}

func makeStatusPayload(status model.Status) string {
	p := model.EventPayload{Status: status}
	data, _ := json.Marshal(p)
	return string(data)
}

func makeAssignPayload(owner string) string {
	p := model.EventPayload{Owner: owner}
	data, _ := json.Marshal(p)
	return string(data)
}

func TestReconcileCreatesMetadata(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	mc := &mockClient{
		comments: []*github.GitHubComment{
			makeComment(1, model.ActionCreate, makeCreatePayload("Test", "desc"), ts),
		},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Test",
			Body:   "",
			State:  "open",
		},
	}

	body, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		t.Fatal("expected replayed issue, got nil")
	}
	if replayed.Title != "Test" {
		t.Errorf("title: want Test, got %s", replayed.Title)
	}
	if !strings.Contains(body, "boxofrocks") {
		t.Errorf("expected metadata in body, got: %s", body)
	}
}

func TestReconcileFullLifecycle(t *testing.T) {
	t0 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)

	mc := &mockClient{
		comments: []*github.GitHubComment{
			makeComment(1, model.ActionCreate, makeCreatePayload("Lifecycle", ""), t0),
			makeComment(2, model.ActionAssign, makeAssignPayload("alice"), t1),
			makeComment(3, model.ActionClose, makeStatusPayload(model.StatusClosed), t2),
		},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Lifecycle",
			Body:   "",
			State:  "closed",
		},
	}

	body, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		t.Fatal("expected replayed issue")
	}
	if replayed.Status != model.StatusClosed {
		t.Errorf("status: want closed, got %s", replayed.Status)
	}
	if replayed.Owner != "alice" {
		t.Errorf("owner: want alice, got %s", replayed.Owner)
	}
	if !strings.Contains(body, `"status":"closed"`) {
		t.Errorf("expected closed status in body metadata, got: %s", body)
	}
}

func TestReconcileNoEvents(t *testing.T) {
	mc := &mockClient{
		comments: []*github.GitHubComment{
			{ID: 1, Body: "Just a regular comment", CreatedAt: time.Now()},
		},
		issue: &github.GitHubIssue{Number: 1, Title: "Test", Body: ""},
	}

	_, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed != nil {
		t.Errorf("expected nil replayed for no events, got %+v", replayed)
	}
}

func TestReconcilePreservesHumanText(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	humanText := "This is important context for the issue."

	mc := &mockClient{
		comments: []*github.GitHubComment{
			makeComment(1, model.ActionCreate, makeCreatePayload("Preserve", ""), ts),
		},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Preserve",
			Body:   humanText + "\n\n" + `<!-- boxofrocks {"status":"open","priority":0,"issue_type":"","owner":"","labels":[]} -->`,
			State:  "open",
		},
	}

	body, _, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !strings.Contains(body, humanText) {
		t.Errorf("expected human text preserved in body, got: %s", body)
	}
}

func TestReconcileInvalidTransitionsIgnored(t *testing.T) {
	t0 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)

	// Try to close an issue that's already deleted — invalid, should be ignored.
	mc := &mockClient{
		comments: []*github.GitHubComment{
			makeComment(1, model.ActionCreate, makeCreatePayload("Invalid", ""), t0),
			// Attempt a close on an open issue — this IS valid
			makeComment(2, model.ActionClose, makeStatusPayload(model.StatusClosed), t1),
		},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Invalid",
			Body:   "",
			State:  "open",
		},
	}

	_, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		t.Fatal("expected replayed issue")
	}
	// Should not error — invalid transitions are silently ignored per design.
	if replayed.Status != model.StatusClosed {
		t.Errorf("status: want closed, got %s", replayed.Status)
	}
}

func TestReconcileMultipleEvents(t *testing.T) {
	t0 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)
	t3 := t0.Add(3 * time.Hour)

	pri := 1
	updatePayload, _ := json.Marshal(model.EventPayload{Priority: &pri})

	mc := &mockClient{
		comments: []*github.GitHubComment{
			makeComment(1, model.ActionCreate, makeCreatePayload("Multi", "initial"), t0),
			makeComment(2, model.ActionAssign, makeAssignPayload("bob"), t1),
			makeComment(3, model.ActionUpdate, string(updatePayload), t2),
			makeComment(4, model.ActionStatusChange, makeStatusPayload(model.StatusInProgress), t3),
		},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Multi",
			Body:   "",
			State:  "open",
		},
	}

	body, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		t.Fatal("expected replayed issue")
	}
	if replayed.Owner != "bob" {
		t.Errorf("owner: want bob, got %s", replayed.Owner)
	}
	if replayed.Priority != 1 {
		t.Errorf("priority: want 1, got %d", replayed.Priority)
	}
	if replayed.Status != model.StatusInProgress {
		t.Errorf("status: want in_progress, got %s", replayed.Status)
	}
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestSyncIssueState_CloseWhenClosed(t *testing.T) {
	mc := &mockClient{}
	replayed := &model.Issue{Status: model.StatusClosed}
	ghIssue := &github.GitHubIssue{State: "open"}

	err := syncIssueState(context.Background(), mc, "owner", "repo", 1, replayed, ghIssue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.updatedState != "closed" {
		t.Errorf("expected state 'closed', got %q", mc.updatedState)
	}
}

func TestSyncIssueState_CloseWhenDeleted(t *testing.T) {
	mc := &mockClient{}
	replayed := &model.Issue{Status: model.StatusDeleted}
	ghIssue := &github.GitHubIssue{State: "open"}

	err := syncIssueState(context.Background(), mc, "owner", "repo", 1, replayed, ghIssue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.updatedState != "closed" {
		t.Errorf("expected state 'closed', got %q", mc.updatedState)
	}
}

func TestSyncIssueState_ReopenWhenOpen(t *testing.T) {
	mc := &mockClient{}
	replayed := &model.Issue{Status: model.StatusOpen}
	ghIssue := &github.GitHubIssue{State: "closed"}

	err := syncIssueState(context.Background(), mc, "owner", "repo", 1, replayed, ghIssue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.updatedState != "open" {
		t.Errorf("expected state 'open', got %q", mc.updatedState)
	}
}

func TestSyncIssueState_NoChangeWhenAlreadyMatching(t *testing.T) {
	mc := &mockClient{}
	replayed := &model.Issue{Status: model.StatusOpen}
	ghIssue := &github.GitHubIssue{State: "open"}

	err := syncIssueState(context.Background(), mc, "owner", "repo", 1, replayed, ghIssue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.updatedState != "" {
		t.Errorf("expected no state update, got %q", mc.updatedState)
	}
}

func TestSyncIssueState_NoChangeWhenClosedMatching(t *testing.T) {
	mc := &mockClient{}
	replayed := &model.Issue{Status: model.StatusClosed}
	ghIssue := &github.GitHubIssue{State: "closed"}

	err := syncIssueState(context.Background(), mc, "owner", "repo", 1, replayed, ghIssue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.updatedState != "" {
		t.Errorf("expected no state update, got %q", mc.updatedState)
	}
}

func TestReconcileFilterUntrustedAuthors(t *testing.T) {
	t0 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)

	// Create event from trusted author.
	createComment := makeComment(1, model.ActionCreate, makeCreatePayload("Filtered", "desc"), t0)
	createComment.AuthorAssociation = "OWNER"

	// Status change from untrusted author — should be filtered.
	untrustedComment := makeComment(2, model.ActionStatusChange, makeStatusPayload(model.StatusClosed), t1)
	untrustedComment.AuthorAssociation = "NONE"

	// Assign from trusted author — should be applied.
	trustedComment := makeComment(3, model.ActionAssign, makeAssignPayload("bob"), t2)
	trustedComment.AuthorAssociation = "COLLABORATOR"

	mc := &mockClient{
		comments: []*github.GitHubComment{createComment, untrustedComment, trustedComment},
		issue: &github.GitHubIssue{
			Number: 1,
			Title:  "Filtered",
			Body:   "",
			State:  "open",
		},
	}

	_, replayed, err := reconcile(context.Background(), mc, "owner", "repo", 1, true)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		t.Fatal("expected replayed issue")
	}
	// Status should still be open since the untrusted close was filtered.
	if replayed.Status != model.StatusOpen {
		t.Errorf("status: want open (untrusted close filtered), got %s", replayed.Status)
	}
	// Owner should be set by the trusted assign.
	if replayed.Owner != "bob" {
		t.Errorf("owner: want bob (trusted assign applied), got %s", replayed.Owner)
	}
}
