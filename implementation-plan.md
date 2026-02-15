# boxofrocks — Implementation Plan

## Context

The current issue tracker ("beads") embeds issue data in git repos, requiring merge drivers, worktrees, and sync daemons. This breaks down with multiple worktrees, PR-based merges, and sandboxed agent environments.

boxofrocks (Box of Rocks, CLI binary: `bor`) replaces this with a daemon + CLI architecture backed by GitHub Issues as the remote store. Issues are event-sourced: comments are an append-only event log, a GitHub Action arbiter computes authoritative state. The daemon caches locally in SQLite for instant reads and manages sync in the background.

**Key decisions:**
- Generic/configurable (no hardcoded repo defaults)
- Multi-repo at daemon level (one daemon manages multiple repos on one machine)
- Single repo per arbiter (each GH repo gets its own Action workflow)
- Auth: 3 methods only (git credentials, gh CLI, env var) — OAuth deferred
- GitHub Action arbiter is in scope

---

## Project Structure

```
boxofrocks/
├── cmd/bor/main.go                        # Single binary (daemon + CLI dispatch)
├── internal/
│   ├── model/
│   │   ├── issue.go                       # Issue struct, status/priority/type constants
│   │   ├── event.go                       # Event struct, action types
│   │   └── repo.go                        # RepoConfig struct
│   ├── store/
│   │   ├── store.go                       # Store interface
│   │   ├── sqlite.go                      # SQLite implementation
│   │   └── migrations.go                  # Schema creation/migration
│   ├── engine/
│   │   ├── engine.go                      # Event replay → derived issue state
│   │   ├── engine_test.go
│   │   └── rules.go                       # State transition validation
│   ├── github/
│   │   ├── client.go                      # GitHub REST API (issues, comments, ETags, pagination)
│   │   ├── auth.go                        # Token resolution chain (3 methods)
│   │   └── parser.go                      # Parse/render boxofrocks JSON in issue body
│   ├── sync/
│   │   ├── syncer.go                      # SyncManager + per-repo RepoSyncer goroutines
│   │   └── reconciler.go                  # Incremental event processing + reconciliation
│   ├── daemon/
│   │   ├── daemon.go                      # Lifecycle (foreground, signal handling)
│   │   ├── server.go                      # HTTP server setup, middleware
│   │   ├── routes.go                      # Route registration
│   │   └── handlers.go                    # REST handlers (issue CRUD, health, sync)
│   ├── cli/
│   │   ├── root.go                        # Subcommand dispatch, global flags
│   │   ├── client.go                      # HTTP client wrapper to daemon
│   │   ├── output.go                      # JSON / --pretty formatting
│   │   ├── repo.go                        # Auto-detect repo from git remote
│   │   ├── daemon.go                      # bor daemon start/stop/status
│   │   ├── init.go                        # bor init --repo owner/name
│   │   ├── list.go, create.go, close.go   # Issue commands
│   │   ├── update.go, next.go             # Issue commands
│   │   └── assign.go                      # Assignment command
│   └── config/
│       └── config.go                      # ~/.boxofrocks/config.json management
├── arbiter/
│   ├── cmd/reconcile/main.go              # Go binary for arbiter (same engine as daemon)
│   ├── action.yml                         # Composite GitHub Action
│   └── README.md                          # Installation guide
├── go.mod
└── boxofrocks-spec.md
```

**External dependency:** `modernc.org/sqlite` (pure-Go SQLite, no CGO). Everything else uses Go stdlib.

---

## Design Decisions

### Issue IDs: `owner/repo` scoping

No generated prefix. Issue IDs use the full `owner/repo` scoped in the local database. For display and CLI usage, use a sequential numeric ID per repo (e.g., `#1`, `#2`). The mapping to GitHub Issue number is stored in the DB. When multiple repos are registered, CLI commands require `--repo` or auto-detect from git remote.

Rationale: Generated prefixes ("first letter + consonants") collide easily ("server" vs "service"). `owner/repo` is guaranteed unique by GitHub.

### Daemon runs in foreground only

`bor daemon start` runs in the foreground. No PID file management, no backgrounding via os/exec self-launch. Users who want background operation use systemd, launchd, or `nohup`/`&`.

Rationale: PID file management is fiddly — stale PIDs, crashed processes, cross-platform differences. Foreground-only is simpler and more debuggable.

