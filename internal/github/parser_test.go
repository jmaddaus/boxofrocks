package github

import (
	"strings"
	"testing"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

func TestParseMetadata_Valid(t *testing.T) {
	body := `This is the issue description.

<!-- boxofrocks {"status":"open","priority":2,"issue_type":"task","owner":"alice","labels":["bug","urgent"]} -->`

	meta, humanText, err := ParseMetadata(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.Status != "open" {
		t.Errorf("expected status 'open', got %q", meta.Status)
	}
	if meta.Priority != 2 {
		t.Errorf("expected priority 2, got %d", meta.Priority)
	}
	if meta.IssueType != "task" {
		t.Errorf("expected issue_type 'task', got %q", meta.IssueType)
	}
	if meta.Owner != "alice" {
		t.Errorf("expected owner 'alice', got %q", meta.Owner)
	}
	if len(meta.Labels) != 2 || meta.Labels[0] != "bug" || meta.Labels[1] != "urgent" {
		t.Errorf("expected labels [bug, urgent], got %v", meta.Labels)
	}
	if humanText != "This is the issue description." {
		t.Errorf("expected trimmed human text, got %q", humanText)
	}
}

func TestParseMetadata_NoMetadata(t *testing.T) {
	body := "Just a regular issue body with no metadata."

	meta, humanText, err := ParseMetadata(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata, got %+v", meta)
	}
	if humanText != body {
		t.Errorf("expected full body returned, got %q", humanText)
	}
}

func TestParseMetadata_EmptyLabels(t *testing.T) {
	body := `Description here.

<!-- boxofrocks {"status":"in_progress","priority":1,"issue_type":"bug","owner":"","labels":[]} -->`

	meta, _, err := ParseMetadata(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got %q", meta.Status)
	}
	if meta.Owner != "" {
		t.Errorf("expected empty owner, got %q", meta.Owner)
	}
	if len(meta.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", meta.Labels)
	}
}

func TestParseMetadata_MetadataWithSurroundingText(t *testing.T) {
	body := `First paragraph.

Second paragraph.

<!-- boxofrocks {"status":"closed","priority":3,"issue_type":"feature","owner":"bob","labels":["enhancement"]} -->`

	meta, humanText, err := ParseMetadata(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.Status != "closed" {
		t.Errorf("expected status 'closed', got %q", meta.Status)
	}
	expected := "First paragraph.\n\nSecond paragraph."
	if humanText != expected {
		t.Errorf("expected human text %q, got %q", expected, humanText)
	}
}

func TestRenderBody_Basic(t *testing.T) {
	meta := &MetadataBlock{
		Status:    "open",
		Priority:  2,
		IssueType: "task",
		Owner:     "",
		Labels:    []string{},
	}

	result := RenderBody("This is a description.", meta)

	// Should contain both the human text and the metadata
	expected := "This is a description.\n\n<!-- boxofrocks {\"status\":\"open\",\"priority\":2,\"issue_type\":\"task\",\"owner\":\"\",\"labels\":[]} -->"
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestRenderBody_EmptyHumanText(t *testing.T) {
	meta := &MetadataBlock{
		Status:    "open",
		Priority:  1,
		IssueType: "bug",
		Owner:     "alice",
		Labels:    []string{"bug"},
	}

	result := RenderBody("", meta)
	expected := `<!-- boxofrocks {"status":"open","priority":1,"issue_type":"bug","owner":"alice","labels":["bug"]} -->`
	if result != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
	}
}

func TestRenderBody_RoundTrip(t *testing.T) {
	originalMeta := &MetadataBlock{
		Status:    "in_progress",
		Priority:  3,
		IssueType: "feature",
		Owner:     "charlie",
		Labels:    []string{"enhancement", "v2"},
	}
	originalText := "This is the feature description.\n\nWith multiple paragraphs."

	rendered := RenderBody(originalText, originalMeta)

	parsedMeta, parsedText, err := ParseMetadata(rendered)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}
	if parsedMeta == nil {
		t.Fatal("expected metadata after round-trip, got nil")
	}

	if parsedMeta.Status != originalMeta.Status {
		t.Errorf("status mismatch: %q vs %q", parsedMeta.Status, originalMeta.Status)
	}
	if parsedMeta.Priority != originalMeta.Priority {
		t.Errorf("priority mismatch: %d vs %d", parsedMeta.Priority, originalMeta.Priority)
	}
	if parsedMeta.IssueType != originalMeta.IssueType {
		t.Errorf("issue_type mismatch: %q vs %q", parsedMeta.IssueType, originalMeta.IssueType)
	}
	if parsedMeta.Owner != originalMeta.Owner {
		t.Errorf("owner mismatch: %q vs %q", parsedMeta.Owner, originalMeta.Owner)
	}
	if len(parsedMeta.Labels) != len(originalMeta.Labels) {
		t.Errorf("labels length mismatch: %d vs %d", len(parsedMeta.Labels), len(originalMeta.Labels))
	}
	for i := range originalMeta.Labels {
		if i < len(parsedMeta.Labels) && parsedMeta.Labels[i] != originalMeta.Labels[i] {
			t.Errorf("label[%d] mismatch: %q vs %q", i, parsedMeta.Labels[i], originalMeta.Labels[i])
		}
	}
	if parsedText != originalText {
		t.Errorf("human text mismatch:\n  got:  %q\n  want: %q", parsedText, originalText)
	}
}

func TestFormatEventComment_And_ParseEventComment_RoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	event := &model.Event{
		Timestamp: ts,
		Action:    model.ActionStatusChange,
		Payload:   `{"status":"in_progress"}`,
		Agent:     "user1",
	}

	formatted := FormatEventComment(event)

	// Verify v2 format: should contain HTML comment with boxofrocks tag.
	if !strings.Contains(formatted, "<!-- [boxofrocks:v2]") {
		t.Fatalf("expected v2 HTML comment tag, got %q", formatted)
	}

	// Verify human-readable portion is present.
	if !strings.Contains(formatted, "**Status changed**") {
		t.Errorf("expected human-readable status change text, got %q", formatted)
	}

	parsed, err := ParseEventComment(formatted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected parsed event, got nil")
	}

	if !parsed.Timestamp.Equal(ts) {
		t.Errorf("timestamp mismatch: got %v, want %v", parsed.Timestamp, ts)
	}
	if parsed.Action != model.ActionStatusChange {
		t.Errorf("action mismatch: got %q, want %q", parsed.Action, model.ActionStatusChange)
	}
	if parsed.Payload != `{"status":"in_progress"}` {
		t.Errorf("payload mismatch: got %q", parsed.Payload)
	}
	if parsed.Agent != "user1" {
		t.Errorf("agent mismatch: got %q, want %q", parsed.Agent, "user1")
	}
}

func TestParseEventComment_LegacyUnversionedPrefix(t *testing.T) {
	// Old format without version — must still parse for backwards compatibility.
	body := `[boxofrocks] {"timestamp":"2024-01-15T10:30:00Z","action":"status_change","payload":"{\"status\":\"in_progress\"}","agent":"user1"}`

	parsed, err := ParseEventComment(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected parsed event, got nil")
	}
	if parsed.Action != model.ActionStatusChange {
		t.Errorf("expected action status_change, got %q", parsed.Action)
	}
}

func TestParseEventComment_UnsupportedVersion(t *testing.T) {
	body := `[boxofrocks:v99] {"timestamp":"2024-01-15T10:30:00Z","action":"create","payload":"{}","agent":"bot"}`

	_, err := ParseEventComment(body)
	if err == nil {
		t.Fatal("expected error for unsupported schema version")
	}
	if !strings.Contains(err.Error(), "unsupported boxofrocks schema version v99") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseEventComment_NonAgentTracker(t *testing.T) {
	comments := []string{
		"This is a regular comment.",
		"LGTM!",
		"<!-- some other metadata -->",
		"[other-tool] some data",
		"",
	}

	for _, body := range comments {
		parsed, err := ParseEventComment(body)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", body, err)
		}
		if parsed != nil {
			t.Errorf("expected nil for non-boxofrocks comment %q, got %+v", body, parsed)
		}
	}
}

func TestParseEventComment_InvalidJSON(t *testing.T) {
	bodies := []string{
		"[boxofrocks] {invalid json}",
		"[boxofrocks:v1] {invalid json}",
	}
	for _, body := range bodies {
		_, err := ParseEventComment(body)
		if err == nil {
			t.Errorf("expected error for invalid JSON in %q", body)
		}
	}
}

func TestParseEventComment_V1StillParsed(t *testing.T) {
	// Ensure old v1 format is still parsed correctly by the updated parser.
	body := `[boxofrocks:v1] {"timestamp":"2024-01-15T10:30:00Z","action":"status_change","payload":"{\"status\":\"in_progress\"}","agent":"user1"}`

	parsed, err := ParseEventComment(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected parsed event, got nil")
	}
	if parsed.Action != model.ActionStatusChange {
		t.Errorf("expected action status_change, got %q", parsed.Action)
	}
	if parsed.Agent != "user1" {
		t.Errorf("expected agent user1, got %q", parsed.Agent)
	}
}

func TestFormatHumanText_AllActions(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		event    *model.Event
		contains []string
	}{
		{
			name: "create",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionCreate,
				Payload: `{"title":"New Issue","description":"test desc"}`, Agent: "alice",
			},
			contains: []string{"**Created**: New Issue", "test desc", "alice"},
		},
		{
			name: "status_change",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionStatusChange,
				Payload: `{"status":"in_progress","from_status":"open"}`, Agent: "bob",
			},
			contains: []string{"**Status changed**: open", "in_progress", "bob"},
		},
		{
			name: "close",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionClose, Payload: `{}`, Agent: "charlie",
			},
			contains: []string{"**Closed**", "charlie"},
		},
		{
			name: "reopen",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionReopen, Payload: `{}`, Agent: "dave",
			},
			contains: []string{"**Reopened**", "dave"},
		},
		{
			name: "assign",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionAssign,
				Payload: `{"owner":"eve"}`, Agent: "frank",
			},
			contains: []string{"**Assigned** to eve", "frank"},
		},
		{
			name: "update",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionUpdate,
				Payload: `{"title":"Updated Title","priority":1}`, Agent: "grace",
			},
			contains: []string{"**Updated**: title, priority", "grace"},
		},
		{
			name: "delete",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionDelete, Payload: `{}`, Agent: "heidi",
			},
			contains: []string{"**Deleted**", "heidi"},
		},
		{
			name: "comment",
			event: &model.Event{
				Timestamp: ts, Action: model.ActionComment,
				Payload: `{"comment":"This is a note"}`, Agent: "ivan",
			},
			contains: []string{"**Comment**: This is a note", "ivan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatHumanText(tt.event)
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

func TestFormatHumanText_UpdateWithComment(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	event := &model.Event{
		Timestamp: ts, Action: model.ActionUpdate,
		Payload: `{"title":"New Title","comment":"Changed it"}`, Agent: "alice",
	}
	result := FormatHumanText(event)
	if !strings.Contains(result, "> Changed it") {
		t.Errorf("expected quoted comment in human text, got:\n%s", result)
	}
}

func TestV2FormatRoundTrip_AllActions(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	actions := []struct {
		action  model.Action
		payload string
	}{
		{model.ActionCreate, `{"title":"Test"}`},
		{model.ActionStatusChange, `{"status":"in_progress","from_status":"open"}`},
		{model.ActionClose, `{}`},
		{model.ActionReopen, `{}`},
		{model.ActionAssign, `{"owner":"alice"}`},
		{model.ActionUpdate, `{"title":"Updated"}`},
		{model.ActionDelete, `{}`},
		{model.ActionComment, `{"comment":"Hello"}`},
	}

	for _, tt := range actions {
		t.Run(string(tt.action), func(t *testing.T) {
			event := &model.Event{
				Timestamp: ts,
				Action:    tt.action,
				Payload:   tt.payload,
				Agent:     "test-agent",
			}

			formatted := FormatEventComment(event)
			parsed, err := ParseEventComment(formatted)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if parsed == nil {
				t.Fatal("expected parsed event")
			}
			if parsed.Action != tt.action {
				t.Errorf("action: got %q, want %q", parsed.Action, tt.action)
			}
			if parsed.Payload != tt.payload {
				t.Errorf("payload: got %q, want %q", parsed.Payload, tt.payload)
			}
			if parsed.Agent != "test-agent" {
				t.Errorf("agent: got %q, want %q", parsed.Agent, "test-agent")
			}
			if !parsed.Timestamp.Equal(ts) {
				t.Errorf("timestamp: got %v, want %v", parsed.Timestamp, ts)
			}
		})
	}
}

func TestFormatEventComment_EmptyPayload(t *testing.T) {
	event := &model.Event{
		Timestamp: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Action:    model.ActionCreate,
		Payload:   "",
		Agent:     "bot",
	}

	formatted := FormatEventComment(event)
	parsed, err := ParseEventComment(formatted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Payload != "" {
		t.Errorf("expected empty payload, got %q", parsed.Payload)
	}
}
