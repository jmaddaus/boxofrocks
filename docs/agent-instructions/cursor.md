# Task Management

This project uses `bor` for issue tracking. Before starting any work, check for a queued issue:

```bash
bor next                                    # get next unassigned issue
bor assign <id> {{AGENT_NAME}}              # claim it
bor update <id> --status in_progress        # signal start
# ... do the work ...
bor comment <id> "description of progress"  # at milestones
bor close <id>                              # when done
```

If `bor next` returns nothing, proceed with the user's direct request. If the daemon is not running, start it with `bor daemon start`. Do not create issues unless the user explicitly asks.

Other useful commands: `bor list [--status S]`, `bor create "title" [-p N] [-t TYPE] [-d D]`, `bor update <id> --status S [--comment C]`. All output is JSON. Statuses: `open`, `in_progress`, `blocked`, `in_review`, `closed`. Types: `task`, `bug`, `feature`, `epic`. Priority: integer, lower = higher.

In containerized environments, set `TRACKER_HOST` to reach the host daemon (e.g. `export TRACKER_HOST=http://host.docker.internal:8042`).
