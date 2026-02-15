package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// fixtureFile represents the JSON structure of a test fixture.
type fixtureFile struct {
	Name     string                     `json:"name"`
	Events   []*model.Event             `json:"events"`
	Expected map[string]json.RawMessage `json:"expected"`
}

// expectedIssue is a local struct for deserializing expected issue state from JSON.
// It mirrors model.Issue but with pointer fields for optional checking.
type expectedIssue struct {
	ID          int        `json:"id"`
	RepoID      int        `json:"repo_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	IssueType   string     `json:"issue_type"`
	Description string     `json:"description"`
	Owner       string     `json:"owner"`
	Labels      []string   `json:"labels"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

func loadFixture(t *testing.T, name string) fixtureFile {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to unmarshal fixture %s: %v", name, err)
	}
	return f
}

func assertIssueMatches(t *testing.T, issueID string, got *model.Issue, raw json.RawMessage) {
	t.Helper()
	var want expectedIssue
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("failed to unmarshal expected issue %s: %v", issueID, err)
	}

	if got.ID != want.ID {
		t.Errorf("issue %s: ID = %d, want %d", issueID, got.ID, want.ID)
	}
	if got.RepoID != want.RepoID {
		t.Errorf("issue %s: RepoID = %d, want %d", issueID, got.RepoID, want.RepoID)
	}
	if got.Title != want.Title {
		t.Errorf("issue %s: Title = %q, want %q", issueID, got.Title, want.Title)
	}
	if string(got.Status) != want.Status {
		t.Errorf("issue %s: Status = %q, want %q", issueID, got.Status, want.Status)
	}
	if got.Priority != want.Priority {
		t.Errorf("issue %s: Priority = %d, want %d", issueID, got.Priority, want.Priority)
	}
	if string(got.IssueType) != want.IssueType {
		t.Errorf("issue %s: IssueType = %q, want %q", issueID, got.IssueType, want.IssueType)
	}
	if got.Description != want.Description {
		t.Errorf("issue %s: Description = %q, want %q", issueID, got.Description, want.Description)
	}
	if got.Owner != want.Owner {
		t.Errorf("issue %s: Owner = %q, want %q", issueID, got.Owner, want.Owner)
	}

	// Compare labels.
	if len(got.Labels) != len(want.Labels) {
		t.Errorf("issue %s: Labels length = %d, want %d", issueID, len(got.Labels), len(want.Labels))
	} else {
		for i, l := range got.Labels {
			if l != want.Labels[i] {
				t.Errorf("issue %s: Labels[%d] = %q, want %q", issueID, i, l, want.Labels[i])
			}
		}
	}

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("issue %s: CreatedAt = %v, want %v", issueID, got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("issue %s: UpdatedAt = %v, want %v", issueID, got.UpdatedAt, want.UpdatedAt)
	}

	// Compare ClosedAt.
	if want.ClosedAt == nil {
		if got.ClosedAt != nil {
			t.Errorf("issue %s: ClosedAt = %v, want nil", issueID, got.ClosedAt)
		}
	} else {
		if got.ClosedAt == nil {
			t.Errorf("issue %s: ClosedAt = nil, want %v", issueID, want.ClosedAt)
		} else if !got.ClosedAt.Equal(*want.ClosedAt) {
			t.Errorf("issue %s: ClosedAt = %v, want %v", issueID, *got.ClosedAt, *want.ClosedAt)
		}
	}
}

func runFixture(t *testing.T, name string) {
	t.Helper()
	f := loadFixture(t, name)
	t.Logf("Fixture: %s", f.Name)

	result, err := Replay(f.Events)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(result) != len(f.Expected) {
		t.Fatalf("Replay returned %d issues, want %d", len(result), len(f.Expected))
	}

	for idStr, raw := range f.Expected {
		var id int
		if err := json.Unmarshal([]byte(idStr), &id); err != nil {
			t.Fatalf("invalid issue ID key %q in expected: %v", idStr, err)
		}
		got, ok := result[id]
		if !ok {
			t.Fatalf("issue %d not found in replay result", id)
		}
		assertIssueMatches(t, idStr, got, raw)
	}
}

