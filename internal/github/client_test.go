package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, *clientImpl) {
	ts := httptest.NewServer(handler)
	c := newClientWithBaseURL("test-token", ts.Client(), ts.URL)
	return ts, c
}

func TestListIssues_Basic(t *testing.T) {
	issues := []*GitHubIssue{
		{Number: 1, Title: "Issue 1", State: "open"},
		{Number: 2, Title: "Issue 2", State: "closed"},
	}

	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", got)
		}
		if got := r.Header.Get("Accept"); got != acceptHeader {
			t.Errorf("expected Accept %q, got %q", acceptHeader, got)
		}
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("expected User-Agent %q, got %q", userAgent, got)
		}

		w.Header().Set("ETag", `"etag123"`)
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issues)
	})
	defer ts.Close()

	result, etag, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if etag != `"etag123"` {
		t.Errorf("expected etag %q, got %q", `"etag123"`, etag)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result))
	}
	if result[0].Title != "Issue 1" {
		t.Errorf("expected 'Issue 1', got %q", result[0].Title)
	}
	if result[1].Number != 2 {
		t.Errorf("expected issue number 2, got %d", result[1].Number)
	}
}

func TestListIssues_ETag304(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"etag123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		t.Error("expected If-None-Match header")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	})
	defer ts.Close()

	result, etag, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{ETag: `"etag123"`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result on 304, got %v", result)
	}
	if etag != `"etag123"` {
		t.Errorf("expected same etag, got %q", etag)
	}
}

func TestListIssues_Pagination(t *testing.T) {
	page1 := []*GitHubIssue{{Number: 1, Title: "Issue 1"}}
	page2 := []*GitHubIssue{{Number: 2, Title: "Issue 2"}}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First page: include Link header pointing to page 2
			nextURL := fmt.Sprintf("http://%s%s?page=2&per_page=1", r.Host, r.URL.Path)
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
			w.Header().Set("ETag", `"page1-etag"`)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page1)
		} else {
			// Second page: no Link header
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page2)
		}
	}))
	defer ts.Close()

	client := newClientWithBaseURL("test-token", ts.Client(), ts.URL)

	result, etag, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{PerPage: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 issues across 2 pages, got %d", len(result))
	}
	if result[0].Title != "Issue 1" || result[1].Title != "Issue 2" {
		t.Errorf("unexpected issue titles: %v, %v", result[0].Title, result[1].Title)
	}
	if etag != `"page1-etag"` {
		t.Errorf("expected etag from first page, got %q", etag)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", callCount)
	}
}

func TestListIssues_WithLabelsAndSince(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("labels"); got != "bug,feature" {
			t.Errorf("expected labels 'bug,feature', got %q", got)
		}
		if got := query.Get("since"); got != "2024-01-01T00:00:00Z" {
			t.Errorf("expected since param, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	})
	defer ts.Close()

	_, _, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{
		Labels: "bug,feature",
		Since:  "2024-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateIssue_Success(t *testing.T) {
	created := &GitHubIssue{
		Number: 42,
		Title:  "New Issue",
		Body:   "Issue body",
		State:  "open",
	}

	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["title"] != "New Issue" {
			t.Errorf("expected title 'New Issue', got %v", payload["title"])
		}
		if payload["body"] != "Issue body" {
			t.Errorf("expected body 'Issue body', got %v", payload["body"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	})
	defer ts.Close()

	result, err := client.CreateIssue(context.Background(), "owner", "repo", "New Issue", "Issue body", []string{"bug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Number != 42 {
		t.Errorf("expected issue number 42, got %d", result.Number)
	}
}

func TestUpdateIssueBody_Success(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["body"] != "Updated body" {
			t.Errorf("expected body 'Updated body', got %v", payload["body"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	defer ts.Close()

	err := client.UpdateIssueBody(context.Background(), "owner", "repo", 42, "Updated body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListComments_Basic(t *testing.T) {
	comments := []*GitHubComment{
		{ID: 1, Body: "Comment 1"},
		{ID: 2, Body: "Comment 2"},
	}

	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/issues/10/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("ETag", `"comments-etag"`)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(comments)
	})
	defer ts.Close()

	result, etag, err := client.ListComments(context.Background(), "owner", "repo", 10, ListOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if etag != `"comments-etag"` {
		t.Errorf("expected etag %q, got %q", `"comments-etag"`, etag)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(result))
	}
}

func TestListComments_Pagination(t *testing.T) {
	page1 := []*GitHubComment{{ID: 1, Body: "Comment 1"}}
	page2 := []*GitHubComment{{ID: 2, Body: "Comment 2"}}

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			nextURL := fmt.Sprintf("http://%s%s?page=2&per_page=1", r.Host, r.URL.Path)
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page1)
		} else {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page2)
		}
	}))
	defer ts.Close()

	client := newClientWithBaseURL("test-token", ts.Client(), ts.URL)

	result, _, err := client.ListComments(context.Background(), "owner", "repo", 10, ListOpts{PerPage: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 comments across 2 pages, got %d", len(result))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", callCount)
	}
}

func TestCreateComment_Success(t *testing.T) {
	created := &GitHubComment{ID: 99, Body: "New comment"}

	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/5/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	})
	defer ts.Close()

	result, err := client.CreateComment(context.Background(), "owner", "repo", 5, "New comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != 99 {
		t.Errorf("expected comment ID 99, got %d", result.ID)
	}
}

