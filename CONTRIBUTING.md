# Contributing to Box of Rocks

## Prerequisites

- **Go 1.22+** — [install](https://go.dev/dl/)
- **Git** — with a GitHub remote configured
- **GitHub token** — for syncing issues (optional for local-only use)

## Quick Setup

From the root of a repo that uses Box of Rocks:

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
bor init
```

`bor init` handles everything automatically:
1. Starts the daemon in the background (if not already running)
2. Checks for GitHub authentication
3. Registers the repository
4. Triggers an initial sync

## Authentication

Box of Rocks tries four methods in order:

| Priority | Method | Setup |
|----------|--------|-------|
| 1 | `GITHUB_TOKEN` env var | `export GITHUB_TOKEN=ghp_...` |
| 2 | `~/.boxofrocks/token` file | `bor login` |
| 3 | GitHub CLI | `gh auth login` |
| 4 | Git credential manager | Automatic if using VS Code or GCM |

Most developers using VS Code with GitHub won't need to do anything — the git credential manager provides tokens automatically.

If none of the implicit methods work:

```bash
bor login --token ghp_your_token_here
# or interactively:
bor login
```

Check current auth status:

```bash
bor login --status
```

Remove stored token:

```bash
bor logout
```

## Daemon Management

The daemon runs in the background by default:

```bash
bor daemon start          # Start in background
bor daemon start --foreground  # Run in foreground (for debugging)
bor daemon status         # Check if running
bor daemon stop           # Stop the daemon
bor daemon logs           # View recent logs
bor daemon logs -f        # Follow log output
bor daemon logs -n 50     # Show last 50 lines
```

The daemon stores its data in `~/.boxofrocks/`:
- `bor.db` — SQLite database
- `daemon.pid` — PID file (background mode)
- `daemon.log` — Log output (background mode)
- `token` — Stored GitHub token (`bor login`)
- `config.json` — Configuration overrides

## Common Commands

```bash
bor list                  # List all open issues
bor list --status closed  # List closed issues
bor create --title "Fix bug" --type bug
bor next                  # Get the next issue to work on
bor update 42 --status in_progress
bor assign 42 --owner @me
bor close 42
```

Use `--pretty` for human-readable output, or pipe JSON (default) to `jq`.

## Development Workflow

```bash
# Build
go build ./...

# Run tests
go test ./...

# Run a specific test
go test -run TestReplay ./internal/engine/

# Static analysis
go vet ./...

# Format
go fmt ./...
```

## Troubleshooting

**"daemon not running"** — Run `bor daemon start` or let `bor init` start it automatically.

**"unable to resolve GitHub token"** — Run `bor login --status` to diagnose. Use `bor login` to set a token.

**"port :8042 already in use"** — Another daemon instance is running. Use `bor daemon stop` first, or check with `bor daemon status`.

**Sync not working** — Check `bor daemon logs` for errors. Verify auth with `bor login --status`.
