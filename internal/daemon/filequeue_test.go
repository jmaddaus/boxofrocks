package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupQueueTest creates a daemon with a repo that has a queue dir in a temp directory.
// Returns the daemon, the repo, and the queue directory path.
func setupQueueTest(t *testing.T) (*Daemon, *model.RepoConfig, string) {
	t.Helper()
	d := testDaemon(t)

	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, ".boxofrocks", "queue")

	// Register a repo with local path and queue enabled.
	rr := doRequest(t, d, "POST", "/repos", map[string]interface{}{
		"owner":      "org",
		"name":       "repo",
		"local_path": tmpDir,
		"queue":      true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var repo model.RepoConfig
	decodeJSON(t, rr, &repo)

	return d, &repo, queueDir
}

// writeReqFile writes a .req file atomically (via .tmp + rename).
func writeReqFile(t *testing.T, queueDir, id string, freq fileQueueRequest) string {
	t.Helper()
	data, err := json.Marshal(freq)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatalf("mkdir queue: %v", err)
	}

	reqPath := filepath.Join(queueDir, id+".req")
	tmpPath := reqPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Rename(tmpPath, reqPath); err != nil {
		t.Fatalf("rename: %v", err)
	}
	return reqPath
}

// readRespFile reads and parses a .resp file, waiting up to 5 seconds for it to appear.
func readRespFile(t *testing.T, queueDir, id string) fileQueueResponse {
	t.Helper()
	respPath := filepath.Join(queueDir, id+".resp")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(respPath); err == nil {
			data, err := os.ReadFile(respPath)
			if err != nil {
				t.Fatalf("read resp: %v", err)
			}
			var resp fileQueueResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				t.Fatalf("unmarshal resp: %v", err)
			}
			return resp
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for response file %s", respPath)
	return fileQueueResponse{}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFileQueueProcessRequest(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	reqPath := writeReqFile(t, queueDir, "test1", fileQueueRequest{
		Method: "GET",
		Path:   "/health",
	})

	d.processQueueFile(reqPath, repo.ID)

	// .req should be removed.
	if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
		t.Error("expected .req file to be removed")
	}

	// .resp should exist.
	respPath := filepath.Join(queueDir, "test1.resp")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}

	var resp fileQueueResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Status)
	}

	// Check body contains "ok".
	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected health status ok, got %v", body["status"])
	}
}

func TestFileQueueGetIssuesNext(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	// Create issues via HTTP.
	doRequest(t, d, "POST", "/issues?repo=org/repo", map[string]interface{}{
		"title":    "Low priority",
		"priority": 10,
	})
	doRequest(t, d, "POST", "/issues?repo=org/repo", map[string]interface{}{
		"title":    "High priority",
		"priority": 1,
	})

	// Queue a GET /issues/next request.
	reqPath := writeReqFile(t, queueDir, "next1", fileQueueRequest{
		Method: "GET",
		Path:   "/issues/next",
	})

	d.processQueueFile(reqPath, repo.ID)

	respPath := filepath.Join(queueDir, "next1.resp")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}

	var resp fileQueueResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Status)
	}

	var issue model.Issue
	if err := json.Unmarshal(resp.Body, &issue); err != nil {
		t.Fatalf("unmarshal issue: %v", err)
	}
	if issue.Title != "High priority" {
		t.Errorf("expected 'High priority', got %q", issue.Title)
	}
}

func TestFileQueuePostCreate(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	body, _ := json.Marshal(map[string]interface{}{
		"title":       "Created via queue",
		"description": "test",
		"priority":    3,
	})

	reqPath := writeReqFile(t, queueDir, "create1", fileQueueRequest{
		Method: "POST",
		Path:   "/issues",
		Body:   json.RawMessage(body),
	})

	d.processQueueFile(reqPath, repo.ID)

	respPath := filepath.Join(queueDir, "create1.resp")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}

	var resp fileQueueResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Status != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", resp.Status, string(resp.Body))
	}

	var issue model.Issue
	if err := json.Unmarshal(resp.Body, &issue); err != nil {
		t.Fatalf("unmarshal issue: %v", err)
	}
	if issue.Title != "Created via queue" {
		t.Errorf("expected 'Created via queue', got %q", issue.Title)
	}
	if issue.Priority != 3 {
		t.Errorf("expected priority 3, got %d", issue.Priority)
	}
}

