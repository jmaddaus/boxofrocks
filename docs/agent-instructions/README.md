# Agent Instruction Templates

Drop-in instruction files that teach AI coding agents to use `bor` for task management via Unix domain sockets. No binary installation needed in the sandbox — agents use `curl --unix-socket` to talk to the daemon on the host.

## Setup

On the host machine:

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
bor daemon start
cd /path/to/your/repo
bor init --repo owner/name --socket    # creates .boxofrocks/bor.sock
```

The `--socket` flag tells the daemon to listen on a Unix domain socket at `.boxofrocks/bor.sock` inside the repo directory. Since sandboxes automatically mount the repo, the socket is available to agents with zero extra config.

## Templates

| Platform | Template | Copy to |
|----------|----------|---------|
| Claude Code | [claude-code.md](claude-code.md) | Append to project `CLAUDE.md` |
| Cursor | [cursor.md](cursor.md) | Save as `.cursorrules` |
| GitHub Copilot | [github-copilot.md](github-copilot.md) | Save as `.github/copilot-instructions.md` |
| OpenAI Codex | [codex.md](codex.md) | Save as or append to `AGENTS.md` |

## Placeholders

Replace these in the template after copying:

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{{AGENT_NAME}}` | Name the agent uses to claim issues | `claude`, `cursor`, `copilot` |
| `{{REPO}}` | GitHub repo in `owner/name` format | `acme/webapp` |

## How It Works

The daemon on the host listens on both TCP (`:8042`) and a per-repo Unix domain socket (`.boxofrocks/bor.sock`). Users interact via `bor` CLI commands over TCP. Sandbox agents use `curl --unix-socket` over the mounted socket file — no network access or binary installation required.

```
Host:     bor CLI ──TCP──> daemon ──> SQLite
Sandbox:  curl --unix-socket .boxofrocks/bor.sock ──> daemon (same)
```
