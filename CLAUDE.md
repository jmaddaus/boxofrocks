# CLAUDE.md - Box of Rocks (`bor`)

## Project Overview

Box of Rocks is a daemon + CLI issue tracker backed by GitHub Issues, written in Go. Issues are event-sourced: GitHub Issue comments form an append-only event log, the daemon caches locally in SQLite, and a GitHub Action arbiter computes authoritative state server-side.

**Module:** `github.com/jmaddaus/boxofrocks`
**Binary:** `cmd/bor/main.go` (single binary for both CLI and daemon)
**Go version:** 1.22+ (uses `ServeMux` path value routing)
**External dependency:** `modernc.org/sqlite` (pure Go, no CGO)

## Commands

```bash
go build ./...          # Build everything
go test ./...           # Run all tests (~120 tests across 5 packages)
go vet ./...            # Static analysis
go fmt ./...            # Format code
go test -run TestName ./internal/store/  # Run a specific test in a specific package
```

## Architecture

```
CLI ──HTTP/TCP──> Daemon ──> SQLite       (local, instant)
Agent ──unix sock─┘   │
                      └──sync──> GitHub Issues  (remote, background)
                                     │
                           GitHub Action (arbiter)
```

### Package Dependency Graph

```
cmd/bor → cli → daemon → {store, engine, sync, config}
                         sync → {store, engine, github}
arbiter → {engine, github}
model ← (used by all packages)
```

### Package Responsibilities

| Package | Role | Has tests |
|---------|------|-----------|
| `internal/model` | Data types: Issue, Event, RepoConfig, constants | No |
| `internal/store` | `Store` interface + SQLite implementation | Yes (33) |
| `internal/engine` | Pure-logic event replay (`Replay`, `Apply`) | Yes (21) |
| `internal/github` | GitHub REST API client, auth, body/comment parser, `IsTrustedAuthor` | Yes (37) |
| `internal/sync` | `SyncManager` + per-repo `RepoSyncer` goroutines, trusted-author filtering | Yes (17) |
| `internal/daemon` | HTTP server, routes, handlers, middleware, Unix socket lifecycle | Yes (25) |
| `internal/cli` | CLI commands, HTTP client to daemon, output formatting | No |
| `internal/config` | `~/.boxofrocks/config.json` management | No |
| `arbiter/cmd/reconcile` | Standalone binary for GitHub Action | No |

## Key Design Patterns

### Event Sourcing

Every mutation (create, update, close, assign, delete) appends an `Event` to the store with `synced=0`. The sync layer pushes these as `[boxofrocks] {...}` comments on GitHub Issues. Inbound comments are parsed and applied incrementally via `engine.Apply()`.

**Critical invariant:** `engine.Apply()` and `engine.Replay()` must produce identical results for the same event sequence. The arbiter binary uses the same engine package. Do not duplicate replay logic.

### Handler Pattern (daemon/handlers.go)

All issue-mutation handlers follow this pattern:
1. Resolve repo (query param → X-Repo header → implicit single repo)
2. Validate input
3. Generate event with `synced=0`
4. Apply event to local state via `engine.Apply()`
5. Persist to store
6. Return JSON response

### From-Status Validation (engine/rules.go)

Status change events include a `from_status` field declaring the expected current state. The engine skips stale events where `from_status` doesn't match the current computed state. Events without `from_status` (legacy) are always accepted. The `deleted` status is terminal — no further status changes are allowed.

**Statuses:** `open`, `in_progress`, `blocked`, `in_review`, `closed`, `deleted`
**Issue types:** `task`, `bug`, `feature`, `epic`

Stale/skipped events are silently ignored during replay (not errors). This is intentional for forward compatibility, conflict resolution, and race condition handling between agents.

### Sync Flow (sync/syncer.go)

Each `RepoSyncer` poll cycle:
1. **Push outbound:** query `PendingEvents(synced=0)`, post as GitHub comments, mark synced
2. **Pull inbound:** list GitHub issues with `boxofrocks` label, fetch new comments since `last_comment_id`, filter by `author_association` if `TrustedAuthorsOnly` is enabled, apply incrementally
3. **Web-created issues:** GitHub issues with `boxofrocks` label but no local match get a synthetic `create` event

**Trusted author filtering:** When `RepoConfig.TrustedAuthorsOnly` is true, inbound comments are filtered by `github.IsTrustedAuthor(c.AuthorAssociation)` before processing (both incremental and full replay paths). Trusted associations: OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR. Auto-enabled for public repos during `bor init`. The arbiter applies the same filter by checking repo visibility via `GetRepo`.

**Adaptive polling:** Each syncer tracks a `lastActivityAt` timestamp. If a cycle pushes outbound events or receives inbound changes, `lastActivityAt` is reset. Polling uses two tiers:
- **Fast** (5s base, scaled by repo count): used when `lastActivityAt` is within 2 minutes
- **Slow** (60s): used when idle longer than 2 minutes

Force sync always resets to fast tier. The `SyncStatus.Idle` field reports whether a repo is in slow mode.

### Interfaces for Testability

