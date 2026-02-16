# Task Management (bor)

This project uses `bor` for issue tracking via a file-based queue.

**Setup** — source the helper at the start of your session:

```bash
source .boxofrocks/bor_api.sh
```

Before starting work, check for a queued issue:

```bash
bor_api GET /issues/next
bor_api POST /issues/<id>/assign '{"owner":"{{AGENT_NAME}}"}'
bor_api PATCH /issues/<id> '{"status":"in_progress","comment":"starting: brief plan"}'

# At milestones — comment on progress
bor_api POST /issues/<id>/comment '{"comment":"implemented X, moving to Y"}'

# If blocked
bor_api PATCH /issues/<id> '{"status":"blocked","comment":"reason"}'

# When ready for review
bor_api PATCH /issues/<id> '{"status":"in_review","comment":"summary of changes"}'

# When done
bor_api PATCH /issues/<id> '{"status":"closed","comment":"what was done"}'
```

If `bor_api` returns a timeout, the daemon may not be running. If next-issue returns 404, proceed with the user's direct request. Do not create issues unless explicitly asked.

Reference:
- List by status: `bor_api GET '/issues?status=open'`
- Create issue: `bor_api POST /issues '{"title":"..."}'`

Responses: `{"status":<http_code>,"body":<response>}`. Statuses: `open` `in_progress` `blocked` `in_review` `closed`. Types: `task` `bug` `feature` `epic`. Priority: integer, lower = higher.