### Incremental event replay (not full replay every cycle)

Each repo tracks `last_processed_comment_id` per GitHub issue. On each sync cycle, only fetch and process comments newer than the last processed ID. Full replay only happens on first sync or explicit `bor sync --full`.

Schema addition:
```sql
CREATE TABLE issue_sync_state (
    repo_id            INTEGER NOT NULL,
    github_issue_number INTEGER NOT NULL,
    last_comment_id    INTEGER NOT NULL DEFAULT 0,
    last_comment_at    TEXT,
    PRIMARY KEY (repo_id, github_issue_number)
);
```

Rationale: Issues can accumulate hundreds of comments. Full replay every 5 seconds is wasteful.

### Arbiter written in Go (not JS)

The arbiter is a small Go binary (`arbiter/cmd/reconcile/main.go`) that reuses the same `internal/engine` package as the daemon. A release workflow pre-builds the binary for linux/amd64 and attaches it to GitHub Releases. The arbiter Action downloads the pre-built binary instead of compiling from source on every trigger — avoids 10-20 seconds of `go build` latency per reconciliation.

Rationale: Two implementations in different languages will drift. Shared Go code guarantees consistency.

### Event comment marker

Agent-tracker events use a dedicated prefix to distinguish them from human comments:

```
[boxofrocks] {"timestamp":"...","action":"status_change",...}
```

The `[boxofrocks]` prefix is used for:
- **GitHub Action filter:** `if: startsWith(github.event.comment.body, '[boxofrocks]')` — more specific than matching any `{`
- **Comment parsing:** Daemon and arbiter strip the prefix before parsing JSON
- **Human-friendly:** The prefix is visible in comments, making it clear what they are

Human comments and random JSON pastes are ignored entirely.

### Label auto-creation

`bor init` creates the `boxofrocks` label on the repo if it doesn't already exist. The label is used to filter issues during sync. If label creation fails (e.g., permissions), `bor init` warns but continues — the user can create it manually. The sync cycle also attempts label creation on the first issue create if it hasn't been created yet, handling the 422 gracefully.

### Web-created issues are handled

When the sync pulls a GitHub issue with the `boxofrocks` label but no `<!-- boxofrocks ... -->` metadata block:
1. The daemon treats it as a new issue, generates a synthetic `create` event from the issue title/body/labels
2. Posts the `create` event as a comment
3. The arbiter then writes the metadata block into the issue body

This allows humans to create issues via GitHub's web UI and have them enter the tracking system.

### DELETE is a soft-delete event

`DELETE /issues/{id}` appends a `delete` event (synced=0) and marks the issue as `status: deleted` locally. The issue is excluded from `list` and `next` results but remains in the database and event log. Consistent with the append-only event sourcing model.

### GitHub API pagination

The GitHub client handles pagination via `Link` headers. `ListComments` and `ListIssues` follow `rel="next"` links automatically, collecting all pages before returning. This is essential for issues with >100 comments.

### Multi-repo rate limiting

The SyncManager distributes API budget across repos. With a 5000 req/hour limit:
- Each repo's poll interval is adjusted based on the number of registered repos
- For N repos, effective interval = `max(5s, 5s * N / 2)` (ensures total API usage stays within budget)
- Polling is staggered (not all repos poll at the same instant)
- Rate limit headers from GitHub responses are tracked and respected globally

### Offline / disconnected operation

- **After init:** If GitHub is unreachable, daemon starts and serves from local cache. Issues can be created, updated, closed — all stored as pending events (synced=0). When connectivity returns, pending events sync on the next cycle.
- **First-time init:** `bor init --repo owner/name` requires connectivity to validate the repo exists and discover existing issues. Without connectivity, `bor init --repo owner/name --offline` skips validation and starts with an empty state, syncing when connectivity becomes available.

---

## SQLite Schema

