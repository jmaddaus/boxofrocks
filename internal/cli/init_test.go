package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRunInit_DaemonAlreadyRunning(t *testing.T) {
	// Daemon mock that accepts repo registration and sync.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.URL.Path == "/repos" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "created"})
		case r.URL.Path == "/sync" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "synced"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	gf := globalFlags{host: ts.URL}
	err := runInit([]string{"--repo", "owner/name"}, gf)
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
}

func TestRunInit_AlreadyRegistered(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.URL.Path == "/repos" && r.Method == "POST":
			// 409 Conflict — already registered.
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "already exists"})
		case r.URL.Path == "/repos/paths" && r.Method == "POST":
			// AddRepoPath for worktree local_path.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "path added"})
		case r.URL.Path == "/sync" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "synced"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	gf := globalFlags{host: ts.URL}
	err := runInit([]string{"--repo", "owner/name"}, gf)
	if err != nil {
		t.Fatalf("runInit should succeed for already-registered repo: %v", err)
	}
}

func TestRunInit_InvalidRepoFormat(t *testing.T) {
	gf := globalFlags{host: "http://localhost:9999"}
	tests := []string{"noslash", "", "/", "a/", "/b"}

	for _, repo := range tests {
		if repo == "" {
			continue // empty gets a different error
		}
		err := runInit([]string{"--repo", repo}, gf)
		if err == nil {
			t.Errorf("expected error for repo %q", repo)
		}
	}
}

func TestRunInit_Offline(t *testing.T) {
	syncCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.URL.Path == "/repos" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "created"})
		case r.URL.Path == "/sync":
			syncCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	gf := globalFlags{host: ts.URL}
	err := runInit([]string{"--repo", "owner/name", "--offline"}, gf)
	if err != nil {
		t.Fatalf("runInit --offline: %v", err)
	}
	if syncCalled {
		t.Error("sync should not be called with --offline")
	}
}

func TestRunInit_JSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.URL.Path == "/repos" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "created"})
		case r.URL.Path == "/sync":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "synced"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	gf := globalFlags{host: ts.URL, pretty: false}
	err := runInit([]string{"--repo", "owner/name"}, gf)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Output may contain auth warning lines before the JSON object.
	// Extract the JSON portion (starts with '{').
	jsonStart := strings.Index(output, "{")
	if jsonStart < 0 {
		t.Fatalf("no JSON found in output: %s", output)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(output[jsonStart:]), &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", output[jsonStart:])
	}
	if result["status"] != "initialized" {
		t.Errorf("status = %q, want 'initialized'", result["status"])
	}
	if result["repo"] != "owner/name" {
		t.Errorf("repo = %q, want 'owner/name'", result["repo"])
	}
}

func TestRunInit_AuthGuidance(t *testing.T) {
	// Ensure no token is available so the guidance message is printed.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.URL.Path == "/repos" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "created"})
		case r.URL.Path == "/sync":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "synced"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	// Capture stdout to check for auth guidance.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	gf := globalFlags{host: ts.URL, pretty: true}
	err := runInit([]string{"--repo", "owner/name"}, gf)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "bor login") {
		t.Errorf("expected auth guidance mentioning 'bor login', got:\n%s", output)
	}
	if !strings.Contains(output, "Ready!") {
		t.Errorf("expected 'Ready!' message, got:\n%s", output)
	}
}