// --- Fixture-driven tests ---

func TestReplay_Empty(t *testing.T) {
	runFixture(t, "empty.json")
}

func TestReplay_SingleCreate(t *testing.T) {
	runFixture(t, "single_create.json")
}

func TestReplay_FullLifecycle(t *testing.T) {
	runFixture(t, "full_lifecycle.json")
}

func TestReplay_InvalidTransitions(t *testing.T) {
	runFixture(t, "invalid_transitions.json")
}

func TestReplay_MultipleIssues(t *testing.T) {
	runFixture(t, "multiple_issues.json")
}

// --- Apply matches Replay ---

func TestApplyMatchesReplay(t *testing.T) {
	f := loadFixture(t, "full_lifecycle.json")

	// Replay path.
	replayResult, err := Replay(f.Events)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	// Incremental Apply path.
	issues := make(map[int]*model.Issue)
	for _, ev := range f.Events {
		updated, err := Apply(issues[ev.IssueID], ev)
		if err != nil {
			t.Fatalf("Apply failed for event %d: %v", ev.ID, err)
		}
		issues[ev.IssueID] = updated
	}

	// Compare results.
	if len(issues) != len(replayResult) {
		t.Fatalf("Apply produced %d issues, Replay produced %d", len(issues), len(replayResult))
	}
	for id, replayIssue := range replayResult {
		applyIssue, ok := issues[id]
		if !ok {
			t.Fatalf("issue %d missing from Apply result", id)
		}
		// Marshal both to JSON for deep comparison.
		rJSON, _ := json.Marshal(replayIssue)
		aJSON, _ := json.Marshal(applyIssue)
		if string(rJSON) != string(aJSON) {
			t.Errorf("issue %d mismatch:\nReplay: %s\nApply:  %s", id, rJSON, aJSON)
		}
	}
}

// --- Apply with multiple issues (interleaved) ---

func TestApplyMatchesReplay_MultipleIssues(t *testing.T) {
	f := loadFixture(t, "multiple_issues.json")

	replayResult, err := Replay(f.Events)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	issues := make(map[int]*model.Issue)
	for _, ev := range f.Events {
		updated, err := Apply(issues[ev.IssueID], ev)
		if err != nil {
			t.Fatalf("Apply failed for event %d: %v", ev.ID, err)
		}
		issues[ev.IssueID] = updated
	}

	if len(issues) != len(replayResult) {
		t.Fatalf("Apply produced %d issues, Replay produced %d", len(issues), len(replayResult))
	}
	for id, replayIssue := range replayResult {
		applyIssue, ok := issues[id]
		if !ok {
			t.Fatalf("issue %d missing from Apply result", id)
		}
		rJSON, _ := json.Marshal(replayIssue)
		aJSON, _ := json.Marshal(applyIssue)
		if string(rJSON) != string(aJSON) {
			t.Errorf("issue %d mismatch:\nReplay: %s\nApply:  %s", id, rJSON, aJSON)
		}
	}
}

// --- Duplicate create error ---

func TestReplay_DuplicateCreate(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []*model.Event{
		{
			ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
			Action:  model.ActionCreate,
			Payload: `{"title":"Issue 1"}`,
		},
		{
			ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
			Action:  model.ActionCreate,
			Payload: `{"title":"Issue 1 duplicate"}`,
		},
	}

	_, err := Replay(events)
	if err == nil {
		t.Fatal("expected error for duplicate create, got nil")
	}
}

// --- Rules tests ---