```sql
CREATE TABLE repos (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    owner            TEXT NOT NULL,
    name             TEXT NOT NULL,
    poll_interval_ms INTEGER DEFAULT 5000,
    last_sync_at     TEXT,
    issues_etag      TEXT DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(owner, name)
);

CREATE TABLE issues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- local sequential ID
    repo_id     INTEGER NOT NULL REFERENCES repos(id),
    github_id   INTEGER,                            -- GitHub Issue number (null before sync)
    title       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open',       -- open, in_progress, closed, deleted
    priority    INTEGER NOT NULL DEFAULT 2,
    issue_type  TEXT NOT NULL DEFAULT 'task',
    description TEXT DEFAULT '',
    owner       TEXT DEFAULT '',
    labels      TEXT DEFAULT '[]',                  -- JSON array
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    closed_at   TEXT
);

CREATE INDEX idx_issues_repo_status ON issues(repo_id, status);
CREATE INDEX idx_issues_repo_priority ON issues(repo_id, priority);
CREATE INDEX idx_issues_github_id ON issues(repo_id, github_id);

CREATE TABLE events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id           INTEGER NOT NULL REFERENCES repos(id),
    github_comment_id INTEGER,
    issue_id          INTEGER NOT NULL,              -- local issue ID (no FK: events may arrive before issue row)
    github_issue_number INTEGER,                     -- GitHub issue number (for correlation)
    timestamp         TEXT NOT NULL,
    action            TEXT NOT NULL,
    payload           TEXT NOT NULL,                 -- full JSON event
    agent             TEXT DEFAULT '',
    synced            INTEGER DEFAULT 0,             -- 0=pending, 1=synced
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_events_repo_issue ON events(repo_id, issue_id);
CREATE INDEX idx_events_synced ON events(synced);

CREATE TABLE issue_sync_state (
    repo_id              INTEGER NOT NULL,
    github_issue_number  INTEGER NOT NULL,
    last_comment_id      INTEGER NOT NULL DEFAULT 0,
    last_comment_at      TEXT,
    PRIMARY KEY (repo_id, github_issue_number)
);
```

---

## Phases

### Phase 0: Project Scaffolding

**Goal:** Compilable Go module. `go build` works. `bor help` prints usage.

**Create:**
- `go.mod` (module `github.com/jmaddaus/boxofrocks`, Go 1.22+)
- `cmd/bor/main.go` — minimal main with subcommand dispatch skeleton, prints help/version
- `internal/model/issue.go` — Issue struct, Status/Priority/IssueType constants
- `internal/model/event.go` — Event struct, Action constants
- `internal/model/repo.go` — RepoConfig struct

**Verify:** `go build ./...` and `go vet ./...` pass.

---

### Phase 1: SQLite Store

**Goal:** Full CRUD on issues, events, and repos via SQLite. Thoroughly tested in isolation.

**Create:**
- `internal/store/store.go` — `Store` interface
- `internal/store/sqlite.go` — SQLite implementation using `modernc.org/sqlite`
- `internal/store/migrations.go` — Schema creation/migration
- `internal/store/sqlite_test.go`

**Key interface methods:**
- Repos: `AddRepo`, `GetRepo`, `ListRepos`, `UpdateRepo`
- Issues: `CreateIssue`, `GetIssue`, `ListIssues(filter)`, `UpdateIssue`, `DeleteIssue`, `NextIssue`
- Events: `AppendEvent`, `ListEvents`, `PendingEvents`, `MarkEventSynced`
- Sync state: `GetIssueSyncState`, `SetIssueSyncState`

**`NextIssue`** returns highest-priority open unassigned issue (`ORDER BY priority ASC, created_at ASC` where `status='open' AND owner=''`).

**Tests:** Create/read/update/delete, list with filters, next issue priority ordering, event lifecycle, concurrent access, migration idempotency.

---

### Phase 2: Event Engine

**Goal:** Pure-logic event replay that derives issue state from a sequence of events. No I/O.

**Create:**
- `internal/engine/engine.go` — `Replay(events) → map[issueID]*Issue` and `Apply(issue, event) → issue`
- `internal/engine/rules.go` — `ValidTransition(from, to Status) bool`
- `internal/engine/engine_test.go`, `rules_test.go`
- `internal/engine/testdata/` — JSON fixture files for event sequences and expected outputs

**State transitions:**
- `open` → `in_progress`, `closed`
- `in_progress` → `open`, `closed`
- `closed` → `open` (reopen)
- Invalid transitions are silently ignored during replay

**Event application:** `create` initializes issue; `status_change` updates status; `assign` sets owner; `close` sets status+closed_at; `update` patches fields; `delete` sets status to deleted.

**`Apply` method:** Applies a single event to an existing issue state. Used for incremental processing (apply only new events without full replay).

