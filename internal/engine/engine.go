package engine

import (
	"encoding/json"
	"fmt"

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

	switch event.Action {
	case model.ActionCreate:
		return applyCreate(event, &payload)
	case model.ActionStatusChange:
		return applyStatusChange(issue, event, &payload)
	case model.ActionAssign:
		return applyAssign(issue, event, &payload)
	case model.ActionClose:
		return applyClose(issue, event)
	case model.ActionUpdate:
		return applyUpdate(issue, event, &payload)
	case model.ActionDelete:
		return applyDelete(issue, event)
	case model.ActionReopen:
		return applyReopen(issue, event)
	default:
		return nil, fmt.Errorf("unknown action: %s", event.Action)
	}
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
