package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// captureStdout runs fn and returns what was written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintJSON(t *testing.T) {
	out := captureStdout(t, func() {
		printJSON(map[string]string{"key": "value"})
	})

	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\nOutput: %s", err, out)
	}
	if m["key"] != "value" {
		t.Errorf("expected key=value, got %v", m)
	}
}

func TestPrintPretty(t *testing.T) {
	issues := []*model.Issue{
		{
			ID:        1,
			Status:    model.StatusOpen,
			Priority:  1,
			IssueType: model.IssueTypeTask,
			Title:     "Test Issue",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	out := captureStdout(t, func() {
		printPretty(issues)
	})

	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") {
		t.Errorf("expected table header, got: %s", out)
	}
	if !strings.Contains(out, "Test Issue") {
		t.Errorf("expected issue title in output, got: %s", out)
	}
}

func TestPrintPrettyIssue(t *testing.T) {
	ghID := 42
	issue := &model.Issue{
		ID:          1,
		GitHubID:    &ghID,
		Title:       "My Issue",
		Status:      model.StatusOpen,
		Priority:    2,
		IssueType:   model.IssueTypeTask,
		Description: "A description",
		Owner:       "alice",
		Labels:      []string{"bug"},
		CreatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC),
	}

	out := captureStdout(t, func() {
		printPrettyIssue(issue)
	})

	if !strings.Contains(out, "Issue #1 (GitHub #42)") {
		t.Errorf("expected issue header with GitHub ID, got: %s", out)
	}
	if !strings.Contains(out, "My Issue") {
		t.Errorf("expected title in output, got: %s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected owner in output, got: %s", out)
	}
}

func TestPrintMessage(t *testing.T) {
	// Pretty mode.
	out := captureStdout(t, func() {
		printMessage("hello world", true)
	})
	if strings.TrimSpace(out) != "hello world" {
		t.Errorf("pretty message: want 'hello world', got %q", strings.TrimSpace(out))
	}

	// JSON mode.
	out = captureStdout(t, func() {
		printMessage("hello world", false)
	})
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if m["message"] != "hello world" {
		t.Errorf("JSON message: want 'hello world', got %v", m)
	}
}