func TestCreateLabel_Success(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/labels" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "agent-tracker" {
			t.Errorf("expected name 'agent-tracker', got %v", payload["name"])
		}
		if payload["color"] != "0e8a16" {
			t.Errorf("expected color '0e8a16', got %v", payload["color"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"name":"agent-tracker","color":"0e8a16"}`))
	})
	defer ts.Close()

	err := client.CreateLabel(context.Background(), "owner", "repo", "agent-tracker", "#0e8a16", "Managed by agent-tracker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateLabel_AlreadyExists(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// 422 means label already exists
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed","errors":[{"code":"already_exists"}]}`))
	})
	defer ts.Close()

	err := client.CreateLabel(context.Background(), "owner", "repo", "agent-tracker", "0e8a16", "desc")
	if err != nil {
		t.Fatalf("expected no error on 422 (already exists), got: %v", err)
	}
}

func TestRateLimitTracking(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Hour).Unix()

	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "42")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	})
	defer ts.Close()

	_, _, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rl := client.GetRateLimit()
	if rl.Remaining != 42 {
		t.Errorf("expected remaining 42, got %d", rl.Remaining)
	}
	if rl.Reset.Unix() != resetTime {
		t.Errorf("expected reset time %d, got %d", resetTime, rl.Reset.Unix())
	}
}

func TestParseLinkNext(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "single next link",
			header:   `<https://api.github.com/repos/foo/bar/issues?page=2>; rel="next"`,
			expected: "https://api.github.com/repos/foo/bar/issues?page=2",
		},
		{
			name:     "next and last links",
			header:   `<https://api.github.com/repos/foo/bar/issues?page=2>; rel="next", <https://api.github.com/repos/foo/bar/issues?page=5>; rel="last"`,
			expected: "https://api.github.com/repos/foo/bar/issues?page=2",
		},
		{
			name:     "only last link",
			header:   `<https://api.github.com/repos/foo/bar/issues?page=5>; rel="last"`,
			expected: "",
		},
		{
			name:     "empty header",
			header:   "",
			expected: "",
		},
		{
			name:     "prev and next",
			header:   `<https://api.github.com/repos/foo/bar/issues?page=1>; rel="prev", <https://api.github.com/repos/foo/bar/issues?page=3>; rel="next"`,
			expected: "https://api.github.com/repos/foo/bar/issues?page=3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLinkNext(tt.header)
			if got != tt.expected {
				t.Errorf("parseLinkNext(%q) = %q, want %q", tt.header, got, tt.expected)
			}
		})
	}
}

func TestListIssues_ErrorStatus(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	defer ts.Close()

	_, _, err := client.ListIssues(context.Background(), "owner", "repo", ListOpts{})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestCreateIssue_ErrorStatus(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	})
	defer ts.Close()

	_, err := client.CreateIssue(context.Background(), "owner", "repo", "title", "body", nil)
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
}