func TestFileQueuePatchUpdate(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	// Create an issue.
	rr := doRequest(t, d, "POST", "/issues?repo=org/repo", map[string]interface{}{
		"title": "Update Me",
	})
	var iss model.Issue
	decodeJSON(t, rr, &iss)

	// Queue a PATCH to change status.
	body, _ := json.Marshal(map[string]interface{}{
		"status": "in_progress",
	})

	reqPath := writeReqFile(t, queueDir, "patch1", fileQueueRequest{
		Method: "PATCH",
		Path:   "/issues/" + itoa(iss.ID),
		Body:   json.RawMessage(body),
	})

	d.processQueueFile(reqPath, repo.ID)

	respPath := filepath.Join(queueDir, "patch1.resp")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}

	var resp fileQueueResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", resp.Status, string(resp.Body))
	}

	var updated model.Issue
	if err := json.Unmarshal(resp.Body, &updated); err != nil {
		t.Fatalf("unmarshal issue: %v", err)
	}
	if updated.Status != model.StatusInProgress {
		t.Errorf("expected in_progress, got %q", updated.Status)
	}
}

func TestFileQueueInvalidJSON(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	// Write a malformed request file directly.
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	reqPath := filepath.Join(queueDir, "bad1.req")
	if err := os.WriteFile(reqPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d.processQueueFile(reqPath, repo.ID)

	// .req should be removed.
	if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
		t.Error("expected .req file to be removed after invalid JSON")
	}

	// .resp should contain a 400 error.
	respPath := filepath.Join(queueDir, "bad1.resp")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}

	var resp fileQueueResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
}

