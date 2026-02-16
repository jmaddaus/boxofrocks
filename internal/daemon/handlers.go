package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/engine"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
)

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal error: "+err.Error())
		return
	}
	w.WriteHeader(status)
	w.Write(data)
}

func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// Repo resolution
// ---------------------------------------------------------------------------

// resolveRepo determines the target repo from the request.
// Priority: ?repo= query param > X-Repo header > single registered repo.
func (d *Daemon) resolveRepo(r *http.Request) (*model.RepoConfig, error) {
	ctx := r.Context()

	// 1. Query param.
	repoParam := r.URL.Query().Get("repo")
	if repoParam != "" {
		parts := strings.SplitN(repoParam, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid repo format, use owner/name")
		}
		repo, err := d.store.GetRepoByName(ctx, parts[0], parts[1])
		if err != nil {
			return nil, fmt.Errorf("repo %s not found", repoParam)
		}
		return repo, nil
	}

	// 2. X-Repo header.
	repoHeader := r.Header.Get("X-Repo")
	if repoHeader != "" {
		parts := strings.SplitN(repoHeader, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid X-Repo format, use owner/name")
		}
		repo, err := d.store.GetRepoByName(ctx, parts[0], parts[1])
		if err != nil {
			return nil, fmt.Errorf("repo %s not found", repoHeader)
		}
		return repo, nil
	}

	// 3. Implicit: exactly one repo registered.
	repos, err := d.store.ListRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	if len(repos) == 1 {
		return repos[0], nil
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repos registered")
	}
	return nil, fmt.Errorf("multiple repos registered, specify ?repo=owner/name or X-Repo header")
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func (d *Daemon) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repos, err := d.store.ListRepos(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	repoNames := make([]string, len(repos))
	for i, repo := range repos {
		repoNames[i] = repo.FullName()
	}

	resp := map[string]interface{}{
		"status": "ok",
		"repos":  repoNames,
	}

	// Include uptime if the daemon has been started via Run().
	if !d.startedAt.IsZero() {
		resp["uptime"] = time.Since(d.startedAt).Round(time.Second).String()
	}

	// Include per-repo sync status if SyncManager is available.
	if d.syncMgr != nil {
		syncStatuses := d.syncMgr.Status()
		syncInfo := make(map[string]interface{}, len(syncStatuses))
		for _, st := range syncStatuses {
			entry := map[string]interface{}{
				"pending_events": st.PendingEvents,
				"syncing":        st.Syncing,
			}
			if st.LastSyncAt != nil {
				entry["last_sync"] = st.LastSyncAt.Format(time.RFC3339)
			}
			if st.LastError != "" {
				entry["last_error"] = st.LastError
			}
			syncInfo[st.RepoName] = entry
		}
		resp["sync_status"] = syncInfo
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Force sync (stub)
// ---------------------------------------------------------------------------

func (d *Daemon) forceSync(w http.ResponseWriter, r *http.Request) {
	if d.syncMgr == nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "sync not yet implemented",
		})
		return
	}

	repo, err := d.resolveRepo(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := d.syncMgr.ForceSync(repo.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "sync triggered",
		"repo":   repo.FullName(),
	})
}

// ---------------------------------------------------------------------------
// Repos
// ---------------------------------------------------------------------------

type addRepoRequest struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

