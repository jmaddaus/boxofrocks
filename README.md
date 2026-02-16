# Box of Rocks (`bor`)

A daemon + CLI issue tracker backed by GitHub Issues. Issues are event-sourced: comments form an append-only event log, and a GitHub Action arbiter computes authoritative state. The daemon caches locally in SQLite for instant reads and manages bidirectional sync in the background.

## Quick Start

```bash
# Build
go build -o bor ./cmd/bor

# Initialize a repo (auto-starts daemon in background)
bor init --socket

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

- **Daemon** serves a REST API on `127.0.0.1:8042` and syncs with GitHub in the background. Runs in background by default.
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

The daemon resolves a GitHub token using four methods (in order):

1. `GITHUB_TOKEN` environment variable
2. `~/.boxofrocks/token` file (managed by `bor login`)
3. `gh auth token` (GitHub CLI)
4. `git credential fill` (git credential helper — works automatically with VS Code/GCM)

If no token is found, the daemon starts but sync is disabled. Issues can still be created and managed locally.

### Managing Tokens

```bash
bor login                  # Enter token interactively
bor login --token ghp_...  # Provide token directly
bor login --status         # Show which auth methods are available
bor logout                 # Remove stored token
```

## CLI Reference

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--host URL` | Daemon URL | `$TRACKER_HOST` or `http://127.0.0.1:8042` |
| `-r`, `--repo NAME` | Repository `owner/name` | Auto-detected from git remote or working directory |
| `--pretty` | Human-readable output | JSON output |

### Commands

#### `bor daemon start [--foreground]`

Start the daemon in the background (default). Use `--foreground` to run in the foreground for debugging.

#### `bor daemon stop`

Stop the running daemon.

#### `bor daemon status`

Check if the daemon is running and show sync status.

#### `bor daemon logs [-f] [-n N]`

View daemon logs. Use `-f` to follow output, `-n` to set number of lines (default 20).

#### `bor login [--token TOK] [--status]`

Authenticate with GitHub. Validates the token and saves it to `~/.boxofrocks/token`. Use `--status` to check current auth.

#### `bor logout`

Remove the stored GitHub token.

#### `bor init [--repo owner/name] [--socket] [--offline]`

Initialize a repository. Auto-starts the daemon if not running, checks auth, registers the repo, and triggers initial sync. Use `--socket` to enable a Unix domain socket at `.boxofrocks/bor.sock` for sandbox agent access. Use `--offline` to skip sync.

#### `bor create "title" [-p priority] [-t type] [-d description]`

Create an issue. Priority is numeric (lower = higher priority, default 0). Type is `task`, `bug`, `feature`, or `epic`.

#### `bor list [--all] [--status S] [--priority N]`

List issues. By default, deleted issues are hidden. Use `--all` to include them.

#### `bor next`

Get the highest-priority open unassigned issue.

#### `bor update <id> [--status S] [--priority N] [--title T] [--description D]`

Update issue fields. Status can be `open`, `in_progress`, `blocked`, `in_review`, or `closed`.

#### `bor close <id>`

Close an issue (shorthand for `bor update <id> --status closed`).

#### `bor assign <id> <owner>`

Assign an issue to an owner.

#### `bor comment <id> "text"`

Add a comment to an issue.

#### `bor config trusted-authors-only <true|false>`

Toggle trusted author filtering for a repo. When enabled, inbound sync only applies GitHub comments from trusted authors (OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR). Comments from untrusted users (NONE, FIRST_TIMER, FIRST_TIME_CONTRIBUTOR) are silently skipped.

This is auto-enabled for public repos during `bor init`. Use `-r` to target a specific repo.

## Multi-Repo Support

The daemon manages multiple repositories on one machine. Repo resolution uses this priority chain:

1. **Explicit flag:** `-r owner/name` or `--repo owner/name`
2. **Working directory:** CLI sends cwd automatically; daemon matches against registered repos' local paths
3. **Git remote:** auto-detected from `git remote get-url origin`
4. **Single-repo fallback:** if only one repo is registered, it is used implicitly

In practice, if you initialized with `bor init --socket` (which stores the local path), running any `bor` command from inside that repo directory just works:

```bash
cd ~/projects/repo1
bor list                    # resolves via working directory

bor -r owner/repo2 list    # explicit override
```

## Docker / Sandbox Usage

When the agent runs inside a container or sandbox, the daemon on the host is not reachable at `localhost`. Two options:

**Unix socket (recommended):** Initialize with `--socket` and agents use `curl` over the mounted socket — no network access or binary installation required:

```bash
# On host:
bor init --socket

# In sandbox (repo directory is mounted):
curl -s --unix-socket .boxofrocks/bor.sock http://l/issues/next
```

See [docs/agent-instructions/](docs/agent-instructions/) for drop-in templates.

**TCP via TRACKER_HOST (legacy):** Set the `TRACKER_HOST` environment variable:

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

## Trusted Author Filtering

On public repositories, anyone can comment on issues. Since bor parses `[boxofrocks]`-formatted comments as events, this could allow untrusted users to inject status changes. To prevent this, bor filters inbound comments by GitHub's `author_association` field.

- **Auto-enabled** for public repos during `bor init`
- **Off by default** for private repos
- **Toggle per repo:** `bor config trusted-authors-only true/false`
- **Trusted associations:** OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR
- **Untrusted (filtered):** FIRST_TIMER, FIRST_TIME_CONTRIBUTOR, NONE

Both the daemon sync layer and the arbiter GitHub Action apply this filter.

## Event Model

Issues are event-sourced. Each mutation appends an event comment to the GitHub Issue:

```
[boxofrocks] {"timestamp":"2024-01-15T10:30:00Z","action":"status_change","payload":{"status":"in_progress"}}
```

**Event types:** `create`, `status_change`, `assign`, `close`, `update`, `delete`, `reopen`

**From-status validation:** Status change events include a `from_status` field declaring the expected current state. If the actual current state doesn't match, the event is skipped (stale). Events without `from_status` (legacy) are always accepted. The `deleted` status is terminal — no further status changes are allowed.

**Statuses:** `open`, `in_progress`, `blocked`, `in_review`, `closed`, `deleted`
**Issue types:** `task`, `bug`, `feature`, `epic`

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
