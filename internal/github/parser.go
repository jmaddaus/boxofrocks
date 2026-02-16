package github

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// metadataRe matches the boxofrocks metadata comment block in an issue body.
var metadataRe = regexp.MustCompile(`(?m)^<!-- boxofrocks ({.*}) -->$`)

// SchemaVersion is the current event comment wire format version.
const SchemaVersion = 2

// eventPrefixRe matches both [boxofrocks:v1] (versioned) and [boxofrocks] (legacy) prefixes (v1 bare format).
var eventPrefixRe = regexp.MustCompile(`^\[boxofrocks(?::v(\d+))?\]\s*(.+)$`)

// v2EventRe matches the v2 HTML-embedded format: <!-- [boxofrocks:v2] {...} -->
var v2EventRe = regexp.MustCompile(`<!-- \[boxofrocks:v(\d+)\]\s*(.+?) -->`)

// MetadataBlock holds the structured metadata stored in a GitHub issue body.
type MetadataBlock struct {
	Status    string          `json:"status"`
	Priority  int             `json:"priority"`
	IssueType string          `json:"issue_type"`
	Owner     string          `json:"owner"`
	Labels    []string        `json:"labels"`
	Comments  []model.Comment `json:"comments,omitempty"`
}

// ParseMetadata extracts the boxofrocks JSON from an issue body.
// Returns the metadata and the human-visible text (body without the metadata block).
// If no metadata block is found, returns nil metadata and the full body.
func ParseMetadata(body string) (*MetadataBlock, string, error) {
	matches := metadataRe.FindStringSubmatchIndex(body)
	if matches == nil {
		return nil, body, nil
	}

	// Extract the JSON substring (submatch group 1)
	jsonStr := body[matches[2]:matches[3]]

	var meta MetadataBlock
	if err := json.Unmarshal([]byte(jsonStr), &meta); err != nil {
		return nil, body, fmt.Errorf("parse boxofrocks metadata: %w", err)
	}

	// Remove the metadata line from the body to get human-visible text
	humanText := body[:matches[0]] + body[matches[1]:]

	// Trim trailing whitespace/newlines that were separating the metadata
	humanText = strings.TrimRight(humanText, "\n\r ")

	return &meta, humanText, nil
}

// RenderBody combines human text with boxofrocks metadata into a full issue body.
func RenderBody(humanText string, meta *MetadataBlock) string {
	jsonData, err := json.Marshal(meta)
	if err != nil {
		// This should never happen with our simple struct
		panic(fmt.Sprintf("failed to marshal metadata: %v", err))
	}

	metaLine := fmt.Sprintf("<!-- boxofrocks %s -->", string(jsonData))

	if humanText == "" {
		return metaLine
	}

	return humanText + "\n\n" + metaLine
}

// eventJSON is the wire format for events stored in GitHub comments.
type eventJSON struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Payload   string `json:"payload"`
	Agent     string `json:"agent"`
}

// FormatEventComment formats an event for posting as a GitHub comment.
// Produces v2 format: human-readable text followed by JSON in an HTML comment.
func FormatEventComment(event *model.Event) string {
	ej := eventJSON{
		Timestamp: event.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		Action:    string(event.Action),
		Payload:   event.Payload,
		Agent:     event.Agent,
	}
	data, err := json.Marshal(ej)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal event: %v", err))
	}

	humanText := FormatHumanText(event)
	jsonTag := fmt.Sprintf("<!-- [boxofrocks:v%d] %s -->", SchemaVersion, string(data))

	return humanText + "\n\n" + jsonTag
}