func (d *Daemon) addRepo(w http.ResponseWriter, r *http.Request) {
	var req addRepoRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Owner == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "owner and name are required")
		return
	}

	repo, err := d.store.AddRepo(r.Context(), req.Owner, req.Name)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if d.syncMgr != nil {
		if err := d.syncMgr.AddRepo(repo); err != nil {
			slog.Warn("failed to start syncer for new repo", "repo", repo.FullName(), "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, repo)
}

func (d *Daemon) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := d.store.ListRepos(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if repos == nil {
		repos = []*model.RepoConfig{}
	}
	writeJSON(w, http.StatusOK, repos)
}

// ---------------------------------------------------------------------------
// Issues
// ---------------------------------------------------------------------------

func (d *Daemon) listIssues(w http.ResponseWriter, r *http.Request) {
	repo, err := d.resolveRepo(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := store.IssueFilter{
		RepoID: repo.ID,
	}

	if s := r.URL.Query().Get("status"); s != "" {
		filter.Status = model.Status(s)
	}
	if p := r.URL.Query().Get("priority"); p != "" {
		pv, err := strconv.Atoi(p)
		if err == nil {
			filter.Priority = &pv
		}
	}
	if t := r.URL.Query().Get("type"); t != "" {
		filter.Type = model.IssueType(t)
	}
	if o := r.URL.Query().Get("owner"); o != "" {
		filter.Owner = o
	}

	// Unless ?all=true, exclude deleted issues. If no explicit status filter
	// is set, we need to filter out deleted in the result set.
	showAll := r.URL.Query().Get("all") == "true"

	issues, err := d.store.ListIssues(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !showAll && filter.Status == "" {
		filtered := make([]*model.Issue, 0, len(issues))
		for _, iss := range issues {
			if iss.Status != model.StatusDeleted {
				filtered = append(filtered, iss)
			}
		}
		issues = filtered
	}

	if issues == nil {
		issues = []*model.Issue{}
	}

	writeJSON(w, http.StatusOK, issues)
}

func (d *Daemon) nextIssue(w http.ResponseWriter, r *http.Request) {
	repo, err := d.resolveRepo(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	issue, err := d.store.NextIssue(r.Context(), repo.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "no issues available")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func (d *Daemon) getIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue id")
		return
	}

	issue, err := d.store.GetIssue(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

type createIssueRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    *int     `json:"priority"`
	IssueType   string   `json:"issue_type"`
	Labels      []string `json:"labels"`
	Comment     string   `json:"comment"`
}

func (d *Daemon) createIssue(w http.ResponseWriter, r *http.Request) {
	repo, err := d.resolveRepo(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req createIssueRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	// Build the issue.
	issue := &model.Issue{
		RepoID:      repo.ID,
		Title:       req.Title,
		Description: req.Description,
		Status:      model.StatusOpen,
		IssueType:   model.IssueTypeTask,
		Labels:      req.Labels,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if req.Priority != nil {
		issue.Priority = *req.Priority
	}
	if req.IssueType != "" {
		issue.IssueType = model.IssueType(req.IssueType)
	}
	if issue.Labels == nil {
		issue.Labels = []string{}
	}

	// Persist the issue first to get its ID.
	created, err := d.store.CreateIssue(ctx, issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create issue: "+err.Error())
		return
	}

	// Build and append the create event.
	payload := model.EventPayload{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		IssueType:   req.IssueType,
		Labels:      req.Labels,
		Comment:     req.Comment,
	}
	payloadJSON, _ := json.Marshal(payload)

	event := &model.Event{
		RepoID:    repo.ID,
		IssueID:   created.ID,
		Timestamp: now,
		Action:    model.ActionCreate,
		Payload:   string(payloadJSON),
		Synced:    0,
	}

	if _, err := d.store.AppendEvent(ctx, event); err != nil {
		writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

type updateIssueRequest struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	IssueType   string   `json:"issue_type,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Comment     string   `json:"comment,omitempty"`
}

func (d *Daemon) updateIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue id")
		return
	}

	var req updateIssueRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	issue, err := d.store.GetIssue(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If status is changing, use a status_change or close event.
	statusChanged := false
	if req.Status != "" && model.Status(req.Status) != issue.Status {
		statusChanged = true
		newStatus := model.Status(req.Status)

		var action model.Action
		if newStatus == model.StatusClosed {
			action = model.ActionClose
		} else {
			action = model.ActionStatusChange
		}

		payload := model.EventPayload{
			Status:     newStatus,
			FromStatus: issue.Status,
			Comment:    req.Comment,
		}
		payloadJSON, _ := json.Marshal(payload)

		event := &model.Event{
			RepoID:    issue.RepoID,
			IssueID:   issue.ID,
			Timestamp: now,
			Action:    action,
			Payload:   string(payloadJSON),
			Synced:    0,
		}

		savedEvent, err := d.store.AppendEvent(ctx, event)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
			return
		}

		issue, err = engine.Apply(issue, savedEvent)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "apply event: "+err.Error())
			return
		}
	}

	// If there are non-status field changes, generate an update event.
	hasFieldChange := req.Title != "" || req.Description != "" ||
		req.Priority != nil || req.IssueType != "" || req.Labels != nil
	if hasFieldChange {
		// If the comment was already attached to a status_change event, don't duplicate it.
		comment := req.Comment
		if statusChanged {
			comment = ""
		}
		payload := model.EventPayload{
			Title:       req.Title,
			Description: req.Description,
			Priority:    req.Priority,
			IssueType:   req.IssueType,
			Labels:      req.Labels,
			Comment:     comment,
		}
		payloadJSON, _ := json.Marshal(payload)

		event := &model.Event{
			RepoID:    issue.RepoID,
			IssueID:   issue.ID,
			Timestamp: now,
			Action:    model.ActionUpdate,
			Payload:   string(payloadJSON),
			Synced:    0,
		}

		savedEvent, err := d.store.AppendEvent(ctx, event)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
			return
		}

		issue, err = engine.Apply(issue, savedEvent)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "apply event: "+err.Error())
			return
		}
	}

	// If there's a comment but no other changes carried it, generate a standalone comment event.
	if req.Comment != "" && !hasFieldChange && !statusChanged {
		payload := model.EventPayload{
			Comment: req.Comment,
		}
		payloadJSON, _ := json.Marshal(payload)

		event := &model.Event{
			RepoID:    issue.RepoID,
			IssueID:   issue.ID,
			Timestamp: now,
			Action:    model.ActionComment,
			Payload:   string(payloadJSON),
			Synced:    0,
		}

		savedEvent, err := d.store.AppendEvent(ctx, event)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
			return
		}

		issue, err = engine.Apply(issue, savedEvent)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "apply event: "+err.Error())
			return
		}
	}

	if err := d.store.UpdateIssue(ctx, issue); err != nil {
		writeError(w, http.StatusInternalServerError, "update issue: "+err.Error())
		return
	}

	// Re-fetch to get the canonical stored state.
	issue, err = d.store.GetIssue(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func (d *Daemon) deleteIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue id")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	issue, err := d.store.GetIssue(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Append a delete event.
	deletePayload := model.EventPayload{
		FromStatus: issue.Status,
	}
	deletePayloadJSON, _ := json.Marshal(deletePayload)

	event := &model.Event{
		RepoID:    issue.RepoID,
		IssueID:   issue.ID,
		Timestamp: now,
		Action:    model.ActionDelete,
		Payload:   string(deletePayloadJSON),
		Synced:    0,
	}

	if _, err := d.store.AppendEvent(ctx, event); err != nil {
		writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
		return
	}

	// Soft-delete in store.
	if err := d.store.DeleteIssue(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete issue: "+err.Error())
		return
	}

	// Re-fetch to return current state.
	issue, err = d.store.GetIssue(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

type assignIssueRequest struct {
	Owner string `json:"owner"`
}

func (d *Daemon) assignIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue id")
		return
	}

	var req assignIssueRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	issue, err := d.store.GetIssue(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Append an assign event.
	payload := model.EventPayload{
		Owner: req.Owner,
	}
	payloadJSON, _ := json.Marshal(payload)

	event := &model.Event{
		RepoID:    issue.RepoID,
		IssueID:   issue.ID,
		Timestamp: now,
		Action:    model.ActionAssign,
		Payload:   string(payloadJSON),
		Synced:    0,
	}

	savedEvent, err := d.store.AppendEvent(ctx, event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
		return
	}

	issue, err = engine.Apply(issue, savedEvent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "apply event: "+err.Error())
		return
	}

	if err := d.store.UpdateIssue(ctx, issue); err != nil {
		writeError(w, http.StatusInternalServerError, "update issue: "+err.Error())
		return
	}

	// Re-fetch.
	issue, err = d.store.GetIssue(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

// ---------------------------------------------------------------------------
// Comment on issue
// ---------------------------------------------------------------------------

type commentIssueRequest struct {
	Comment string `json:"comment"`
}

func (d *Daemon) commentIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue id")
		return
	}

	var req commentIssueRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Comment == "" {
		writeError(w, http.StatusBadRequest, "comment is required")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	issue, err := d.store.GetIssue(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload := model.EventPayload{
		Comment: req.Comment,
	}
	payloadJSON, _ := json.Marshal(payload)

	event := &model.Event{
		RepoID:    issue.RepoID,
		IssueID:   issue.ID,
		Timestamp: now,
		Action:    model.ActionComment,
		Payload:   string(payloadJSON),
		Synced:    0,
	}

	savedEvent, err := d.store.AppendEvent(ctx, event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "append event: "+err.Error())
		return
	}

	issue, err = engine.Apply(issue, savedEvent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "apply event: "+err.Error())
		return
	}

	if err := d.store.UpdateIssue(ctx, issue); err != nil {
		writeError(w, http.StatusInternalServerError, "update issue: "+err.Error())
		return
	}

	issue, err = d.store.GetIssue(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, issue)
}
