# Task Management

This project uses `bor` for issue tracking via a Unix socket.

```bash
SOCK=.boxofrocks/bor.sock
```

Before starting any work, check for a queued issue:

```bash
# Get next unassigned issue
curl -s --unix-socket $SOCK http://l/issues/next

# Claim it (replace <id> and {{AGENT_NAME}})
curl -s --unix-socket $SOCK -X POST \
  -d '{"owner":"{{AGENT_NAME}}"}' http://l/issues/<id>/assign

# Signal start
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"in_progress"}' http://l/issues/<id>

# ... do the work ...

# At milestones
curl -s --unix-socket $SOCK -X POST \
  -d '{"comment":"description of progress"}' http://l/issues/<id>/comment

# When done
curl -s --unix-socket $SOCK -X PATCH \
  -d '{"status":"closed"}' http://l/issues/<id>
```

If `curl` returns a connection error, the daemon may not be running on the host. If the next-issue response is a 404, proceed with the user's direct request. Do not create issues unless the user explicitly asks.

Other useful commands:
- List issues: `curl -s --unix-socket $SOCK http://l/issues`
- List by status: `curl -s --unix-socket $SOCK 'http://l/issues?status=open'`
- Create issue: `curl -s --unix-socket $SOCK -X POST -d '{"title":"..."}' http://l/issues`
- Update status: `curl -s --unix-socket $SOCK -X PATCH -d '{"status":"S","comment":"text"}' http://l/issues/<id>`

All responses are JSON. Statuses: `open`, `in_progress`, `blocked`, `in_review`, `closed`. Types: `task`, `bug`, `feature`, `epic`. Priority: integer, lower = higher.
