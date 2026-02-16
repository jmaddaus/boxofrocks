# Task Management

This project uses `bor` for issue tracking. The `bor` CLI must be installed and the daemon running.

Before starting work, check for a queued issue:

```bash
# Get next unassigned issue
bor next

# Claim it (replace <id> and {{AGENT_NAME}})
bor assign <id> {{AGENT_NAME}}

# Signal start
bor update <id> --status in_progress

# ... do the work ...

# At milestones
bor comment <id> "description of progress"

# When done
bor close <id>
```

If `bor` returns a connection error, the daemon may not be running. Start it with `bor daemon start`. If `bor next` returns "no issues available", proceed with the user's direct request. Do not create issues unless the user explicitly asks.

Other useful commands:
- List issues: `bor list`
- List by status: `bor list --status open`
- Create issue: `bor create "title" -p 1 -t task`
- Update fields: `bor update <id> --title "new title" --priority 2`
- Add comment with status: `bor update <id> --status in_review --comment "ready for review"`

Statuses: `open`, `in_progress`, `blocked`, `in_review`, `closed`. Types: `task`, `bug`, `feature`, `epic`. Priority: integer, lower = higher.
