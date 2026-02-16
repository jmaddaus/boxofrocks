package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/config"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
	borSync "github.com/jmaddaus/boxofrocks/internal/sync"
)

// testDaemon creates a Daemon backed by an in-memory SQLite store for testing.
func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		ListenAddr: ":0",
		DataDir:    t.TempDir(),
		DBPath:     ":memory:",
	}

	return NewWithStore(cfg, s)
}

// doRequest is a helper that sends an HTTP request and returns the response.
func doRequest(t *testing.T, d *Daemon, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	d.Handler().ServeHTTP(rr, req)
	return rr
}

// doRequestWithHeader is like doRequest but adds a custom header.
func doRequestWithHeader(t *testing.T, d *Daemon, method, path, headerKey, headerVal string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(headerKey, headerVal)
	rr := httptest.NewRecorder()
	d.Handler().ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	d := testDaemon(t)

	rr := doRequest(t, d, "GET", "/health", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestCreateAndListRepos(t *testing.T) {
	d := testDaemon(t)

	// Create a repo.
	rr := doRequest(t, d, "POST", "/repos", map[string]string{
		"owner": "testorg",
		"name":  "testrepo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var repo model.RepoConfig
	decodeJSON(t, rr, &repo)
	if repo.Owner != "testorg" || repo.Name != "testrepo" {
		t.Errorf("unexpected repo: %+v", repo)
	}

	// List repos.
	rr = doRequest(t, d, "GET", "/repos", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list repos: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var repos []*model.RepoConfig
	decodeJSON(t, rr, &repos)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].FullName() != "testorg/testrepo" {
		t.Errorf("unexpected repo name: %s", repos[0].FullName())
	}
}

func TestCreateGetListIssues(t *testing.T) {
	d := testDaemon(t)

	// Register a repo first.
	doRequest(t, d, "POST", "/repos", map[string]string{
		"owner": "org", "name": "repo",
	})

	// Create an issue.
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":       "Test Issue",
		"description": "A test issue",
		"priority":    1,
		"issue_type":  "bug",
		"labels":      []string{"backend"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create issue: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created model.Issue
	decodeJSON(t, rr, &created)
	if created.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %q", created.Title)
	}
	if created.Status != model.StatusOpen {
		t.Errorf("expected status open, got %q", created.Status)
	}
	if created.Priority != 1 {
		t.Errorf("expected priority 1, got %d", created.Priority)
	}

	// Get issue by ID.
	rr = doRequest(t, d, "GET", "/issues/"+itoa(created.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get issue: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var fetched model.Issue
	decodeJSON(t, rr, &fetched)
	if fetched.ID != created.ID {
		t.Errorf("expected id %d, got %d", created.ID, fetched.ID)
	}

	// List issues.
	rr = doRequest(t, d, "GET", "/issues", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list issues: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var issues []*model.Issue
	decodeJSON(t, rr, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

func TestUpdateIssueTitleChange(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Original Title",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Update title.
	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"title": "Updated Title",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update issue: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if updated.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %q", updated.Title)
	}
}

func TestUpdateIssueStatusChange(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Status Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Change status to in_progress.
	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"status": "in_progress",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update status: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if updated.Status != model.StatusInProgress {
		t.Errorf("expected in_progress, got %q", updated.Status)
	}

	// Change status to closed.
	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"status": "closed",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("close issue: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	decodeJSON(t, rr, &updated)
	if updated.Status != model.StatusClosed {
		t.Errorf("expected closed, got %q", updated.Status)
	}
	if updated.ClosedAt == nil {
		t.Error("expected ClosedAt to be set")
	}
}

func TestDeleteIssueSoftDelete(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Delete Me",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Delete the issue.
	rr = doRequest(t, d, "DELETE", "/issues/"+itoa(iss.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var deleted model.Issue
	decodeJSON(t, rr, &deleted)
	if deleted.Status != model.StatusDeleted {
		t.Errorf("expected deleted status, got %q", deleted.Status)
	}

	// List issues should NOT include deleted.
	rr = doRequest(t, d, "GET", "/issues", nil)
	var issues []*model.Issue
	decodeJSON(t, rr, &issues)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (deleted excluded), got %d", len(issues))
	}

	// List issues with ?all=true should include deleted.
	rr = doRequest(t, d, "GET", "/issues?all=true", nil)
	decodeJSON(t, rr, &issues)
	if len(issues) != 1 {
		t.Errorf("expected 1 issue (all=true), got %d", len(issues))
	}
}

func TestAssignIssue(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Assign Me",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Assign.
	rr = doRequest(t, d, "POST", "/issues/"+itoa(iss.ID)+"/assign", map[string]string{
		"owner": "alice",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("assign: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var assigned model.Issue
	decodeJSON(t, rr, &assigned)
	if assigned.Owner != "alice" {
		t.Errorf("expected owner 'alice', got %q", assigned.Owner)
	}
}

func TestNextIssueReturnsHighestPriority(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	// Create two issues with different priorities.
	doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":    "Low priority",
		"priority": 10,
	})
	doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":    "High priority",
		"priority": 1,
	})

	rr := doRequest(t, d, "GET", "/issues/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("next: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var next model.Issue
	decodeJSON(t, rr, &next)
	if next.Title != "High priority" {
		t.Errorf("expected 'High priority', got %q", next.Title)
	}
}

func TestNextIssueReturns404WhenNoneAvailable(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "GET", "/issues/next", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("next (empty): expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNextIssueSkipsAssigned(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	// Create an issue and assign it.
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":    "Assigned issue",
		"priority": 1,
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)
	doRequest(t, d, "POST", "/issues/"+itoa(iss.ID)+"/assign", map[string]string{
		"owner": "bob",
	})

	// Create another unassigned issue.
	doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":    "Unassigned issue",
		"priority": 5,
	})

	rr = doRequest(t, d, "GET", "/issues/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("next: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var next model.Issue
	decodeJSON(t, rr, &next)
	if next.Title != "Unassigned issue" {
		t.Errorf("expected 'Unassigned issue', got %q", next.Title)
	}
}

func TestRepoResolutionQueryParam(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org1", "name": "repo1"})
	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org2", "name": "repo2"})

	// Create issue with explicit repo query param.
	rr := doRequest(t, d, "POST", "/issues?repo=org1/repo1", map[string]interface{}{
		"title": "Issue in repo1",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create with repo param: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// List issues for repo1.
	rr = doRequest(t, d, "GET", "/issues?repo=org1/repo1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list with repo param: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var issues []*model.Issue
	decodeJSON(t, rr, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue in repo1, got %d", len(issues))
	}
}

func TestRepoResolutionXRepoHeader(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org1", "name": "repo1"})
	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org2", "name": "repo2"})

	// Create issue via X-Repo header.
	rr := doRequestWithHeader(t, d, "POST", "/issues", "X-Repo", "org2/repo2", map[string]interface{}{
		"title": "Issue in repo2",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create with X-Repo: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// List issues for repo2.
	rr = doRequestWithHeader(t, d, "GET", "/issues", "X-Repo", "org2/repo2", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list with X-Repo: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var issues []*model.Issue
	decodeJSON(t, rr, &issues)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue in repo2, got %d", len(issues))
	}
}

func TestRepoResolutionSingleRepoImplicit(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "only", "name": "repo"})

	// No repo specified; should implicitly use the single registered repo.
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Implicit repo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create (implicit repo): expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRepoResolutionMultiRepoAmbiguous(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "a", "name": "1"})
	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "b", "name": "2"})

	// No repo specified; should return 400.
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Ambiguous",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("ambiguous repo: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInvalidJSONBody(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	// Send malformed JSON.
	req := httptest.NewRequest("POST", "/issues", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	d.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIssueNotFound(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "GET", "/issues/99999", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("not found: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestForceSyncStub(t *testing.T) {
	d := testDaemon(t)

	rr := doRequest(t, d, "POST", "/sync", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("sync: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "sync not yet implemented" {
		t.Errorf("unexpected sync response: %v", resp)
	}
}

func TestHealthEndpointWithRepos(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org", "name": "repo1"})
	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "org", "name": "repo2"})

	rr := doRequest(t, d, "GET", "/health", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("health: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)

	repos, ok := resp["repos"].([]interface{})
	if !ok {
		t.Fatalf("expected repos array, got %T", resp["repos"])
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos in health, got %d", len(repos))
	}
}

func TestCreateIssueRequiresTitle(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"description": "no title",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing title: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteIssueNotFound(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "DELETE", "/issues/99999", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delete not found: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAssignIssueNotFound(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "POST", "/issues/99999/assign", map[string]string{
		"owner": "alice",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("assign not found: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateIssueNotFound(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "PATCH", "/issues/99999", map[string]interface{}{
		"title": "nope",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("update not found: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// itoa is a convenience for tests.
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// ---------------------------------------------------------------------------
// noopGitHubClient implements github.Client for wiring tests.
// ---------------------------------------------------------------------------

type noopGitHubClient struct{}

func (noopGitHubClient) ListIssues(ctx context.Context, owner, repo string, opts github.ListOpts) ([]*github.GitHubIssue, string, error) {
	return nil, "", nil
}
func (noopGitHubClient) GetIssue(ctx context.Context, owner, repo string, number int) (*github.GitHubIssue, error) {
	return nil, fmt.Errorf("not implemented")
}
func (noopGitHubClient) CreateIssue(ctx context.Context, owner, repo, title, body string, labels []string) (*github.GitHubIssue, error) {
	return nil, fmt.Errorf("not implemented")
}
func (noopGitHubClient) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	return fmt.Errorf("not implemented")
}
func (noopGitHubClient) ListComments(ctx context.Context, owner, repo string, number int, opts github.ListOpts) ([]*github.GitHubComment, string, error) {
	return nil, "", nil
}
func (noopGitHubClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (*github.GitHubComment, error) {
	return nil, fmt.Errorf("not implemented")
}
func (noopGitHubClient) CreateLabel(ctx context.Context, owner, repo, name, color, description string) error {
	return nil
}
func (noopGitHubClient) UpdateIssueState(ctx context.Context, owner, repo string, number int, state string) error {
	return nil
}
func (noopGitHubClient) GetRateLimit() github.RateLimit {
	return github.RateLimit{Remaining: 5000, Reset: time.Now().Add(time.Hour)}
}

func TestAddRepoStartsSyncer(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	cfg := &config.Config{
		ListenAddr: ":0",
		DataDir:    t.TempDir(),
		DBPath:     ":memory:",
	}

	sm := borSync.NewSyncManager(s, noopGitHubClient{})

	d := NewWithStoreAndSync(cfg, s, sm)

	rr := doRequest(t, d, "POST", "/repos", map[string]string{
		"owner": "testorg",
		"name":  "testrepo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// AddRepo adds the syncer to the map synchronously (under lock) before
	// spawning the goroutine, so Status() can see it immediately.
	status := sm.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 repo in sync status, got %d", len(status))
	}
	for _, st := range status {
		if st.RepoName != "testorg/testrepo" {
			t.Errorf("expected repo name 'testorg/testrepo', got %q", st.RepoName)
		}
	}

	// Stop the sync manager before closing the store to ensure the syncer
	// goroutine has exited and won't race with store.Close().
	sm.Stop()
	s.Close()
}

func TestUpdateIssueStatusToBlocked(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Blocked Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"status": "blocked",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update to blocked: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if updated.Status != model.StatusBlocked {
		t.Errorf("expected blocked, got %q", updated.Status)
	}
}

func TestUpdateIssueStatusToInReview(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "InReview Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"status": "in_review",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update to in_review: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if updated.Status != model.StatusInReview {
		t.Errorf("expected in_review, got %q", updated.Status)
	}
}

func TestCreateIssueWithEpicType(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title":      "Epic Test",
		"issue_type": "epic",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create epic: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var iss model.Issue
	decodeJSON(t, rr, &iss)
	if iss.IssueType != model.IssueTypeEpic {
		t.Errorf("expected epic, got %q", iss.IssueType)
	}
}

func TestCommentIssue(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Comment Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Add a comment.
	rr = doRequest(t, d, "POST", "/issues/"+itoa(iss.ID)+"/comment", map[string]string{
		"comment": "This is a test comment",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("comment: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var commented model.Issue
	decodeJSON(t, rr, &commented)
	if len(commented.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(commented.Comments))
	}
	if commented.Comments[0].Text != "This is a test comment" {
		t.Errorf("expected comment text 'This is a test comment', got %q", commented.Comments[0].Text)
	}
}

func TestCommentIssueNotFound(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})

	rr := doRequest(t, d, "POST", "/issues/99999/comment", map[string]string{
		"comment": "test",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("comment not found: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCommentIssueEmptyComment(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Empty Comment Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	rr = doRequest(t, d, "POST", "/issues/"+itoa(iss.ID)+"/comment", map[string]string{
		"comment": "",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty comment: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateIssueWithComment(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Update Comment Test",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Update with a comment.
	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"title":   "New Title",
		"comment": "Changed the title",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update with comment: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if updated.Title != "New Title" {
		t.Errorf("expected title 'New Title', got %q", updated.Title)
	}
	if len(updated.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(updated.Comments))
	}
	if updated.Comments[0].Text != "Changed the title" {
		t.Errorf("expected comment text 'Changed the title', got %q", updated.Comments[0].Text)
	}
}

func TestUpdateIssueCommentOnly(t *testing.T) {
	d := testDaemon(t)

	doRequest(t, d, "POST", "/repos", map[string]string{"owner": "o", "name": "r"})
	rr := doRequest(t, d, "POST", "/issues", map[string]interface{}{
		"title": "Comment Only Update",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Update with only a comment (no field changes).
	rr = doRequest(t, d, "PATCH", "/issues/"+itoa(iss.ID), map[string]interface{}{
		"comment": "Just a note",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("comment-only update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Issue
	decodeJSON(t, rr, &updated)
	if len(updated.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(updated.Comments))
	}
	if updated.Comments[0].Text != "Just a note" {
		t.Errorf("expected comment text 'Just a note', got %q", updated.Comments[0].Text)
	}
}

func TestAddRepoWithoutSyncManager(t *testing.T) {
	d := testDaemon(t) // syncMgr is nil

	rr := doRequest(t, d, "POST", "/repos", map[string]string{
		"owner": "testorg",
		"name":  "testrepo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var repo model.RepoConfig
	decodeJSON(t, rr, &repo)
	if repo.Owner != "testorg" || repo.Name != "testrepo" {
		t.Errorf("unexpected repo: %+v", repo)
	}
}