func TestFileQueueTmpIgnored(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a .req.tmp file — should NOT be processed.
	tmpPath := filepath.Join(queueDir, "partial.req.tmp")
	if err := os.WriteFile(tmpPath, []byte(`{"method":"GET","path":"/health"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Scan the queue dir.
	d.scanQueueDir(queueDir, repo.ID)

	// The .tmp file should still exist (not processed).
	if _, err := os.Stat(tmpPath); err != nil {
		t.Errorf("expected .req.tmp file to still exist, got error: %v", err)
	}

	// No .resp file should be created.
	respPath := filepath.Join(queueDir, "partial.resp")
	if _, err := os.Stat(respPath); !os.IsNotExist(err) {
		t.Error("expected no .resp file for .tmp request")
	}
}

func TestFileQueueRepoContext(t *testing.T) {
	d := testDaemon(t)

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	queueDir1 := filepath.Join(tmpDir1, ".boxofrocks", "queue")
	queueDir2 := filepath.Join(tmpDir2, ".boxofrocks", "queue")

	// Register two repos.
	rr := doRequest(t, d, "POST", "/repos", map[string]interface{}{
		"owner": "org1", "name": "repo1",
		"local_path": tmpDir1, "queue": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo1: %d: %s", rr.Code, rr.Body.String())
	}
	var repo1 model.RepoConfig
	decodeJSON(t, rr, &repo1)

	rr = doRequest(t, d, "POST", "/repos", map[string]interface{}{
		"owner": "org2", "name": "repo2",
		"local_path": tmpDir2, "queue": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create repo2: %d: %s", rr.Code, rr.Body.String())
	}
	var repo2 model.RepoConfig
	decodeJSON(t, rr, &repo2)

	// Create an issue in repo1 via queue.
	body1, _ := json.Marshal(map[string]interface{}{"title": "Issue in repo1"})
	reqPath1 := writeReqFile(t, queueDir1, "ctx1", fileQueueRequest{
		Method: "POST",
		Path:   "/issues",
		Body:   json.RawMessage(body1),
	})
	d.processQueueFile(reqPath1, repo1.ID)

	// Create an issue in repo2 via queue.
	body2, _ := json.Marshal(map[string]interface{}{"title": "Issue in repo2"})
	reqPath2 := writeReqFile(t, queueDir2, "ctx2", fileQueueRequest{
		Method: "POST",
		Path:   "/issues",
		Body:   json.RawMessage(body2),
	})
	d.processQueueFile(reqPath2, repo2.ID)

	// List issues in repo1 via queue.
	reqPath1 = writeReqFile(t, queueDir1, "list1", fileQueueRequest{
		Method: "GET",
		Path:   "/issues",
	})
	d.processQueueFile(reqPath1, repo1.ID)
	resp1 := readRespFile(t, queueDir1, "list1")
	if resp1.Status != http.StatusOK {
		t.Fatalf("list repo1: expected 200, got %d", resp1.Status)
	}

	var issues1 []*model.Issue
	if err := json.Unmarshal(resp1.Body, &issues1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(issues1) != 1 || issues1[0].Title != "Issue in repo1" {
		t.Errorf("expected 1 issue in repo1, got %d", len(issues1))
	}

	// List issues in repo2 via queue.
	reqPath2 = writeReqFile(t, queueDir2, "list2", fileQueueRequest{
		Method: "GET",
		Path:   "/issues",
	})
	d.processQueueFile(reqPath2, repo2.ID)
	resp2 := readRespFile(t, queueDir2, "list2")
	if resp2.Status != http.StatusOK {
		t.Fatalf("list repo2: expected 200, got %d", resp2.Status)
	}

	var issues2 []*model.Issue
	if err := json.Unmarshal(resp2.Body, &issues2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(issues2) != 1 || issues2[0].Title != "Issue in repo2" {
		t.Errorf("expected 1 issue in repo2, got %d", len(issues2))
	}
}

func TestCleanStaleQueueFiles(t *testing.T) {
	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, "queue")
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create stale files.
	staleFiles := []string{"old1.req", "old2.resp", "old3.resp.tmp", "old4.req.tmp"}
	for _, f := range staleFiles {
		if err := os.WriteFile(filepath.Join(queueDir, f), []byte("stale"), 0644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	// Also create a non-queue file that should NOT be removed.
	keepFile := filepath.Join(queueDir, "readme.txt")
	if err := os.WriteFile(keepFile, []byte("keep"), 0644); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	cleanStaleQueueFiles(queueDir)

	// All stale files should be removed.
	for _, f := range staleFiles {
		if _, err := os.Stat(filepath.Join(queueDir, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", f)
		}
	}

	// Non-queue file should still exist.
	if _, err := os.Stat(keepFile); err != nil {
		t.Errorf("expected readme.txt to still exist, got: %v", err)
	}
}

func TestQueueResponseWriter(t *testing.T) {
	w := newQueueResponseWriter()

	// Default status should be 200.
	if w.statusCode != http.StatusOK {
		t.Errorf("expected default status 200, got %d", w.statusCode)
	}

	// Set headers.
	w.Header().Set("Content-Type", "application/json")
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type header to be set")
	}

	// Write status.
	w.WriteHeader(http.StatusCreated)
	if w.statusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.statusCode)
	}

	// Write body.
	n, err := w.Write([]byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 11 {
		t.Errorf("expected 11 bytes written, got %d", n)
	}
	if w.body.String() != `{"ok":true}` {
		t.Errorf("unexpected body: %s", w.body.String())
	}
}

func TestFileQueuePolling(t *testing.T) {
	d, repo, queueDir := setupQueueTest(t)

	// Start a file queue goroutine.
	if err := d.startFileQueue(repo); err != nil {
		t.Fatalf("start file queue: %v", err)
	}
	t.Cleanup(func() { d.cleanupFileQueues() })

	// Write a request file and wait for the response.
	writeReqFile(t, queueDir, "poll1", fileQueueRequest{
		Method: "GET",
		Path:   "/health",
	})

	resp := readRespFile(t, queueDir, "poll1")
	if resp.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Status)
	}

	// Request file should be cleaned up.
	reqPath := filepath.Join(queueDir, "poll1.req")
	if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
		t.Error("expected .req file to be removed after processing")
	}
}
