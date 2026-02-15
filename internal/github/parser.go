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

// eventPrefixRe matches the [boxofrocks] prefix in event comments.
var eventPrefixRe = regexp.MustCompile(`^\[boxofrocks\]\s*(.+)$`)

// MetadataBlock holds the structured metadata stored in a GitHub issue body.
type MetadataBlock struct {
	Status    string   `json:"status"`
	Priority  int      `json:"priority"`
	IssueType string   `json:"issue_type"`
	Owner     string   `json:"owner"`
	Labels    []string `json:"labels"`
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
	return "[boxofrocks] " + string(data)
}

// ParseEventComment parses a boxofrocks event from a comment body.
// Returns nil if the comment is not a boxofrocks event.
func ParseEventComment(body string) (*model.Event, error) {
	body = strings.TrimSpace(body)
	matches := eventPrefixRe.FindStringSubmatch(body)
	if matches == nil {
		return nil, nil
	}

	jsonStr := matches[1]
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
