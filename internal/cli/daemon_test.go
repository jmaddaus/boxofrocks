package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitForDaemon_AlreadyRunning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	t.Cleanup(ts.Close)

	client := NewClient(ts.URL)
	err := waitForDaemon(client, 2*time.Second)
	if err != nil {
		t.Fatalf("waitForDaemon: %v", err)
	}
}

func TestWaitForDaemon_StartsLate(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	t.Cleanup(ts.Close)

	client := NewClient(ts.URL)
	err := waitForDaemon(client, 5*time.Second)
	if err != nil {
		t.Fatalf("waitForDaemon should succeed after retries: %v", err)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestWaitForDaemon_Timeout(t *testing.T) {
	// Use a server that always fails.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts.Close)

	client := NewClient(ts.URL)
	err := waitForDaemon(client, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "did not respond") {
		t.Errorf("expected 'did not respond' in error, got: %v", err)
	}
}

func TestReadTailLines(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	lines := []string{"line1", "line2", "line3", "line4", "line5"}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	// Read last 3 lines.
	got, err := readTailLines(logPath, 3)
	if err != nil {
		t.Fatalf("readTailLines: %v", err)
	}

	gotLines := strings.Split(strings.TrimSpace(got), "\n")
	if len(gotLines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(gotLines), gotLines)
	}
	if gotLines[0] != "line3" {
		t.Errorf("first line = %q, want 'line3'", gotLines[0])
	}
	if gotLines[2] != "line5" {
		t.Errorf("last line = %q, want 'line5'", gotLines[2])
	}
}

func TestReadTailLines_MoreThanAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	if err := os.WriteFile(logPath, []byte("only\ntwo\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	got, err := readTailLines(logPath, 10)
	if err != nil {
		t.Fatalf("readTailLines: %v", err)
	}

	gotLines := strings.Split(strings.TrimSpace(got), "\n")
	if len(gotLines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(gotLines))
	}
}

func TestRunDaemonBackground_AlreadyRunning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	t.Cleanup(ts.Close)

	gf := globalFlags{host: ts.URL}
	err := runDaemonBackground(gf)
	if err == nil {
		t.Fatal("expected error when daemon already running")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' in error, got: %v", err)
	}
}
