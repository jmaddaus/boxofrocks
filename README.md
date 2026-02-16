<!-- @format -->

# Box of Rocks (`bor`)
Inspired by having too many issues with async control, json sync and daemon-spawn running various agents in VMs/sandboxes and worktrees using beads.

A single machine daemon + CLI issue tracker backed by GitHub Issues. Issues are event-sourced: comments form an append-only event log, and a GitHub Action arbiter computes authoritative state. The daemon caches locally in SQLite for instant reads and manages bidirectional sync in the background. Daemon uses http to coordinate between any number of local agents/humans across multiple repos. Supports unix socket and file-based queue in repo to communicate with default docker sandbox images and other VMs. Launch simple web UI at localhost:8042.

## Grug First Start

### On Computer
Mac
```bash
brew install jmaddaus/tap/bor
```
Linux
```bash
curl -fsSL https://raw.githubusercontent.com/jmaddaus/boxofrocks/main/install.sh | sh\
```
Win
```powershell
scoop bucket add bor https://github.com/jmaddaus/scoop-bucket
scoop install bor
```

### In Repo Directory (also each worktree path being used)
bor init (—json for full sandbox, —socket if you need it)
* registers repo for use on this machine by agent or human
* creates .github/workflows/arbiter.yml (if missing)
* starts daemon (if not running)
* creates .boxofrocks/ (if using socket or queue)
* creates .boxofrocks/bor_api.sh reference (if using json method for agent reference)
* updates .gitignore with .boxofrocks/ (if missing)

### (First Time for Repo Only) On GitHub
* enable Issues if disabled
* push arbiter and updated gitignore to your repo, pull into main
		
### In Agent File (e.g. agents.md, claude.md)
* copy/paste md instructions for your agents into file from docs/agent-instructions (native = agent runs on machine so uses CLI, json = full sandbox agent so uses json queue, socket = linux shared kernel VM option)
* edit {{AGENT_NAME}} in stub with your agent name/id to use

### Success!
* arbiter.yml will pull binary/checksum to run GH Actions to adjudicate conflicts between updates in issues
* issues created by (approved) humans in GH will need to have label boxofrocks added to the issue to get flagged for inclusion into work chain

### (Optional) In Web Browser
Visit localhost:8042 for basic UI to see local issues info

### (Optional) In Term
```
bor help
```

## Quick Start

```bash
# Build
go build -o bor ./cmd/bor

# Initialize a repo (auto-starts daemon in background)
bor init
#bor init --socket if you need unix socket / file queue for VM comms Linux -> Linux.
#bor init --json if you need json path for sandbox comms Linux -> Win/Mac/Linux.

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

### Homebrew (macOS/Linux)

```bash
brew install jmaddaus/tap/bor
```

### Scoop (Windows)

```powershell
scoop bucket add bor https://github.com/jmaddaus/scoop-bucket
scoop install bor
```

### Shell script (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/jmaddaus/boxofrocks/main/install.sh | sh
```

Set `BOR_INSTALL_DIR` to change the install location (default `/usr/local/bin`).

### Go install

Requires Go 1.22+.

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
```

### Build from source

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

| Flag                | Description             | Default                                            |
| ------------------- | ----------------------- | -------------------------------------------------- |
| `--host URL`        | Daemon URL              | `$TRACKER_HOST` or `http://127.0.0.1:8042`         |
| `-r`, `--repo NAME` | Repository `owner/name` | Auto-detected from git remote or working directory |
| `--pretty`          | Human-readable output   | JSON output                                        |

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

Initialize a repository. Auto-starts the daemon if not running, checks auth, registers the repo, and triggers initial sync. Use `--socket` to enable a Unix domain socket at `.boxofrocks/bor.sock` and a file-based queue at `.boxofrocks/queue/` for sandbox agent access. Use `--offline` to skip sync.

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

When the agent runs inside a container or sandbox, the daemon on the host is not reachable at `localhost`. Three options:

**File queue (recommended for Docker Desktop):** Initialize with `--socket` and agents use a shell function that writes JSON request files to `.boxofrocks/queue/`. Works across the macOS/Windows Docker Desktop VM boundary where Unix sockets cannot:

```bash
# On host:
bor init --socket

# In sandbox — use the bor_api shell function from the agent templates:
bor_api GET /issues/next
bor_api POST /issues/1/assign '{"owner":"claude"}'
```

**Unix socket:** Also enabled by `--socket`. Agents use `curl` over the mounted socket — works on Linux hosts where sockets cross the container boundary:

```bash
curl -s --unix-socket .boxofrocks/bor.sock http://l/issues/next
```

See [docs/agent-instructions/](docs/agent-instructions/) for drop-in templates with three variants: `-native` (bor CLI), `-socket` (curl), and `-json` (file queue).

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
