# Task Management

This project uses `bor` for issue tracking via a Unix socket.

```bash
SOCK=.boxofrocks/bor.sock
```

Before starting any work, check for a queued issue:

```bash
curl -s --unix-socket $SOCK http://l/issues/next
curl -s --unix-socket $SOCK -X POST \
  -d '{"owner":"{{AGENT_NAME}}"}' http://l/issues/<id>/assign
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"in_progress","comment":"starting: brief plan"}' http://l/issues/<id>

# At milestones — comment on progress
curl -s --unix-socket $SOCK -X POST \
  -d '{"comment":"implemented X, moving to Y"}' http://l/issues/<id>/comment

# If blocked
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"blocked","comment":"reason"}' http://l/issues/<id>

# When ready for review
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"in_review","comment":"summary of changes"}' http://l/issues/<id>

# When done
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"closed","comment":"what was done"}' http://l/issues/<id>
```

If `curl` returns a connection error, the daemon may not be running. If next-issue returns 404, proceed with the user's direct request. Do not create issues unless explicitly asked.

Reference:
- List by status: `curl -s --unix-socket $SOCK 'http://l/issues?status=open'`
- Create issue: `curl -s --unix-socket $SOCK -X POST -d '{"title":"..."}' http://l/issues`

All responses are JSON. Statuses: `open` `in_progress` `blocked` `in_review` `closed`. Types: `task` `bug` `feature` `epic`. Priority: integer, lower = higher.
