# Task Management (bor)

This project uses `bor` for issue tracking. The daemon must be running (`bor daemon start`).

Before starting work, check for a queued issue:

```bash
bor next
bor assign <id> {{AGENT_NAME}}
bor update <id> --status in_progress --comment "starting: brief plan"

# At milestones — comment on progress
bor comment <id> "implemented X, moving to Y"

# If blocked
bor update <id> --status blocked --comment "reason"

# When ready for review
bor update <id> --status in_review --comment "summary of changes"

# When done
bor update <id> --status closed --comment "what was done"
```

If `bor next` returns "no issues available", proceed with the user's direct request. Do not create issues unless explicitly asked.

Reference:
- `bor list --status open` — list issues by status
- `bor create "title" -p 1 -t task` — create issue (priority: lower = higher)
- `bor update <id> --title "..." --priority 2` — update fields

Statuses: `open` `in_progress` `blocked` `in_review` `closed`. Types: `task` `bug` `feature` `epic`.
