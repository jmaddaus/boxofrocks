package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/engine"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
)

// SyncStatus describes the current sync state of a single repo.
type SyncStatus struct {
	RepoName      string     `json:"repo_name"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	PendingEvents int        `json:"pending_events"`
	Syncing       bool       `json:"syncing"`
	LastError     string     `json:"last_error,omitempty"`
}

// SyncManager orchestrates sync goroutines for multiple repositories.
type SyncManager struct {
	store     store.Store
	ghClient  github.Client
	syncers   map[int]*RepoSyncer // keyed by repo ID
	mu        sync.Mutex
	rateMu    sync.Mutex
	rateLimit github.RateLimit
	stopCh    chan struct{}
}

// NewSyncManager creates a new SyncManager.
func NewSyncManager(s store.Store, gh github.Client) *SyncManager {
	return &SyncManager{
		store:    s,
		ghClient: gh,
		syncers:  make(map[int]*RepoSyncer),
		stopCh:   make(chan struct{}),
	}
}

// AddRepo starts a syncer goroutine for the given repo.
func (sm *SyncManager) AddRepo(repo *model.RepoConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.syncers[repo.ID]; exists {
		return fmt.Errorf("repo %d already being synced", repo.ID)
	}

	interval := sm.effectiveInterval()
	rs := newRepoSyncer(repo, sm.store, sm.ghClient, sm, interval)
	sm.syncers[repo.ID] = rs

	// Stagger start: repo gets a delay based on current count of syncers.
	idx := len(sm.syncers) - 1
	n := len(sm.syncers)
	var delay time.Duration
	if n > 1 {
		delay = time.Duration(idx) * (interval / time.Duration(n))
	}

	go rs.run(delay)
	return nil
}

// RemoveRepo stops the syncer goroutine for the given repo.
func (sm *SyncManager) RemoveRepo(repoID int) error {
	sm.mu.Lock()
	rs, ok := sm.syncers[repoID]
	if !ok {
		sm.mu.Unlock()
		return fmt.Errorf("repo %d not being synced", repoID)
	}
	delete(sm.syncers, repoID)
	sm.mu.Unlock()

	rs.stop()
	return nil
}

// ForceSync triggers an immediate incremental sync for the given repo.
func (sm *SyncManager) ForceSync(repoID int) error {
	sm.mu.Lock()
	rs, ok := sm.syncers[repoID]
	sm.mu.Unlock()

	if !ok {
		return fmt.Errorf("repo %d not being synced", repoID)
	}

	rs.force(false)
	return nil
}

// ForceSyncFull triggers an immediate full-replay sync for the given repo.
func (sm *SyncManager) ForceSyncFull(repoID int) error {
	sm.mu.Lock()
	rs, ok := sm.syncers[repoID]
	sm.mu.Unlock()

	if !ok {
		return fmt.Errorf("repo %d not being synced", repoID)
	}

	rs.force(true)
	return nil
}

// Status returns per-repo sync status.
func (sm *SyncManager) Status() map[int]*SyncStatus {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make(map[int]*SyncStatus, len(sm.syncers))
	for id, rs := range sm.syncers {
		st := rs.getStatus()
		result[id] = &st
	}
	return result
}

// Stop stops all syncer goroutines.
func (sm *SyncManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, rs := range sm.syncers {
		rs.stop()
		delete(sm.syncers, id)
	}
}

// effectiveInterval computes the poll interval adjusted by repo count.
// For N repos, effective interval = max(5s, 5s * N / 2).
func (sm *SyncManager) effectiveInterval() time.Duration {
	n := len(sm.syncers) + 1 // +1 for the repo being added
	interval := time.Duration(5*n/2) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	return interval
}

// checkRateLimit checks the shared rate limit and sleeps if necessary.
func (sm *SyncManager) checkRateLimit() {
	sm.rateMu.Lock()
	defer sm.rateMu.Unlock()

	rl := sm.ghClient.GetRateLimit()
	sm.rateLimit = rl

	if rl.Remaining > 0 && rl.Remaining < 100 {
		sleepDuration := time.Until(rl.Reset)
		if sleepDuration > 0 {
			slog.Info("rate limit low, sleeping until reset", "remaining", rl.Remaining, "reset", rl.Reset)
			time.Sleep(sleepDuration)
		}
	}
}

// ---------------------------------------------------------------------------
// syncRequest
// ---------------------------------------------------------------------------

type syncRequest struct {
	full bool // true for full replay
}

// ---------------------------------------------------------------------------
// RepoSyncer
// ---------------------------------------------------------------------------

// RepoSyncer runs a sync loop for a single repository.
type RepoSyncer struct {
	repo     *model.RepoConfig
	store    store.Store
	ghClient github.Client
	manager  *SyncManager // back-reference for rate limit
	interval time.Duration
	forceCh  chan syncRequest
	stopCh   chan struct{}
	status   SyncStatus
	mu       sync.RWMutex
}

func newRepoSyncer(repo *model.RepoConfig, s store.Store, gh github.Client, mgr *SyncManager, interval time.Duration) *RepoSyncer {
	return &RepoSyncer{
		repo:     repo,
		store:    s,
		ghClient: gh,
		manager:  mgr,
		interval: interval,
		forceCh:  make(chan syncRequest, 1),
		stopCh:   make(chan struct{}),
		status: SyncStatus{
			RepoName:   repo.FullName(),
			LastSyncAt: repo.LastSyncAt,
		},
	}
}

func (rs *RepoSyncer) run(startDelay time.Duration) {
	if startDelay > 0 {
		select {
		case <-time.After(startDelay):
		case <-rs.stopCh:
			return
		}
	}

	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	// Do an initial sync immediately.
	rs.cycle(false)

	for {
		select {
		case <-ticker.C:
			rs.cycle(false)
		case req := <-rs.forceCh:
			rs.cycle(req.full)
		case <-rs.stopCh:
			return
		}
	}
}

func (rs *RepoSyncer) stop() {
	select {
	case <-rs.stopCh:
		// Already stopped.
	default:
		close(rs.stopCh)
	}
}

func (rs *RepoSyncer) force(full bool) {
	select {
	case rs.forceCh <- syncRequest{full: full}:
	default:
		// A force request is already queued.
	}
}

func (rs *RepoSyncer) getStatus() SyncStatus {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.status
}

func (rs *RepoSyncer) setStatus(fn func(s *SyncStatus)) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	fn(&rs.status)
}

func (rs *RepoSyncer) cycle(full bool) {
	rs.setStatus(func(s *SyncStatus) {
		s.Syncing = true
		s.LastError = ""
	})

	ctx := context.Background()

	// Push outbound events first.
	if err := rs.pushOutbound(ctx); err != nil {
		rs.setStatus(func(s *SyncStatus) {
			s.Syncing = false
			s.LastError = fmt.Sprintf("push: %v", err)
		})
		return
	}

	// Pull inbound events.
	var err error
	if full {
		err = rs.pullInboundFull(ctx)
	} else {
		err = rs.pullInbound(ctx)
	}

	now := time.Now().UTC()
	if err != nil {
		rs.setStatus(func(s *SyncStatus) {
			s.Syncing = false
			s.LastError = fmt.Sprintf("pull: %v", err)
			s.LastSyncAt = &now
		})
		return
	}

	// Update pending count.
	pending, _ := rs.store.PendingEvents(ctx, rs.repo.ID)
	rs.setStatus(func(s *SyncStatus) {
		s.Syncing = false
		s.LastSyncAt = &now
		s.PendingEvents = len(pending)
	})

	// Persist last sync time.
	rs.repo.LastSyncAt = &now
	_ = rs.store.UpdateRepo(ctx, rs.repo)
}

// pushOutbound sends locally-created events to GitHub.
func (rs *RepoSyncer) pushOutbound(ctx context.Context) error {
	pending, err := rs.store.PendingEvents(ctx, rs.repo.ID)
	if err != nil {
		return fmt.Errorf("query pending events: %w", err)
	}

	for _, ev := range pending {
		rs.manager.checkRateLimit()

		issue, err := rs.store.GetIssue(ctx, ev.IssueID)
		if err != nil {
			return fmt.Errorf("get issue %d: %w", ev.IssueID, err)
		}

		if ev.Action == model.ActionCreate && issue.GitHubID == nil {
			// Create a new GitHub issue.
			ghIssue, err := rs.ghClient.CreateIssue(
				ctx,
				rs.repo.Owner,
				rs.repo.Name,
				issue.Title,
				issue.Description,
				append([]string{"agent-tracker"}, issue.Labels...),
			)
			if err != nil {
				return fmt.Errorf("create github issue: %w", err)
			}

			// Store the GitHub issue number on the local issue.
			issue.GitHubID = &ghIssue.Number
			if err := rs.store.UpdateIssue(ctx, issue); err != nil {
				return fmt.Errorf("update issue github_id: %w", err)
			}

			// Post the create event as the first comment.
			rs.manager.checkRateLimit()
			commentBody := github.FormatEventComment(ev)
			ghComment, err := rs.ghClient.CreateComment(ctx, rs.repo.Owner, rs.repo.Name, ghIssue.Number, commentBody)
			if err != nil {
				return fmt.Errorf("create initial comment: %w", err)
			}

			if err := rs.store.MarkEventSynced(ctx, ev.ID, ghComment.ID); err != nil {
				return fmt.Errorf("mark event synced: %w", err)
			}
		} else {
			// Post event as a comment on the existing GitHub issue.
			if issue.GitHubID == nil {
				// Skip events whose issue has no GitHub counterpart yet.
				continue
			}

			rs.manager.checkRateLimit()
			commentBody := github.FormatEventComment(ev)
			ghComment, err := rs.ghClient.CreateComment(ctx, rs.repo.Owner, rs.repo.Name, *issue.GitHubID, commentBody)
			if err != nil {
				return fmt.Errorf("create comment for event %d: %w", ev.ID, err)
			}

			if err := rs.store.MarkEventSynced(ctx, ev.ID, ghComment.ID); err != nil {
				return fmt.Errorf("mark event synced: %w", err)
			}
		}
	}

	return nil
}

// pullInbound fetches new comments from GitHub and applies them incrementally.
func (rs *RepoSyncer) pullInbound(ctx context.Context) error {
	rs.manager.checkRateLimit()

	// List GitHub issues with agent-tracker label.
	issues, newETag, err := rs.ghClient.ListIssues(ctx, rs.repo.Owner, rs.repo.Name, github.ListOpts{
		ETag:   rs.repo.IssuesETag,
		Labels: "agent-tracker",
	})
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	// Update the ETag.
	rs.repo.IssuesETag = newETag

	// If no new issues (304 Not Modified), issues will be nil.
	if issues == nil {
		return nil
	}

	for _, ghIssue := range issues {
		if err := rs.processGitHubIssue(ctx, ghIssue, false); err != nil {
			return fmt.Errorf("process issue #%d: %w", ghIssue.Number, err)
		}
	}

	return nil
}

// pullInboundFull fetches all comments and uses full replay.
func (rs *RepoSyncer) pullInboundFull(ctx context.Context) error {
	rs.manager.checkRateLimit()

	issues, newETag, err := rs.ghClient.ListIssues(ctx, rs.repo.Owner, rs.repo.Name, github.ListOpts{
		Labels: "agent-tracker",
	})
	if err != nil {
		return fmt.Errorf("list issues (full): %w", err)
	}

	rs.repo.IssuesETag = newETag

	if issues == nil {
		return nil
	}

	for _, ghIssue := range issues {
		if err := rs.processGitHubIssue(ctx, ghIssue, true); err != nil {
			return fmt.Errorf("process issue #%d (full): %w", ghIssue.Number, err)
		}
	}

	return nil
}

// processGitHubIssue handles a single GitHub issue, syncing comments locally.
func (rs *RepoSyncer) processGitHubIssue(ctx context.Context, ghIssue *github.GitHubIssue, full bool) error {
	// Find the local issue with this GitHub ID.
	localIssue := rs.findLocalIssueByGitHubID(ctx, ghIssue.Number)

	if localIssue == nil {
		// This is a web-created issue. Create a local issue and synthetic create event.
		var err error
		localIssue, err = rs.handleWebCreatedIssue(ctx, ghIssue)
		if err != nil {
			return fmt.Errorf("handle web-created issue: %w", err)
		}
	}

	// Get sync state for this issue.
	lastCommentID, lastCommentAt, err := rs.store.GetIssueSyncState(ctx, rs.repo.ID, ghIssue.Number)
	if err != nil {
		return fmt.Errorf("get sync state: %w", err)
	}

	// Build list opts: if not full, only fetch comments since last sync.
	opts := github.ListOpts{}
	if !full && lastCommentAt != "" {
		opts.Since = lastCommentAt
	}

	rs.manager.checkRateLimit()
	comments, _, err := rs.ghClient.ListComments(ctx, rs.repo.Owner, rs.repo.Name, ghIssue.Number, opts)
	if err != nil {
		return fmt.Errorf("list comments: %w", err)
	}

	if full {
		// Full replay: parse all comments into events and replay.
		if err := rs.fullReplayComments(ctx, localIssue, comments, ghIssue.Number); err != nil {
			return fmt.Errorf("full replay: %w", err)
		}
	} else {
		// Incremental: process only new comments.
		for _, c := range comments {
			if c.ID <= lastCommentID {
				continue
			}

			ev, err := github.ParseEventComment(c.Body)
			if err != nil || ev == nil {
				// Not an agent-tracker comment; skip.
				continue
			}

			// Check if we already have this comment in our events.
			if rs.hasGitHubComment(ctx, localIssue.ID, c.ID) {
				continue
			}

			// Apply incrementally.
			ev.RepoID = rs.repo.ID
			ev.IssueID = localIssue.ID
			ghCommentID := c.ID
			ev.GitHubCommentID = &ghCommentID
			ghIssueNum := ghIssue.Number
			ev.GitHubIssueNumber = &ghIssueNum
			ev.Synced = 1

			updated, err := engine.Apply(localIssue, ev)
			if err != nil {
				return fmt.Errorf("apply event from comment %d: %w", c.ID, err)
			}
			localIssue = updated

			if err := rs.store.UpdateIssue(ctx, localIssue); err != nil {
				return fmt.Errorf("update issue: %w", err)
			}

			if _, err := rs.store.AppendEvent(ctx, ev); err != nil {
				return fmt.Errorf("append event: %w", err)
			}

			lastCommentID = c.ID
			lastCommentAt = c.CreatedAt.UTC().Format(time.RFC3339)
		}
	}

	// Update the sync state with the latest comment.
	if len(comments) > 0 {
		last := comments[len(comments)-1]
		if last.ID > lastCommentID {
			lastCommentID = last.ID
			lastCommentAt = last.CreatedAt.UTC().Format(time.RFC3339)
		}
		if err := rs.store.SetIssueSyncState(ctx, rs.repo.ID, ghIssue.Number, lastCommentID, lastCommentAt); err != nil {
			return fmt.Errorf("set sync state: %w", err)
		}
	}

	return nil
}

// fullReplayComments parses all comments, builds events, and uses engine.Replay.
func (rs *RepoSyncer) fullReplayComments(ctx context.Context, localIssue *model.Issue, comments []*github.GitHubComment, ghIssueNumber int) error {
	var events []*model.Event

	// Start with events already in the store.
	existing, err := rs.store.ListEvents(ctx, rs.repo.ID, localIssue.ID)
	if err != nil {
		return fmt.Errorf("list existing events: %w", err)
	}

	// Build a set of known github_comment_ids.
	knownComments := make(map[int]bool, len(existing))
	for _, e := range existing {
		if e.GitHubCommentID != nil {
			knownComments[*e.GitHubCommentID] = true
		}
	}

	events = append(events, existing...)

	for _, c := range comments {
		if knownComments[c.ID] {
			continue
		}

		ev, err := github.ParseEventComment(c.Body)
		if err != nil || ev == nil {
			continue
		}

		ev.RepoID = rs.repo.ID
		ev.IssueID = localIssue.ID
		ghCommentID := c.ID
		ev.GitHubCommentID = &ghCommentID
		ghIssueNum := ghIssueNumber
		ev.GitHubIssueNumber = &ghIssueNum
		ev.Synced = 1

		events = append(events, ev)

		// Persist the new event.
		if _, err := rs.store.AppendEvent(ctx, ev); err != nil {
			return fmt.Errorf("append event: %w", err)
		}
	}

	// Replay all events.
	issueMap, err := engine.Replay(events)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}

	if replayed, ok := issueMap[localIssue.ID]; ok {
		replayed.ID = localIssue.ID
		replayed.RepoID = rs.repo.ID
		replayed.GitHubID = localIssue.GitHubID
		if err := rs.store.UpdateIssue(ctx, replayed); err != nil {
			return fmt.Errorf("update issue after replay: %w", err)
		}
	}

	return nil
}

// handleWebCreatedIssue creates a local issue from a GitHub issue with no local counterpart.
func (rs *RepoSyncer) handleWebCreatedIssue(ctx context.Context, ghIssue *github.GitHubIssue) (*model.Issue, error) {
	ghNum := ghIssue.Number

	// Create a local issue.
	localIssue := &model.Issue{
		RepoID:    rs.repo.ID,
		GitHubID:  &ghNum,
		Title:     ghIssue.Title,
		Status:    model.StatusOpen,
		IssueType: model.IssueTypeTask,
		Labels:    []string{},
		CreatedAt: ghIssue.CreatedAt,
		UpdatedAt: ghIssue.UpdatedAt,
	}

	// Parse metadata from the body if present.
	meta, description, err := github.ParseMetadata(ghIssue.Body)
	if err == nil && meta != nil {
		localIssue.Description = description
		if meta.Status != "" {
			localIssue.Status = model.Status(meta.Status)
		}
		localIssue.Priority = meta.Priority
		if meta.IssueType != "" {
			localIssue.IssueType = model.IssueType(meta.IssueType)
		}
		localIssue.Owner = meta.Owner
		if meta.Labels != nil {
			localIssue.Labels = meta.Labels
		}
	} else {
		localIssue.Description = ghIssue.Body
	}

	created, err := rs.store.CreateIssue(ctx, localIssue)
	if err != nil {
		return nil, fmt.Errorf("create local issue: %w", err)
	}

	// Generate and persist synthetic create event.
	syntheticEvent := GenerateSyntheticCreate(ghIssue, rs.repo.ID, created.ID)
	syntheticEvent.Synced = 1 // It came from GitHub, so it is already synced.

	storedEvent, err := rs.store.AppendEvent(ctx, syntheticEvent)
	if err != nil {
		return nil, fmt.Errorf("append synthetic create: %w", err)
	}

	// Post the create event as a comment on GitHub so other syncers can see it.
	rs.manager.checkRateLimit()
	commentBody := github.FormatEventComment(syntheticEvent)
	ghComment, err := rs.ghClient.CreateComment(ctx, rs.repo.Owner, rs.repo.Name, ghIssue.Number, commentBody)
	if err != nil {
		return nil, fmt.Errorf("post synthetic create comment: %w", err)
	}

	// Mark the synthetic event synced with the comment ID.
	if err := rs.store.MarkEventSynced(ctx, storedEvent.ID, ghComment.ID); err != nil {
		// Non-fatal: event is already synced=1.
		slog.Error("failed to update synthetic event comment ID", "error", err)
	}

	// Update the sync state so subsequent comment fetches skip this comment.
	if err := rs.store.SetIssueSyncState(ctx, rs.repo.ID, ghIssue.Number, ghComment.ID, ghComment.CreatedAt.UTC().Format(time.RFC3339)); err != nil {
		slog.Error("failed to set sync state after web-created issue", "error", err)
	}

	return created, nil
}

// findLocalIssueByGitHubID looks for a local issue matching the given GitHub issue number.
func (rs *RepoSyncer) findLocalIssueByGitHubID(ctx context.Context, ghIssueNumber int) *model.Issue {
	issues, err := rs.store.ListIssues(ctx, store.IssueFilter{RepoID: rs.repo.ID})
	if err != nil {
		return nil
	}
	for _, iss := range issues {
		if iss.GitHubID != nil && *iss.GitHubID == ghIssueNumber {
			return iss
		}
	}
	return nil
}

// hasGitHubComment checks whether we already have an event with the given github_comment_id.
func (rs *RepoSyncer) hasGitHubComment(ctx context.Context, issueID, ghCommentID int) bool {
	events, err := rs.store.ListEvents(ctx, rs.repo.ID, issueID)
	if err != nil {
		return false
	}
	for _, e := range events {
		if e.GitHubCommentID != nil && *e.GitHubCommentID == ghCommentID {
			return true
		}
	}
	return false
}