func TestFromStatusMatch(t *testing.T) {
	cases := []struct {
		name    string
		current model.Status
		from    model.Status
		want    bool
	}{
		{"match", model.StatusOpen, model.StatusOpen, true},
		{"mismatch", model.StatusInProgress, model.StatusOpen, false},
		{"empty_from_legacy", model.StatusOpen, "", true},
		{"empty_from_legacy_in_progress", model.StatusInProgress, "", true},
		{"blocked_match", model.StatusBlocked, model.StatusBlocked, true},
		{"in_review_match", model.StatusInReview, model.StatusInReview, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromStatusMatch(tc.current, tc.from)
			if got != tc.want {
				t.Errorf("FromStatusMatch(%q, %q) = %v, want %v", tc.current, tc.from, got, tc.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	cases := []struct {
		status model.Status
		want   bool
	}{
		{model.StatusOpen, false},
		{model.StatusInProgress, false},
		{model.StatusBlocked, false},
		{model.StatusInReview, false},
		{model.StatusClosed, false},
		{model.StatusDeleted, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := IsTerminal(tc.status)
			if got != tc.want {
				t.Errorf("IsTerminal(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// --- Reopen from closed ---

func TestApply_ReopenFromClosed(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Start with a create.
	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Reopen test","description":"testing reopen"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Close it.
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionClose,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusClosed {
		t.Fatalf("status = %q after close, want %q", issue.Status, model.StatusClosed)
	}
	if issue.ClosedAt == nil {
		t.Fatal("ClosedAt should be set after close")
	}

	// Reopen.
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionReopen,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusOpen {
		t.Errorf("status = %q after reopen, want %q", issue.Status, model.StatusOpen)
	}
	if issue.ClosedAt != nil {
		t.Errorf("ClosedAt should be nil after reopen, got %v", issue.ClosedAt)
	}
	if !issue.UpdatedAt.Equal(ts.Add(2 * time.Hour)) {
		t.Errorf("UpdatedAt = %v, want %v", issue.UpdatedAt, ts.Add(2*time.Hour))
	}
}

// --- Reopen from non-closed is silently ignored ---

func TestApply_ReopenFromOpenIgnored(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Reopen ignored test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try to reopen from "open" -- should be silently ignored.
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionReopen,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusOpen {
		t.Errorf("status = %q, want %q (unchanged)", issue.Status, model.StatusOpen)
	}
}

// --- Delete from various states ---

func TestApply_DeleteFromOpen(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Delete from open"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusDeleted {
		t.Errorf("status = %q, want %q", issue.Status, model.StatusDeleted)
	}
}

func TestApply_DeleteFromInProgress(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Delete from in_progress"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"in_progress"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusDeleted {
		t.Errorf("status = %q, want %q", issue.Status, model.StatusDeleted)
	}
}

func TestApply_DeleteFromClosed(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Delete from closed"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionClose,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusDeleted {
		t.Errorf("status = %q, want %q", issue.Status, model.StatusDeleted)
	}
}

// --- Delete from deleted is silently ignored ---

func TestApply_DeleteFromDeletedIgnored(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Already deleted"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Delete again -- should be silently ignored (no valid transitions out of deleted).
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusDeleted {
		t.Errorf("status = %q, want %q", issue.Status, model.StatusDeleted)
	}
}

// --- Update patches only non-zero fields ---

func TestApply_UpdatePartialPatch(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Original Title","description":"Original Desc","priority":3,"issue_type":"bug","labels":["a","b"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Update only title and labels.
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionUpdate,
		Payload: `{"title":"Updated Title","labels":["x"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if issue.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", issue.Title, "Updated Title")
	}
	if issue.Description != "Original Desc" {
		t.Errorf("Description = %q, want %q (unchanged)", issue.Description, "Original Desc")
	}
	if issue.Priority != 3 {
		t.Errorf("Priority = %d, want 3 (unchanged)", issue.Priority)
	}
	if string(issue.IssueType) != "bug" {
		t.Errorf("IssueType = %q, want %q (unchanged)", issue.IssueType, "bug")
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "x" {
		t.Errorf("Labels = %v, want [x]", issue.Labels)
	}
}

// --- Unknown action error ---

func TestApply_UnknownAction(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.Action("unknown_action"),
		Payload: `{}`,
	})
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
}

// --- Close from already closed is silently ignored ---

func TestApply_CloseFromClosedIgnored(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Close twice"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionClose,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	closedAt := *issue.ClosedAt

	// Close again -- closed->closed is not a valid transition.
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionClose,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if issue.Status != model.StatusClosed {
		t.Errorf("status = %q, want %q", issue.Status, model.StatusClosed)
	}
	if !issue.ClosedAt.Equal(closedAt) {
		t.Errorf("ClosedAt changed from %v to %v, should be unchanged", closedAt, *issue.ClosedAt)
	}
}

// --- Apply errors on nil issue for non-create actions ---

func TestApply_NilIssueErrors(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	actions := []model.Action{
		model.ActionStatusChange,
		model.ActionAssign,
		model.ActionClose,
		model.ActionUpdate,
		model.ActionDelete,
		model.ActionReopen,
	}

	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			_, err := Apply(nil, &model.Event{
				ID: 1, RepoID: 1, IssueID: 99, Timestamp: ts,
				Action:  action,
				Payload: `{}`,
			})
			if err == nil {
				t.Errorf("expected error for %s on nil issue, got nil", action)
			}
		})
	}
}

// --- From-status validation tests ---

func TestApply_StatusChangeWithFromStatus(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"FromStatus test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Correct from_status: accepted.
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"in_progress","from_status":"open"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusInProgress {
		t.Errorf("status = %q, want in_progress", issue.Status)
	}

	// Wrong from_status: skipped.
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"closed","from_status":"open"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusInProgress {
		t.Errorf("status = %q after stale event, want in_progress (unchanged)", issue.Status)
	}
}

func TestApply_StatusChangeWithoutFromStatus_LegacyAccepted(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Legacy test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No from_status (legacy): accepted.
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"in_progress"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusInProgress {
		t.Errorf("status = %q, want in_progress", issue.Status)
	}
}

func TestApply_NewStatuses(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"New statuses test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// open -> blocked
	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"blocked","from_status":"open"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusBlocked {
		t.Errorf("status = %q, want blocked", issue.Status)
	}

	// blocked -> in_review
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"in_review","from_status":"blocked"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusInReview {
		t.Errorf("status = %q, want in_review", issue.Status)
	}

	// in_review -> closed
	issue, err = Apply(issue, &model.Event{
		ID: 4, RepoID: 1, IssueID: 1, Timestamp: ts.Add(3 * time.Hour),
		Action:  model.ActionClose,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusClosed {
		t.Errorf("status = %q, want closed", issue.Status)
	}
}

func TestApply_EpicIssueType(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Epic test","issue_type":"epic"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.IssueType != model.IssueTypeEpic {
		t.Errorf("issue_type = %q, want epic", issue.IssueType)
	}
}

func TestApply_DeletedTerminal_StatusChangeSkipped(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	issue, err := Apply(nil, &model.Event{
		ID: 1, RepoID: 1, IssueID: 1, Timestamp: ts,
		Action:  model.ActionCreate,
		Payload: `{"title":"Terminal test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err = Apply(issue, &model.Event{
		ID: 2, RepoID: 1, IssueID: 1, Timestamp: ts.Add(time.Hour),
		Action:  model.ActionDelete,
		Payload: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try status_change on deleted issue with correct from_status — should be skipped.
	issue, err = Apply(issue, &model.Event{
		ID: 3, RepoID: 1, IssueID: 1, Timestamp: ts.Add(2 * time.Hour),
		Action:  model.ActionStatusChange,
		Payload: `{"status":"open","from_status":"deleted"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != model.StatusDeleted {
		t.Errorf("status = %q, want deleted (terminal, should block all changes)", issue.Status)
	}
}

// --- Legacy fixture test ---

func TestReplay_LegacyNoFromStatus(t *testing.T) {
	runFixture(t, "legacy_no_from_status.json")
}
