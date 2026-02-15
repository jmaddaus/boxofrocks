# Box of Rocks — Lightweight Issue Tracker for AI Agents

## Problem

Beads (the current solution) embeds issue data inside git repos, requiring complex sync mechanisms (merge drivers, worktrees, daemons, JSONL export/import). This breaks down with multiple worktrees, PR-based merges, and sandboxed environments.

## Core Principle

**Single source of truth via a local daemon. No files in the repo. No git involvement. Event-sourced state with GitHub Actions as the arbiter.**

## Architecture

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Agent in    │   │  Agent in    │   │  CLI from    │
│  Sandbox 1   │   │  Sandbox 2   │   │  Terminal    │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                  │                  │
       │    HTTP (localhost:PORT)             │
       └──────────┬───────┴──────────────────┘
                  │
         ┌────────▼────────┐
         │   Daemon         │
         │   (Go binary)    │
         │                  │
         │  ┌────────────┐  │
         │  │  SQLite DB  │  │  (local cache)
         │  └────────────┘  │
         │                  │
         │  Sync goroutine  │──── GitHub Issues API
         └─────────────────┘     (comments = event log)
                                  (body = computed snapshot)
                                        │
                                 ┌──────▼───────┐
                                 │  GH Action    │
                                 │  Arbiter      │
                                 │               │
                                 │  Triggers on  │
                                 │  new comments │
                                 │  Replays log  │
                                 │  Writes body  │
                                 └───────────────┘
```

## Components

### 1. Daemon (Go binary)

Single long-running process per machine. Listens on `127.0.0.1:<PORT>`.

- Embedded SQLite database at `~/.boxofrocks/bor.db`
- REST API for all operations
- Background sync goroutine polls GitHub Issues every 5 seconds
- Uses ETags to minimize API usage (~720 reads/hour worst case)
- Caches locally, writes propagate to GitHub in background

### 2. CLI

Thin HTTP client that talks to the daemon. Drop-in replacement for `bd` commands.

```
bor list                          # list open issues
bor list --all                    # list all issues
bor create "Fix the bug" -p 1    # create issue
bor close <id>                    # close issue
bor update <id> --status active   # update status
bor next                          # get next available task
```

All output is JSON by default (agent-friendly). Add `--pretty` for human-readable.

### 3. GitHub Sync (Event Sourced)

GitHub Issues is the remote store. Issue comments are the event log. Issue body is the computed snapshot.

- Agent actions append JSON events as GitHub Issue comments
- Daemon polls comments every 5 seconds, replays log to derive state
- ETags prevent unnecessary API calls (~720 reads/hour worst case)
- Local writes update cache immediately (other agents see changes instantly)
- GitHub Action arbiter triggers on new comments, writes authoritative state to issue body

## Data Model

### Issue

| Field        | Type     | Description                    |
|-------------|----------|--------------------------------|
| id          | string   | Short hash ID (e.g., `sdu-a1b2`) |
| github_id   | int      | GitHub Issue number            |
| title       | string   | Issue title                    |
| status      | string   | `open`, `in_progress`, `blocked`, `in_review`, `closed` |
| priority    | int      | 0-4 (0 = highest)             |
| issue_type  | string   | `bug`, `feature`, `task`, `epic` |
| description | string   | Issue body                     |
| owner       | string   | Assigned agent/user            |
| labels      | []string | Labels                         |
| created_at  | datetime |                                |
| updated_at  | datetime |                                |
| closed_at   | datetime | Null if open                   |

## API

All endpoints return JSON. Base URL: `http://127.0.0.1:<PORT>`

```
GET    /issues                    # list issues (query params: status, priority, type, owner)
GET    /issues/:id                # get single issue
POST   /issues                    # create issue
PATCH  /issues/:id                # update issue fields
DELETE /issues/:id                # delete issue

GET    /issues/next               # get highest priority open unassigned issue
POST   /issues/:id/assign         # assign to agent/user

GET    /health                    # daemon health + sync status
POST   /sync                      # force immediate sync with GitHub
```

## Authentication (Zero Config)

On startup, the daemon resolves a GitHub token automatically in order:

1. `git credential fill` — uses existing git HTTPS credentials
2. `gh auth token` — uses GitHub CLI token
3. `GITHUB_TOKEN` env var
4. OAuth Device Flow — interactive first-time setup, token stored in `~/.boxofrocks/token`

User never manually configures auth.

## Docker / Sandbox Access

Agents in Docker containers reach the daemon via:

- `host.docker.internal:<PORT>` (Docker Desktop)
- `--network host` (Linux)
- Mounted unix socket as alternative

CLI binary can be baked into container images. Same commands, just pointed at the host.

```bash
# Inside container
export TRACKER_HOST=http://host.docker.internal:8042
bor list
bor close sdu-a1b2
```

## Installation

