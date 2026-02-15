# Box of Rocks (`bor`)

A daemon + CLI issue tracker backed by GitHub Issues. Issues are event-sourced: comments form an append-only event log, and a GitHub Action arbiter computes authoritative state. The daemon caches locally in SQLite for instant reads and manages bidirectional sync in the background.

## Quick Start

```bash
# Build
go build -o bor ./cmd/bor

# Start the daemon (runs in foreground)
bor daemon start

# In another terminal: initialize a repo
bor init --repo owner/name

# Create and manage issues
bor create "Fix login bug" -p 1 -t bug -d "Users can't log in with SSO"
bor list
bor next
bor update 1 --status in_progress
bor assign 1 agent-1
bor close 1
bor list --all
```

## Installation

Requires Go 1.22+.

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
```

Or build from source:

```bash
git clone https://github.com/jmaddaus/boxofrocks.git
cd boxofrocks
go build -o bor ./cmd/bor
```

## Architecture

```
CLI (bor) ──HTTP──> Daemon ──> SQLite (local cache)
                     │
                     └──sync──> GitHub Issues (remote store)
                                    │
                          GitHub Action (arbiter)
```

- **Daemon** runs in the foreground, serves a REST API on `127.0.0.1:8042`, and syncs with GitHub in the background.
- **CLI** talks to the daemon over HTTP. All commands work instantly from local cache.
- **Events** are appended as GitHub Issue comments prefixed with `[boxofrocks]`. State is derived by replaying events.
- **Arbiter** is a GitHub Action that triggers on new comments, replays events, and writes authoritative state into the issue body.

## Configuration

Config is stored at `~/.boxofrocks/config.json`:

```json
{
  "listen_addr": ":8042",
  "data_dir": "~/.boxofrocks",
  "db_path": "~/.boxofrocks/bor.db"
}
```

## Authentication

The daemon resolves a GitHub token using three methods (in order):

1. `GITHUB_TOKEN` environment variable
2. `gh auth token` (GitHub CLI)
3. `git credential fill` (git credential helper)

If no token is found, the daemon starts but sync is disabled. Issues can still be created and managed locally.

## CLI Reference

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--host URL` | Daemon URL | `$TRACKER_HOST` or `http://127.0.0.1:8042` |
| `--repo NAME` | Repository `owner/name` | Auto-detected from git remote |
| `--pretty` | Human-readable output | JSON output |

### Commands

#### `bor daemon start`

Start the daemon in the foreground. Use systemd, launchd, or `nohup` for background operation.

#### `bor daemon status`

Check if the daemon is running and show sync status.

#### `bor init --repo owner/name [--offline]`

Register a repository and trigger initial sync. Use `--offline` to skip GitHub validation and start with an empty state.

#### `bor create "title" [-p priority] [-t type] [-d description]`

Create an issue. Priority is numeric (lower = higher priority, default 0). Type is `task`, `bug`, or `feature`.

#### `bor list [--all] [--status S] [--priority N]`

List issues. By default, deleted issues are hidden. Use `--all` to include them.

#### `bor next`

Get the highest-priority open unassigned issue.

#### `bor update <id> [--status S] [--priority N] [--title T] [--description D]`

Update issue fields. Status can be `open`, `in_progress`, or `closed`.

#### `bor close <id>`

Close an issue (shorthand for `bor update <id> --status closed`).

#### `bor assign <id> <owner>`

Assign an issue to an owner.

## Multi-Repo Support

The daemon manages multiple repositories on one machine. When multiple repos are registered, specify which repo to target:

```bash
bor --repo owner/repo1 list
bor --repo owner/repo2 create "New issue"
```

If only one repo is registered, it is used implicitly.

## Docker / Sandbox Usage

When running CLI commands from inside a Docker container while the daemon runs on the host:

```bash
export TRACKER_HOST=http://host.docker.internal:8042
bor list
```

## Offline Operation

After initial setup, the daemon works without network connectivity:

- Issues can be created, updated, and closed locally
- Changes are stored as pending events
- When connectivity returns, pending events sync automatically on the next cycle

For first-time setup without connectivity:

```bash
bor init --repo owner/name --offline
```

## Event Model

Issues are event-sourced. Each mutation appends an event comment to the GitHub Issue:

```
[boxofrocks] {"timestamp":"2024-01-15T10:30:00Z","action":"status_change","payload":{"status":"in_progress"}}
```

**Event types:** `create`, `status_change`, `assign`, `close`, `update`, `delete`, `reopen`

**State transitions:**
- `open` -> `in_progress`, `closed`
- `in_progress` -> `open`, `closed`
- `closed` -> `open` (reopen)
- Invalid transitions are silently ignored during replay

## Arbiter (GitHub Action)

The arbiter ensures authoritative state by replaying events server-side. See [arbiter/README.md](arbiter/README.md) for setup instructions.

## Development

```bash
# Build
go build ./...

# Run tests
go test ./...

# Format
go fmt ./...

# Vet
go vet ./...
```

### Project Structure

```
cmd/bor/                   Entry point
internal/
  model/                   Issue, Event, RepoConfig types
  store/                   SQLite storage layer
  engine/                  Event replay engine (pure logic)
  github/                  GitHub REST API client, auth, parser
  sync/                    Bidirectional sync manager
  daemon/                  HTTP server and REST handlers
  cli/                     CLI commands and daemon client
  config/                  Configuration management
arbiter/                   GitHub Action for server-side reconciliation
```

## Dependencies

- `modernc.org/sqlite` - Pure-Go SQLite (no CGO required)
- Go stdlib for everything else