**Fixture files:** JSON test fixtures in `testdata/` that define event sequences and expected issue states. These same fixtures will be used by the arbiter binary, ensuring both produce identical results from the same input.

**Tests:** Empty replay, single create, full lifecycle, invalid transitions ignored, multiple interleaved issues, timestamp ordering, incremental Apply matches full Replay result.

---

### Phase 3: HTTP Daemon (Local Only, No GitHub Sync)

**Goal:** Working REST API backed by SQLite. All endpoints functional. Foreground daemon.

**Create:**
- `internal/daemon/daemon.go` — Start (foreground), signal handling (SIGINT/SIGTERM → graceful shutdown)
- `internal/daemon/server.go` — HTTP server, middleware (request logging, content-type)
- `internal/daemon/routes.go` — Route registration (Go 1.22 `ServeMux`)
- `internal/daemon/handlers.go` — All REST handlers
- `internal/daemon/handlers_test.go`
- `internal/config/config.go` — `~/.boxofrocks/config.json` management

**REST API (all return JSON):**
```
GET    /issues              — list (query: status, priority, type, owner, repo)
GET    /issues/next         — highest-priority open unassigned
GET    /issues/{id}         — get single
POST   /issues              — create (appends "create" event with synced=0)
PATCH  /issues/{id}         — update fields (appends "update" event)
DELETE /issues/{id}         — soft-delete (appends "delete" event)
POST   /issues/{id}/assign  — assign (appends "assign" event)
GET    /health              — health + sync status per repo
POST   /sync                — force sync (query: repo)
POST   /repos               — register repo (used by `bor init`)
GET    /repos               — list registered repos
```

**Repo resolution:** `?repo=owner/name` query param → `X-Repo` header → implicit if single repo → 400 error.

**Handler pattern:** Resolve repo → validate input → generate event (synced=0) → apply event to local state via `engine.Apply()` → return response.

**Tests:** httptest-based tests for every endpoint, valid and invalid inputs, repo resolution logic.

---

### Phase 4: CLI Client

**Goal:** Full CLI. End-to-end local workflow: start daemon, init repo, create/list/update/close issues.

**Create:**
- `internal/cli/root.go` — Dispatch, global flags (`--host`, `--repo`, `--pretty`)
- `internal/cli/client.go` — HTTP client wrapper to daemon
- `internal/cli/output.go` — JSON (default) and pretty-print (tabwriter)
- `internal/cli/repo.go` — Auto-detect repo from `git remote get-url origin`
- `internal/cli/daemon.go` — `bor daemon start` (foreground), `bor daemon status` (ping /health)
- `internal/cli/init.go` — `bor init --repo owner/name [--offline]`
- `internal/cli/list.go` — `bor list [--all] [--status X] [--priority N]`
- `internal/cli/create.go` — `bor create "title" [-p priority] [-t type] [-d description]`
- `internal/cli/close.go` — `bor close <id>`
- `internal/cli/update.go` — `bor update <id> [--status S] [--priority N] ...`
- `internal/cli/next.go` — `bor next`
- `internal/cli/assign.go` — `bor assign <id> <owner>`

**No backgrounding logic.** `bor daemon start` runs in the foreground. Users use systemd/launchd/nohup for background operation.

**CLI env var:** `TRACKER_HOST` overrides base URL (default `http://127.0.0.1:8042`). Used by Docker containers pointing at `host.docker.internal`.

**Repo auto-detection:** Parse `git remote get-url origin` to extract `owner/name` from HTTPS or SSH URLs. Falls back to `--repo` flag.

**Tests:** Client unit tests with httptest mock, repo detection for various URL formats, output formatting.

---

### Phase 5: GitHub Auth and API Client

**Goal:** Daemon can authenticate with GitHub and read/write issues and comments. Full pagination support.

**Create:**
- `internal/github/auth.go` — `ResolveToken()` trying: `git credential fill` → `gh auth token` → `GITHUB_TOKEN`
- `internal/github/client.go` — GitHub REST API client with ETag support and pagination
- `internal/github/parser.go` — Parse/render `<!-- boxofrocks ... -->` JSON in issue bodies
- Tests for all three files

