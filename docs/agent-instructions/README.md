# Agent Instruction Templates

Drop-in instruction files that teach AI coding agents to use `bor` for task management. Pick the variant that matches how the agent communicates with the daemon, then copy it into your project's agent instruction file.

## Variants

| Variant | File | Communication | Best for |
|---------|------|---------------|----------|
| **Native** | [native.md](native.md) | `bor` CLI binary | Agents with `bor` installed (e.g. host-local agents) |
| **Socket** | [socket.md](socket.md) | `curl --unix-socket` | Sandboxed agents where Unix sockets work (Linux containers on Linux hosts) |
| **JSON** | [json.md](json.md) | File-based queue (`bor_api` shell function) | Docker Desktop sandboxes where sockets don't cross the VM boundary |

## Setup

On the host machine:

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
bor daemon start
cd /path/to/your/repo
bor init --repo owner/name --socket    # creates .boxofrocks/bor.sock + .boxofrocks/queue/ + bor_api.sh
```

The `--socket` flag enables the Unix domain socket and the file-based queue. For native mode, agents connect over TCP — no `--socket` flag needed.

## Copy Destinations

| Platform | Copy template to |
|----------|-----------------|
| Claude Code | Append to project `CLAUDE.md` |
| Cursor | Save as `.cursorrules` |
| GitHub Copilot | Save as `.github/copilot-instructions.md` |
| OpenAI Codex | Save as or append to `AGENTS.md` |

## Placeholders

Replace these in the template after copying:

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{{AGENT_NAME}}` | Name the agent uses to claim issues | `claude`, `cursor`, `copilot` |

## Architecture

```
Native:   bor CLI ──TCP──> daemon ──> SQLite
Socket:   curl --unix-socket ──> daemon (same handlers)
JSON:     bor_api() ──write .req──> queue/ ──daemon polls──> same handlers
```

All three variants dispatch through the same HTTP handler chain. The daemon treats file queue requests identically to TCP and socket requests.

### How the file queue works

When `bor init --socket` (or `--json`) is run, the daemon generates `.boxofrocks/bor_api.sh` — a shell function that agents source. It:
1. Writes a JSON request to `.boxofrocks/queue/{id}.req` (atomic rename)
2. Polls for `.boxofrocks/queue/{id}.resp` (up to 30 seconds)
3. Prints the response and cleans up

The daemon polls the queue directory every 100ms, dispatches requests through `ServeHTTP()`, and writes responses atomically.
