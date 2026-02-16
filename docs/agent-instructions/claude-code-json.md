## Task Management (bor)

This project uses `bor` for issue tracking via a file-based queue.

**Setup** — paste this function at the start of your session:

```bash
BOR_QUEUE=.boxofrocks/queue

bor_api() {
  local method="$1" path="$2" body="${3:-null}"
  local id
  id="$(date +%s%N)$$"
  local req="$BOR_QUEUE/${id}.req"
  local resp="$BOR_QUEUE/${id}.resp"

  mkdir -p "$BOR_QUEUE"

  printf '{"method":"%s","path":"%s","body":%s}\n' \
    "$method" "$path" "$body" > "${req}.tmp"
  mv "${req}.tmp" "$req"

  local i=0
  while [ ! -f "$resp" ] && [ $i -lt 300 ]; do
    sleep 0.1
    i=$((i + 1))
  done

  if [ -f "$resp" ]; then
    cat "$resp"
    rm -f "$req" "$resp"
  else
    echo '{"error":"timeout waiting for daemon response"}'
    rm -f "$req"
    return 1
  fi
}
```

Before starting work, check for a queued issue:

```bash
# Get next unassigned issue
bor_api GET /issues/next

# Claim it (replace <id> and {{AGENT_NAME}})
bor_api POST /issues/<id>/assign '{"owner":"{{AGENT_NAME}}"}'

# Signal start
bor_api PATCH /issues/<id> '{"status":"in_progress"}'

# ... do the work ...

# At milestones
bor_api POST /issues/<id>/comment '{"comment":"description of progress"}'

# When done
bor_api PATCH /issues/<id> '{"status":"closed"}'
```

If `bor_api` returns a timeout error, the daemon may not be running on the host. If the next-issue response is a 404, proceed with the user's direct request. Do not create issues unless the user explicitly asks.

Other useful commands:
- List issues: `bor_api GET /issues`
- List by status: `bor_api GET '/issues?status=open'`
- Create issue: `bor_api POST /issues '{"title":"..."}'`
- Update status: `bor_api PATCH /issues/<id> '{"status":"S","comment":"text"}'`

All responses are JSON with `{"status":<http_code>,"body":<response>}` envelope. Statuses: `open`, `in_progress`, `blocked`, `in_review`, `closed`. Types: `task`, `bug`, `feature`, `epic`. Priority: integer, lower = higher.