**GitHub client interface:**
```go
type Client interface {
    ListIssues(ctx, owner, repo, opts ListOpts) ([]*GitHubIssue, string, error)
    CreateIssue(ctx, owner, repo, title, body, labels) (*GitHubIssue, error)
    UpdateIssueBody(ctx, owner, repo, number, body) error
    ListComments(ctx, owner, repo, number, opts ListOpts) ([]*GitHubComment, string, error)
    CreateComment(ctx, owner, repo, number, body) (*GitHubComment, error)
}

type ListOpts struct {
    ETag    string  // If-None-Match header
    Since   string  // Only items after this ID/timestamp
    PerPage int     // Items per page (max 100)
}
```

**Pagination:** Follow `Link: <url>; rel="next"` headers automatically. Collect all pages before returning. Required for issues with >100 comments.

**Rate limiting:** Track `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers. If remaining < 100, slow down. If 0, sleep until reset. Rate limit state is shared across all repos via the SyncManager.

**ETag/304:** Return cached result on 304 Not Modified. Store ETags per-request-URL.

**Tests:** httptest mocking GitHub API (including pagination with Link headers), auth fallback chain, parser with various body formats, rate limit handling.

---

### Phase 6: GitHub Sync

**Goal:** Bidirectional sync between daemon and GitHub. Events flow both ways. Incremental processing.

**Create:**
- `internal/sync/syncer.go` — `SyncManager` (manages per-repo goroutines, shared rate limit) + `RepoSyncer`
- `internal/sync/reconciler.go` — Incremental event processing + full replay fallback
- Tests for both

**SyncManager:**
- Starts/stops `RepoSyncer` goroutines
- Holds shared rate limit state across repos
- Adjusts per-repo poll intervals based on number of repos: `max(5s, 5s * N / 2)`
- Staggers initial poll times so repos don't all poll simultaneously
- Methods: `AddRepo`, `RemoveRepo`, `ForceSync`, `Status`, `Stop`

**RepoSyncer poll cycle:**
1. **Push outbound:** Query pending events (synced=0).
   - For creates where `github_id` is null: call `CreateIssue` on GitHub (with `boxofrocks` label and initial body), store returned issue number, post create event as first comment.
   - For other events: look up `github_id`, post event as comment.
   - Mark synced on success.
2. **Pull inbound (incremental):**
   - List GitHub issues with `boxofrocks` label (using ETag).
   - For each issue, check `issue_sync_state.last_comment_id`.
   - Fetch only comments with `since` parameter (comments after `last_comment_at`).
   - Parse new JSON events from comment bodies (strip `[boxofrocks]` prefix).
   - Apply new events incrementally via `engine.Apply()`.
   - Update `issue_sync_state` with latest comment ID.
3. **Handle web-created issues:** If a GitHub issue has `boxofrocks` label but no `<!-- boxofrocks -->` block and no tracked local issue, generate a synthetic `create` event from the issue metadata, post it as a comment, and create the local issue.
4. **Update metadata:** `last_sync_at`, ETags.

**Force full replay:** `POST /sync?full=true` triggers full comment fetch + `engine.Replay()` for all issues in a repo. Used for recovery or debugging.

**Tests:** Mock GitHub client, incremental pull (only new comments), push pending events, web-created issue handling, multi-repo staggering, rate limit distribution.

---

### Phase 7: `bor init` and Multi-Repo Wiring

**Goal:** Full init flow, multi-repo lifecycle, discovery of existing issues.

**Implement:**
- `bor init --repo owner/name [--offline]` registers repo and starts sync
- Online init: validate repo exists on GitHub, create `boxofrocks` label if missing, discover existing labeled issues, import them via full replay
- Offline init (`--offline`): skip GitHub validation and label creation, start with empty state, sync and create label when connectivity becomes available
- Wire SyncManager into daemon lifecycle (start syncers for all registered repos on daemon start)
- Handle daemon start with no connectivity (start syncers, they retry with backoff)

**Tests:** Init new repo (online), init duplicate (error), offline init, discovery of pre-existing issues, daemon restart with registered repos.

---

### Phase 8: GitHub Action Arbiter (Go binary)

**Goal:** GitHub Action that triggers on `issue_comment`, replays events via a Go binary, writes authoritative state to issue body.

**Create:**
- `arbiter/cmd/reconcile/main.go` — Go binary that:
  1. Reads issue number and repo from env vars (`GITHUB_REPOSITORY`, inputs)
  2. Fetches all comments via GitHub API (using `GITHUB_TOKEN`)
  3. Filters to valid boxofrocks JSON events (strips `[boxofrocks]` prefix)
  4. Replays via `internal/engine.Replay()` (same code as daemon)
  5. Reads current issue body, extracts human text via `internal/github/parser`
  6. Writes updated body with computed state in `<!-- boxofrocks -->` block
- `arbiter/action.yml` — Composite action metadata
- `arbiter/README.md` — Installation and setup guide

**Example workflow for users:**
```yaml
name: Box of Rocks Reconciler
on:
  issue_comment:
    types: [created]
