package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// Replay takes a list of events and produces a map of issueID to derived Issue state.
// Events must be sorted by timestamp. This is the full replay path.
func Replay(events []*model.Event) (map[int]*model.Issue, error) {
	issues := make(map[int]*model.Issue)
	for _, ev := range events {
		existing := issues[ev.IssueID]
		if ev.Action == model.ActionCreate && existing != nil {
			return nil, fmt.Errorf("duplicate create for issue %d", ev.IssueID)
		}
		updated, err := Apply(existing, ev)
		if err != nil {
			return nil, fmt.Errorf("applying event %d (action=%s, issue=%d): %w",
				ev.ID, ev.Action, ev.IssueID, err)
		}
		issues[ev.IssueID] = updated
	}
	return issues, nil
}

// Apply takes an existing issue (can be nil for "create") and a single event,
// returns the updated issue. Used for incremental processing.
func Apply(issue *model.Issue, event *model.Event) (*model.Issue, error) {
	var payload model.EventPayload
	if event.Payload != "" {
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}

	var result *model.Issue
	var err error

	switch event.Action {
	case model.ActionCreate:
		result, err = applyCreate(event, &payload)
	case model.ActionStatusChange:
		result, err = applyStatusChange(issue, event, &payload)
	case model.ActionAssign:
		result, err = applyAssign(issue, event, &payload)
	case model.ActionClose:
		result, err = applyClose(issue, event)
	case model.ActionUpdate:
		result, err = applyUpdate(issue, event, &payload)
	case model.ActionDelete:
		result, err = applyDelete(issue, event)
	case model.ActionReopen:
		result, err = applyReopen(issue, event)
	case model.ActionComment:
		result, err = applyComment(issue, event)
	default:
		return nil, fmt.Errorf("unknown action: %s", event.Action)
	}

	if err != nil {
		return nil, err
	}

	// Any event can carry a comment via the payload Comment field.
	if payload.Comment != "" && result != nil {
		if result.Comments == nil {
			result.Comments = []model.Comment{}
		}
		result.Comments = append(result.Comments, model.Comment{
			Text:      payload.Comment,
			Author:    event.Agent,
			Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
		})
	}

	return result, nil
}

func applyCreate(event *model.Event, payload *model.EventPayload) (*model.Issue, error) {
	issue := &model.Issue{
		ID:          event.IssueID,
		RepoID:      event.RepoID,
		Title:       payload.Title,
		Description: payload.Description,
		Status:      model.StatusOpen,
		Labels:      payload.Labels,
		Owner:       payload.Owner,
		Comments:    []model.Comment{},
		CreatedAt:   event.Timestamp,
		UpdatedAt:   event.Timestamp,
	}
	if payload.Priority != nil {
		issue.Priority = *payload.Priority
	}
	if payload.IssueType != "" {
		issue.IssueType = model.IssueType(payload.IssueType)
	}
	if issue.Labels == nil {
		issue.Labels = []string{}
	}
	return issue, nil
}

func applyStatusChange(issue *model.Issue, event *model.Event, payload *model.EventPayload) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("status_change on non-existent issue %d", event.IssueID)
	}
	if payload.Status == "" {
		return issue, nil
	}
	if IsTerminal(issue.Status) {
		return issue, nil
	}
	if !FromStatusMatch(issue.Status, payload.FromStatus) {
		return issue, nil
	}
	issue.Status = payload.Status
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyAssign(issue *model.Issue, event *model.Event, payload *model.EventPayload) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("assign on non-existent issue %d", event.IssueID)
	}
	issue.Owner = payload.Owner
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyClose(issue *model.Issue, event *model.Event) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("close on non-existent issue %d", event.IssueID)
	}
	if IsTerminal(issue.Status) || issue.Status == model.StatusClosed {
		return issue, nil
	}
	issue.Status = model.StatusClosed
	closedAt := event.Timestamp
	issue.ClosedAt = &closedAt
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyUpdate(issue *model.Issue, event *model.Event, payload *model.EventPayload) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("update on non-existent issue %d", event.IssueID)
	}
	if payload.Title != "" {
		issue.Title = payload.Title
	}
	if payload.Description != "" {
		issue.Description = payload.Description
	}
	if payload.Priority != nil {
		issue.Priority = *payload.Priority
	}
	if payload.IssueType != "" {
		issue.IssueType = model.IssueType(payload.IssueType)
	}
	if payload.Labels != nil {
		issue.Labels = payload.Labels
	}
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyDelete(issue *model.Issue, event *model.Event) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("delete on non-existent issue %d", event.IssueID)
	}
	if IsTerminal(issue.Status) {
		return issue, nil
	}
	issue.Status = model.StatusDeleted
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyReopen(issue *model.Issue, event *model.Event) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("reopen on non-existent issue %d", event.IssueID)
	}
	// Reopen is only valid from closed status.
	if issue.Status != model.StatusClosed {
		return issue, nil
	}
	issue.Status = model.StatusOpen
	issue.ClosedAt = nil
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}

func applyComment(issue *model.Issue, event *model.Event) (*model.Issue, error) {
	if issue == nil {
		return nil, fmt.Errorf("comment on non-existent issue %d", event.IssueID)
	}
	issue.UpdatedAt = event.Timestamp
	return issue, nil
}