// FormatHumanText generates the human-readable portion of a v2 event comment.
func FormatHumanText(event *model.Event) string {
	var payload model.EventPayload
	if event.Payload != "" {
		json.Unmarshal([]byte(event.Payload), &payload)
	}

	var parts []string

	switch event.Action {
	case model.ActionCreate:
		line := fmt.Sprintf("**Created**: %s", payload.Title)
		if payload.Description != "" {
			line += "\n\n" + payload.Description
		}
		parts = append(parts, line)
	case model.ActionStatusChange:
		if payload.FromStatus != "" {
			parts = append(parts, fmt.Sprintf("**Status changed**: %s \u2192 %s", payload.FromStatus, payload.Status))
		} else {
			parts = append(parts, fmt.Sprintf("**Status changed**: \u2192 %s", payload.Status))
		}
	case model.ActionClose:
		parts = append(parts, "**Closed**")
	case model.ActionReopen:
		parts = append(parts, "**Reopened**")
	case model.ActionAssign:
		if payload.Owner != "" {
			parts = append(parts, fmt.Sprintf("**Assigned** to %s", payload.Owner))
		} else {
			parts = append(parts, "**Unassigned**")
		}
	case model.ActionUpdate:
		var changed []string
		if payload.Title != "" {
			changed = append(changed, "title")
		}
		if payload.Description != "" {
			changed = append(changed, "description")
		}
		if payload.Priority != nil {
			changed = append(changed, "priority")
		}
		if payload.IssueType != "" {
			changed = append(changed, "type")
		}
		if payload.Labels != nil {
			changed = append(changed, "labels")
		}
		if len(changed) > 0 {
			parts = append(parts, fmt.Sprintf("**Updated**: %s", strings.Join(changed, ", ")))
		} else {
			parts = append(parts, "**Updated**")
		}
	case model.ActionDelete:
		parts = append(parts, "**Deleted**")
	case model.ActionComment:
		if payload.Comment != "" {
			parts = append(parts, fmt.Sprintf("**Comment**: %s", payload.Comment))
		} else {
			parts = append(parts, "**Comment**")
		}
	default:
		parts = append(parts, fmt.Sprintf("**%s**", event.Action))
	}

	// If the event carries a comment (non-ActionComment events), append as quote.
	if event.Action != model.ActionComment && payload.Comment != "" {
		parts = append(parts, "\n> "+payload.Comment)
	}

	// Agent and timestamp footer.
	ts := event.Timestamp.UTC().Format("2006-01-02 15:04 UTC")
	if event.Agent != "" {
		parts = append(parts, fmt.Sprintf("\n*by %s at %s*", event.Agent, ts))
	} else {
		parts = append(parts, fmt.Sprintf("\n*at %s*", ts))
	}

	return strings.Join(parts, "\n")
}

// ParseEventComment parses a boxofrocks event from a comment body.
// Returns nil if the comment is not a boxofrocks event.
// Accepts v2 (HTML comment), v1 (bare prefix), and legacy (unversioned) formats.
// Returns an error for schema versions newer than SchemaVersion.
func ParseEventComment(body string) (*model.Event, error) {
	body = strings.TrimSpace(body)

	// Try v2 format first: <!-- [boxofrocks:v2] {...} -->
	if v2Matches := v2EventRe.FindStringSubmatch(body); v2Matches != nil {
		return parseEventFromMatches(v2Matches[1], v2Matches[2])
	}

	// Fall back to v1/legacy bare prefix format.
	matches := eventPrefixRe.FindStringSubmatch(body)
	if matches == nil {
		return nil, nil
	}

	return parseEventFromMatches(matches[1], matches[2])
}

// parseEventFromMatches extracts an event from version string and JSON payload.
func parseEventFromMatches(versionStr, jsonStr string) (*model.Event, error) {
	if versionStr != "" {
		var version int
		if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
			return nil, fmt.Errorf("parse schema version: %w", err)
		}
		if version > SchemaVersion {
			return nil, fmt.Errorf("unsupported boxofrocks schema version v%d (this binary supports up to v%d)", version, SchemaVersion)
		}
	}

	var ej eventJSON
	if err := json.Unmarshal([]byte(jsonStr), &ej); err != nil {
		return nil, fmt.Errorf("parse event comment JSON: %w", err)
	}

	ts, err := parseTimestamp(ej.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("parse event comment timestamp: %w", err)
	}

	event := &model.Event{
		Timestamp: ts,
		Action:    model.Action(ej.Action),
		Payload:   ej.Payload,
		Agent:     ej.Agent,
	}

	return event, nil
}

// parseTimestamp tries common timestamp formats.
func parseTimestamp(s string) (t time.Time, err error) {
	formats := []string{
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err = time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp format: %s", s)
}