jobs:
  reconcile:
    if: startsWith(github.event.comment.body, '[boxofrocks]')
    runs-on: ubuntu-latest
    steps:
      - name: Download arbiter
        run: |
          curl -sL https://github.com/jmaddaus/boxofrocks/releases/latest/download/reconcile-linux-amd64 -o /tmp/reconcile
          chmod +x /tmp/reconcile
      - name: Reconcile
        run: /tmp/reconcile
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          ISSUE_NUMBER: ${{ github.event.issue.number }}
```

**Release workflow:** A separate workflow builds the arbiter binary on tagged releases (`go build -o reconcile-linux-amd64 ./arbiter/cmd/reconcile`) and attaches it to the GitHub Release. This avoids compiling from source on every reconciliation trigger.

**Shared test fixtures:** The `internal/engine/testdata/` fixtures from Phase 2 are used by both the daemon's engine tests and the arbiter binary's tests, ensuring identical behavior.

**Arbiter rules:** Invalid transitions ignored, first assigner wins on conflicts.

**Tests:** Arbiter binary integration test (mock GitHub API, feed events, verify body output). Cross-validation: run same fixture through `engine.Replay()` directly and through the arbiter binary, compare outputs.

---

### Phase 9: Polish, Hardening, Documentation

**Goal:** Production readiness.

- Structured logging throughout (`log/slog`)
- Graceful shutdown with context timeout on SIGINT/SIGTERM
- Port conflict handling (clear error message if port already in use)
- Config validation (repo format `owner/name`, port range)
- `bor daemon status` shows per-repo sync status, last sync time, pending event count, uptime
- `--pretty` output polished for all commands (tabwriter alignment)
- Consistent, actionable error messages (e.g., "daemon not running, start with: bor daemon start")
- README.md: installation, quickstart, configuration, Docker/sandbox usage, arbiter setup
- Docker documentation: `TRACKER_HOST=http://host.docker.internal:8042`

---

## Phase Dependencies

```
Phase 0 (Scaffolding)
  ├── Phase 1 (Store)
  │     └── Phase 3 (Daemon) → Phase 4 (CLI) → Phase 7 (Init + Multi-Repo)
  ├── Phase 2 (Engine) ──────────────────────────┐
  └── Phase 5 (GitHub Auth+Client) ──────────────┤
                                                  └── Phase 6 (Sync) → Phase 7
Phase 8 (Arbiter) — depends on Phase 2 (engine) and Phase 5 (parser), otherwise independent
Phase 9 (Polish) — after all other phases
```

Phases 1, 2, and 5 can be built in parallel. Phase 8 can be built after Phases 2 and 5.

---

## Verification Plan

After each phase, run:
```bash
go build ./...
go vet ./...
go test ./...
```

End-to-end smoke test after Phase 7:
```bash
# Terminal 1: start daemon
bor daemon start

# Terminal 2: use CLI
bor init --repo owner/name
bor create "Test issue" -p 1 -t bug
bor list
bor next
bor update 1 --status in_progress
bor assign 1 agent-1
bor close 1
bor list --all
# Ctrl+C daemon in Terminal 1
```

Sync verification (requires GitHub token):
```bash
bor daemon start &
bor init --repo owner/testproject
bor create "Synced issue" -p 2 -t feature
# Wait 5-10s for sync
# Verify issue appears on GitHub with boxofrocks label and metadata block
# Verify event comment posted
# Create issue on GitHub web UI with boxofrocks label
# Wait 5-10s
bor list  # Should show the web-created issue
```

Offline verification:
```bash
# With no network
bor daemon start &
bor init --repo owner/name --offline
bor create "Offline issue" -p 1 -t task
bor list  # Shows the issue
# Restore network, wait for sync
# Issue appears on GitHub
```