- `store.Store` — mocked with in-memory SQLite (`:memory:`) in tests
- `github.Client` — mocked with `mockGitHubClient` struct in `sync/syncer_test.go`
- Daemon handlers tested via `httptest` with `NewWithStore()` constructor

## Testing Conventions

- Use `":memory:"` SQLite for store tests (fast, isolated)
- Use `httptest.NewServer` / `httptest.NewRecorder` for HTTP tests
- Use `t.Helper()` in test helpers
- Use `t.Cleanup()` for resource cleanup
- Engine tests use JSON fixtures in `internal/engine/testdata/` — these fixtures are shared with the arbiter to ensure consistency
- Table-driven tests for transition validation and URL parsing

## Common Pitfalls

- **`AppendEvent` returns the DB-assigned ID.** Always use the returned event when referencing `event.ID` after insert. The in-memory event has `ID=0`.
- **`DeleteIssue` is a soft-delete.** Sets `status=deleted` and appends a delete event. Deleted issues are excluded from `list` and `next` unless `?all=true`.
- **`NextIssue` returns lowest priority number** (lower = higher priority). `ORDER BY priority ASC, created_at ASC` where `status='open' AND owner=''`.
- **Labels are JSON arrays in SQLite.** Stored as TEXT, marshaled/unmarshaled on read/write.
- **Event comments use `[boxofrocks]` prefix.** Parser expects this exact prefix. Human comments without it are ignored.
- **Metadata blocks use HTML comments.** `<!-- boxofrocks {"status":"open",...} -->` in issue bodies. Parser preserves surrounding human text.
- **Rate limiting is shared.** `SyncManager` holds shared rate limit state across all repos. Individual `RepoSyncer` goroutines check via `manager.checkRateLimit()`.
- **Trusted author filtering is silent.** When `TrustedAuthorsOnly=true`, comments from untrusted authors are skipped without error. The same `IsTrustedAuthor()` function is used in both the sync layer and the arbiter. The arbiter checks repo visibility via `GetRepo` since it has no local DB.

## Adding a New Event Action

1. Add the `Action` constant to `internal/model/event.go`
2. Add an `apply*` function in `internal/engine/engine.go`
3. Add the case to the `Apply()` switch
4. If it involves a new terminal status, update `IsTerminal()` in `internal/engine/rules.go`
5. Add a test case in `internal/engine/engine_test.go`
6. Add a fixture to `internal/engine/testdata/` if the scenario is complex
7. Wire the action into the appropriate handler in `internal/daemon/handlers.go`
8. The sync layer and arbiter will handle it automatically (they use the same engine)

## Adding a New CLI Command

1. Create `internal/cli/<command>.go` with `func run<Command>(args []string, gf globalFlags) error`
2. Add the case to the switch in `internal/cli/root.go` `Run()`
3. Add it to the `usage` string in `root.go`
4. Use `newClient(gf)` to get the daemon HTTP client
5. Use `resolveRepo(gf)` for repo resolution (CLI-side; daemon resolves via the 5-step chain above)
6. Use `printIssue()` / `printIssueList()` / `printJSON()` for output

## Adding a New REST Endpoint

1. Add the handler method to `internal/daemon/handlers.go`
2. Register the route in `internal/daemon/routes.go`
3. Add corresponding method to `internal/cli/client.go` `Client` struct
4. Add test(s) in `internal/daemon/handlers_test.go` using `testDaemon()` and `doRequest()`

## Configuration

Default config at `~/.boxofrocks/config.json`:
```json
{"listen_addr": ":8042", "data_dir": "~/.boxofrocks", "db_path": "~/.boxofrocks/bor.db"}
```

`TRACKER_HOST` env var overrides the daemon URL (default `http://127.0.0.1:8042`). Used for Docker containers pointing at `host.docker.internal`.

### Unix Domain Sockets

`bor init --socket` stores the repo's local path and enables a Unix domain socket at `.boxofrocks/bor.sock`. This allows sandbox agents to communicate with the daemon via `curl --unix-socket` without network access or binary installation.

**Repo resolution chain** (`resolveRepo` in `daemon/handlers.go`):
1. `?repo=` query param
2. `X-Repo` header
3. Socket association — `ConnContext` injects repo ID for Unix socket connections
4. `X-Working-Dir` header — CLI sends cwd automatically, daemon matches against `RepoConfig.LocalPath` (longest prefix)
5. Single-repo implicit fallback

**Socket lifecycle** (in `daemon/daemon.go`):
- `CreateSocketForRepo()` — creates `.boxofrocks/` dir, removes stale socket, listens, spawns `server.Serve(ln)` goroutine
- `startRepoSockets()` — called in `Run()` after PID file, iterates repos with `SocketEnabled=true`
- `cleanupSockets()` — removes socket files on shutdown
- `socketRepos map[string]int` — maps socket path to repo ID for `ConnContext` lookup

## Auth Chain

Token resolution order: `GITHUB_TOKEN` env → `gh auth token` → `git credential fill`. Daemon starts without a token (sync disabled, local-only mode).
