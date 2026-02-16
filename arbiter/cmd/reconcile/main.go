package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jmaddaus/boxofrocks/internal/engine"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
)

var version = "dev"

func main() {
	log.Printf("reconcile version %s", version)
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required")
	}

	repoFull := os.Getenv("GITHUB_REPOSITORY") // "owner/repo"
	if repoFull == "" {
		log.Fatal("GITHUB_REPOSITORY is required")
	}

	issueNumStr := os.Getenv("ISSUE_NUMBER")
	if issueNumStr == "" {
		log.Fatal("ISSUE_NUMBER is required")
	}
	issueNum, err := strconv.Atoi(issueNumStr)
	if err != nil {
		log.Fatalf("invalid ISSUE_NUMBER: %v", err)
	}

	parts := strings.SplitN(repoFull, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("invalid GITHUB_REPOSITORY format: %s", repoFull)
	}
	owner, repo := parts[0], parts[1]

	client := github.NewClient(token)
	ctx := context.Background()

	newBody, replayed, err := reconcile(ctx, client, owner, repo, issueNum)
	if err != nil {
		log.Fatalf("reconcile: %v", err)
	}
	if replayed == nil {
		return
	}

	if err := client.UpdateIssueBody(ctx, owner, repo, issueNum, newBody); err != nil {
		log.Fatalf("update issue body: %v", err)
	}

	// Close or reopen the GitHub issue to match replayed state.
	ghIssue, err := client.GetIssue(ctx, owner, repo, issueNum)
	if err != nil {
		log.Fatalf("get issue for state sync: %v", err)
	}
	if err := syncIssueState(ctx, client, owner, repo, issueNum, replayed, ghIssue); err != nil {
		log.Fatalf("sync issue state: %v", err)
	}

	fmt.Printf("reconciled issue #%d: status=%s, priority=%d, owner=%s\n",
		issueNum, replayed.Status, replayed.Priority, replayed.Owner)
}

// syncIssueState closes or reopens the GitHub issue to match the replayed state.
func syncIssueState(ctx context.Context, client github.Client, owner, repo string, issueNum int, replayed *model.Issue, ghIssue *github.GitHubIssue) error {
	if replayed.Status == model.StatusClosed || replayed.Status == model.StatusDeleted {
		if ghIssue.State == "open" {
			return client.UpdateIssueState(ctx, owner, repo, issueNum, "closed")
		}
	} else {
		if ghIssue.State == "closed" {
			return client.UpdateIssueState(ctx, owner, repo, issueNum, "open")
		}
	}
	return nil
}

// reconcile fetches comments for the given issue, replays boxofrocks events,
// and returns the new body and replayed issue state. Returns ("", nil, nil)
// if there are no events to reconcile.
func reconcile(ctx context.Context, client github.Client, owner, repo string, issueNum int) (string, *model.Issue, error) {
	// 1. Fetch all comments (paginated)
	comments, _, err := client.ListComments(ctx, owner, repo, issueNum, github.ListOpts{PerPage: 100})
	if err != nil {
		return "", nil, fmt.Errorf("fetch comments: %w", err)
	}

	// 2. Parse boxofrocks events from comments
	var events []*model.Event
	for _, c := range comments {
		ev, err := github.ParseEventComment(c.Body)
		if err != nil || ev == nil {
			continue // Skip non-boxofrocks comments
		}
		ev.ID = c.ID // Use comment ID for ordering
		ev.IssueID = issueNum
		commentID := c.ID
		ev.GitHubCommentID = &commentID
		events = append(events, ev)
	}

	if len(events) == 0 {
		log.Println("no boxofrocks events found, nothing to reconcile")
		return "", nil, nil
	}

	// 3. Replay all events
	issueMap, err := engine.Replay(events)
	if err != nil {
		return "", nil, fmt.Errorf("replay: %w", err)
	}

	// Get the replayed issue (there should be exactly one since all events reference the same issue)
	var replayed *model.Issue
	for _, iss := range issueMap {
		replayed = iss
		break
	}
	if replayed == nil {
		log.Println("replay produced no issue state")
		return "", nil, nil
	}

	// 4. Fetch current issue body to preserve human text
	ghIssue, err := client.GetIssue(ctx, owner, repo, issueNum)
	if err != nil {
		return "", nil, fmt.Errorf("get issue: %w", err)
	}

	_, humanText, err := github.ParseMetadata(ghIssue.Body)
	if err != nil {
		return "", nil, fmt.Errorf("parse metadata: %w", err)
	}

	// 5. Build metadata and write back
	meta := &github.MetadataBlock{
		Status:    string(replayed.Status),
		Priority:  replayed.Priority,
		IssueType: string(replayed.IssueType),
		Owner:     replayed.Owner,
		Labels:    replayed.Labels,
	}
	if meta.Labels == nil {
		meta.Labels = []string{}
	}

	newBody := github.RenderBody(humanText, meta)
	return newBody, replayed, nil
}
