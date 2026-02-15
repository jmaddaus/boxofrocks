package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/engine"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
)

// ProcessNewComments processes a set of GitHub comments, parses agent-tracker
// events from them, and applies them incrementally to the given issue.
// It returns the updated issue state.
func ProcessNewComments(
	ctx context.Context,
	issue *model.Issue,
	comments []*github.GitHubComment,
	s store.Store,
	repoID int,
	ghIssueNumber int,
) (*model.Issue, error) {
	current := issue
	for _, c := range comments {
		ev, err := github.ParseEventComment(c.Body)
		if err != nil || ev == nil {
			// Not an agent-tracker event; skip.
			continue
		}

		// Fill in fields.
		ev.RepoID = repoID
		ev.IssueID = issue.ID
		ghCommentID := c.ID
		ev.GitHubCommentID = &ghCommentID
		ghNum := ghIssueNumber
		ev.GitHubIssueNumber = &ghNum
		ev.Synced = 1

		// Apply incrementally.
		updated, err := engine.Apply(current, ev)
		if err != nil {
			return nil, fmt.Errorf("apply event from comment %d: %w", c.ID, err)
		}
		current = updated

		// Persist the event.
		if _, err := s.AppendEvent(ctx, ev); err != nil {
			return nil, fmt.Errorf("append event from comment %d: %w", c.ID, err)
		}
	}

	// Persist the updated issue.
	if err := s.UpdateIssue(ctx, current); err != nil {
		return nil, fmt.Errorf("update issue: %w", err)
	}

	return current, nil
}

// GenerateSyntheticCreate creates a "create" event from a GitHub issue that
// has no agent-tracker metadata. This is used when a user creates an issue on
// the web with the agent-tracker label.
func GenerateSyntheticCreate(ghIssue *github.GitHubIssue, repoID int, localIssueID int) *model.Event {
	// Parse metadata if present.
	meta, description, _ := github.ParseMetadata(ghIssue.Body)

	payload := model.EventPayload{
		Title:       ghIssue.Title,
		Description: description,
	}

	if meta != nil {
		payload.Status = model.Status(meta.Status)
		payload.Priority = &meta.Priority
		payload.IssueType = meta.IssueType
		payload.Owner = meta.Owner
		payload.Labels = meta.Labels
	} else {
		payload.Description = ghIssue.Body
	}

	// Collect non-agent-tracker labels from the GitHub issue.
	if meta == nil {
		var labels []string
		for _, l := range ghIssue.Labels {
			if l.Name != "agent-tracker" {
				labels = append(labels, l.Name)
			}
		}
		if labels != nil {
			payload.Labels = labels
		}
	}

	payloadJSON, _ := json.Marshal(payload)

	ghNum := ghIssue.Number
	return &model.Event{
		RepoID:            repoID,
		IssueID:           localIssueID,
		GitHubIssueNumber: &ghNum,
		Timestamp:         ghIssue.CreatedAt.UTC(),
		Action:            model.ActionCreate,
		Payload:           string(payloadJSON),
		Agent:             "github-sync",
		Synced:            0,
	}
}

// ReplayFromComments takes all comments from a GitHub issue, parses events,
// and produces the replayed issue state. This is used for full sync recovery.
func ReplayFromComments(
	comments []*github.GitHubComment,
	repoID int,
	issueID int,
	ghIssueNumber int,
) (*model.Issue, []*model.Event, error) {
	var events []*model.Event

	for _, c := range comments {
		ev, err := github.ParseEventComment(c.Body)
		if err != nil || ev == nil {
			continue
		}

		ev.RepoID = repoID
		ev.IssueID = issueID
		ghCommentID := c.ID
		ev.GitHubCommentID = &ghCommentID
		ghNum := ghIssueNumber
		ev.GitHubIssueNumber = &ghNum
		ev.Synced = 1
		ev.Timestamp = ev.Timestamp.UTC()

		events = append(events, ev)
	}

	if len(events) == 0 {
		return nil, nil, fmt.Errorf("no agent-tracker events found in comments")
	}

	issueMap, err := engine.Replay(events)
	if err != nil {
		return nil, nil, fmt.Errorf("replay: %w", err)
	}

	issue, ok := issueMap[issueID]
	if !ok {
		return nil, nil, fmt.Errorf("issue %d not found in replay result", issueID)
	}

	return issue, events, nil
}

// FormatSyncTimestamp formats a time for use in GitHub API "since" parameters.
func FormatSyncTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
