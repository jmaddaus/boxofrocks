package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// newTestServer creates an httptest server that routes to the given handler func.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, NewClient(ts.URL)
}

func TestCreateRepo(t *testing.T) {
	var gotMethod, gotPath string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})

	if err := c.CreateRepo("owner", "name"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method: want POST, got %s", gotMethod)
	}
	if gotPath != "/repos" {
		t.Errorf("path: want /repos, got %s", gotPath)
	}
}

func TestListRepos(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: want GET, got %s", r.Method)
		}
		repos := []*model.RepoConfig{
			{ID: 1, Owner: "a", Name: "b", CreatedAt: time.Now()},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(repos)
	})

	repos, err := c.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Owner != "a" {
		t.Errorf("owner: want a, got %s", repos[0].Owner)
	}
}

func TestCreateIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: want POST, got %s", r.Method)
		}
		if r.URL.Query().Get("repo") != "owner/name" {
			t.Errorf("repo query: want owner/name, got %s", r.URL.Query().Get("repo"))
		}
		issue := model.Issue{ID: 1, Title: "test", Status: model.StatusOpen}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issue)
	})

	issue, err := c.CreateIssue("owner/name", CreateIssueRequest{Title: "test"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Title != "test" {
		t.Errorf("title: want test, got %s", issue.Title)
	}
}

func TestListIssues(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: want GET, got %s", r.Method)
		}
		issues := []*model.Issue{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issues)
	})

	issues, err := c.ListIssues("owner/name", ListOpts{Status: "open"})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestGetIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: want GET, got %s", r.Method)
		}
		if r.URL.Path != "/issues/42" {
			t.Errorf("path: want /issues/42, got %s", r.URL.Path)
		}
		issue := model.Issue{ID: 42, Title: "found"}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issue)
	})

	issue, err := c.GetIssue(42)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.ID != 42 {
		t.Errorf("ID: want 42, got %d", issue.ID)
	}
}

func TestUpdateIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("method: want PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/issues/5" {
			t.Errorf("path: want /issues/5, got %s", r.URL.Path)
		}
		issue := model.Issue{ID: 5, Title: "updated"}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issue)
	})

	issue, err := c.UpdateIssue(5, map[string]interface{}{"title": "updated"})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if issue.Title != "updated" {
		t.Errorf("title: want updated, got %s", issue.Title)
	}
}

func TestDeleteIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method: want DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/issues/7" {
			t.Errorf("path: want /issues/7, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "deleted"})
	})

	if err := c.DeleteIssue(7); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
}

func TestAssignIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: want POST, got %s", r.Method)
		}
		if r.URL.Path != "/issues/3/assign" {
			t.Errorf("path: want /issues/3/assign, got %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if req["owner"] != "alice" {
			t.Errorf("owner: want alice, got %s", req["owner"])
		}
		issue := model.Issue{ID: 3, Owner: "alice"}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issue)
	})

	issue, err := c.AssignIssue(3, "alice")
	if err != nil {
		t.Fatalf("AssignIssue: %v", err)
	}
	if issue.Owner != "alice" {
		t.Errorf("owner: want alice, got %s", issue.Owner)
	}
}

func TestNextIssue(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: want GET, got %s", r.Method)
		}
		issue := model.Issue{ID: 1, Title: "next one"}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issue)
	})

	issue, err := c.NextIssue("owner/name")
	if err != nil {
		t.Fatalf("NextIssue: %v", err)
	}
	if issue.Title != "next one" {
		t.Errorf("title: want 'next one', got %s", issue.Title)
	}
}

func TestHealth(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: want GET, got %s", r.Method)
		}
		if r.URL.Path != "/health" {
			t.Errorf("path: want /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	})

	result, err := c.Health()
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status: want ok, got %v", result["status"])
	}
}

func TestForceSync(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: want POST, got %s", r.Method)
		}
		if r.URL.Path != "/sync" {
			t.Errorf("path: want /sync, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "synced"})
	})

	if err := c.ForceSync("owner/name"); err != nil {
		t.Fatalf("ForceSync: %v", err)
	}
}

func TestDecodeOrError_ErrorResponse(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad input"})
	})

	err := c.CreateRepo("x", "y")
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}
