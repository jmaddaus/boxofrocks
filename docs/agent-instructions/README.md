# Agent Instruction Templates

Drop-in instruction files that teach AI coding agents to use `bor` for task management. Each platform has three variants depending on how the agent communicates with the daemon.

## Variants

| Variant | Suffix | Communication | Best for |
|---------|--------|---------------|----------|
| **Native** | `-native` | `bor` CLI binary | Agents with `bor` installed (e.g. host-local agents) |
| **Socket** | `-socket` | `curl --unix-socket` | Agents in sandboxes where Unix sockets work (Linux containers on Linux hosts) |
| **JSON** | `-json` | File-based queue (`bor_api` shell function) | Docker Desktop sandboxes where sockets don't cross the VM boundary |

## Setup

On the host machine:

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
bor daemon start
cd /path/to/your/repo
bor init --repo owner/name --socket    # creates .boxofrocks/bor.sock + .boxofrocks/queue/
```

The `--socket` flag enables both the Unix domain socket (`.boxofrocks/bor.sock`) and the file-based queue (`.boxofrocks/queue/`). For native mode, agents connect to the daemon over TCP — no `--socket` flag needed.

## Templates

### Native (`-native`) — agent has `bor` CLI installed

| Platform | Template | Copy to |
|----------|----------|---------|
| Claude Code | [claude-code-native.md](claude-code-native.md) | Append to project `CLAUDE.md` |
| Cursor | [cursor-native.md](cursor-native.md) | Save as `.cursorrules` |
| GitHub Copilot | [github-copilot-native.md](github-copilot-native.md) | Save as `.github/copilot-instructions.md` |
| OpenAI Codex | [codex-native.md](codex-native.md) | Save as or append to `AGENTS.md` |

### Socket (`-socket`) — agent uses `curl --unix-socket`

| Platform | Template | Copy to |
|----------|----------|---------|
| Claude Code | [claude-code-socket.md](claude-code-socket.md) | Append to project `CLAUDE.md` |
| Cursor | [cursor-socket.md](cursor-socket.md) | Save as `.cursorrules` |
| GitHub Copilot | [github-copilot-socket.md](github-copilot-socket.md) | Save as `.github/copilot-instructions.md` |
| OpenAI Codex | [codex-socket.md](codex-socket.md) | Save as or append to `AGENTS.md` |

### JSON (`-json`) — agent uses file-based queue

| Platform | Template | Copy to |
|----------|----------|---------|
| Claude Code | [claude-code-json.md](claude-code-json.md) | Append to project `CLAUDE.md` |
| Cursor | [cursor-json.md](cursor-json.md) | Save as `.cursorrules` |
| GitHub Copilot | [github-copilot-json.md](github-copilot-json.md) | Save as `.github/copilot-instructions.md` |
| OpenAI Codex | [codex-json.md](codex-json.md) | Save as or append to `AGENTS.md` |

## Which variant should I use?

- **Native**: The agent can run `bor` commands directly. Simplest instructions, cleanest output. Requires `bor` to be installed in the agent's environment.
- **Socket**: The agent runs in a sandbox that mounts the repo directory, and Unix sockets work across the boundary (e.g. Linux container on a Linux host). No binary installation needed — just `curl`.
- **JSON**: The agent runs in a Docker Desktop sandbox on macOS/Windows where Unix sockets don't cross the VM boundary, and network access is blocked. The file-based queue uses the mounted filesystem as the communication channel.

## Placeholders

Replace these in the template after copying:

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{{AGENT_NAME}}` | Name the agent uses to claim issues | `claude`, `cursor`, `copilot` |
| `{{REPO}}` | GitHub repo in `owner/name` format | `acme/webapp` |

## Architecture

```
Native:   bor CLI ──TCP──> daemon ──> SQLite
Socket:   curl --unix-socket ──> daemon (same handlers)
JSON:     bor_api() ──write .req──> queue/ ──daemon polls──> same handlers
```

All three variants dispatch through the same HTTP handler chain. The daemon treats file queue requests identically to TCP and socket requests — same middleware, same repo resolution, same event sourcing.

### How the file queue works

The `-json` templates include a `bor_api` shell function that:
1. Generates a unique request ID from timestamp + PID
2. Writes a JSON request to `.boxofrocks/queue/{id}.req` (via `.tmp` + rename for atomicity)
3. Polls for `.boxofrocks/queue/{id}.resp` (up to 30 seconds)
4. Reads and prints the response, then cleans up both files

The daemon polls the queue directory every 100ms, reads `.req` files, builds synthetic `http.Request` objects, dispatches them through `ServeHTTP()`, and writes `.resp` files atomically.
