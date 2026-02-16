# Agent Instruction Templates

Drop-in instruction files that teach AI coding agents to use `bor` for task management.

## Install bor

```bash
go install github.com/jmaddaus/boxofrocks/cmd/bor@latest
bor daemon start
bor init owner/repo          # one-time per repository
```

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
| `{{TRACKER_HOST}}` | Daemon URL (only needed for sandboxes/containers) | `http://host.docker.internal:8042` |

## Docker / Sandbox Environments

When the agent runs inside a container or sandbox, the daemon on the host is not reachable at `localhost`. Set the `TRACKER_HOST` environment variable:

```bash
export TRACKER_HOST=http://host.docker.internal:8042
```