```bash
# Install
go install github.com/<org>/boxofrocks@latest

# Start daemon (auto-detects GitHub auth)
bor daemon start

# Initialize for a GitHub repo
bor init --repo jmaddaus/SDU

# Use
bor create "Implement repair bay" -p 2 -t feature
bor list
```

## What This Doesn't Do (By Design)

- No files in the repo
- No git merge drivers
- No JSONL export/import
- No worktree management
- No per-repo daemon
- No sync branches
- No complex conflict resolution (append-only event log + arbiter eliminates conflicts)

## Event Sourcing & Conflict Resolution

### Model

Agent actions are **requests**, not direct state mutations. State is derived from an append-only event log stored as GitHub Issue comments.

- **Issue body** = structured JSON metadata (computed snapshot, written only by the arbiter)
- **Issue comments** = append-only event log (written by agents and humans)

### Event Format

Each comment is a JSON event appended by an agent or human:

```json
{"timestamp":"2026-02-15T10:00:00Z","action":"status_change","from":"open","to":"in_progress","agent":"agent-1"}
{"timestamp":"2026-02-15T10:05:00Z","action":"assign","to":"agent-2","agent":"agent-1"}
{"timestamp":"2026-02-15T10:10:00Z","action":"close","reason":"Completed","agent":"agent-2"}
```

### Issue Body (Computed Snapshot)

The issue body contains human-readable description plus a hidden JSON block with current computed state:

```markdown
Implement repair bay with animated progress bar

## Acceptance Criteria
- Working feature, tested
- Progress bar animation

<!-- boxofrocks
{"priority":2,"issue_type":"feature","status":"in_progress","owner":"agent-2","local_id":"sdu-a1b2"}
-->
```

HTML comment hides structured data from GitHub web UI. Humans see a clean issue. Agents parse the JSON block.

### Three-Layer Reconciliation

```
┌─────────────────────────────────────────────────────────┐
│  Layer 1: Agent Daemon (local, optimistic)              │
│  - Appends comment events to GitHub Issues              │
│  - Reads all comments, replays in timestamp order       │
│  - Derives local state optimistically                   │
│  - Acts immediately without waiting for arbiter         │
│  - NEVER generates events from reconciliation           │
│  - Only generates events from explicit agent actions    │
├─────────────────────────────────────────────────────────┤
│  Layer 2: Local Cache (SQLite)                          │
│  - Stores derived state for instant reads               │
│  - Updated every sync cycle from comment replay         │
│  - Multiple agents on same machine share one cache      │
├─────────────────────────────────────────────────────────┤
│  Layer 3: GitHub Action Arbiter (authoritative)         │
│  - Triggers on issue_comment events                     │
│  - Replays full comment log                             │
│  - Computes authoritative state                         │
│  - Writes computed state to issue body                  │
│  - Single writer — no race conditions on body           │
│  - Can enforce rules agents cannot override             │
└─────────────────────────────────────────────────────────┘
```

### Arbiter Rules

The arbiter is the authority. It can enforce policies that agents must respect:

- Agent requests close but acceptance criteria aren't met → arbiter rejects, adds comment back
- Two agents claim the same task → arbiter assigns to whoever requested first
- Agent tries to escalate priority → arbiter checks permissions
- Invalid state transition (e.g., closing an already-closed issue) → arbiter ignores the event

Agents see each other's proposals immediately via the comment log. The issue body is the final word if there's disagreement.

### Arbiter GitHub Action

```yaml
name: Issue Reconciler
on:
  issue_comment:
    types: [created]

jobs:
  reconcile:
    runs-on: ubuntu-latest
    steps:
      - name: Reconcile issue state
        uses: actions/github-script@v7
        with:
          script: |
            // Fetch all comments (events)
            // Filter to boxofrocks JSON events
            // Replay in timestamp order
            // Apply rules (permissions, valid transitions)
            // Compute final state
            // Write state to issue body
```

### Why No Race Conditions

- **Comments are append-only** — nothing is ever overwritten or lost
- **Events are never generated from reconciliation** — only from explicit actions
- **Arbiter is the single writer** to the issue body — no contention
- **Agents self-reconcile** from the same event log — two agents replaying the same log always derive the same state
- **Stale reads are harmless** — worst case an agent acts on 5-second-old info, which self-corrects on next sync
- **No ping-pong** — agents propose, arbiter decides. No correction loops.

### Resource Usage

- GitHub Actions: Each reconciliation ~2-3 seconds. Free tier (2,000 min/month) supports ~40,000-60,000 reconciliations
- GitHub API: Daemon uses ~720 reads/hour with ETags. Well within 5,000/hour limit
- Arbiter GITHUB_TOKEN: 1,000 requests/hour/repo — more than sufficient
- Trigger latency: `issue_comment` triggers are not instant (seconds to minutes delay) — doesn't matter because agents read the event log directly
