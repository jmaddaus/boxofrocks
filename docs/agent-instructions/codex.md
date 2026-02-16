## Task Management (bor)

This project uses `bor` for issue tracking. The daemon runs on the host. In the sandbox, set `TRACKER_HOST` first:

```bash
export TRACKER_HOST={{TRACKER_HOST}}        # e.g. http://host.docker.internal:8042
```

Before starting work, check for a queued issue:

```bash
bor next                                    # get next unassigned issue
bor assign <id> {{AGENT_NAME}}              # claim it
bor update <id> --status in_progress        # signal start
# ... do the work ...
bor comment <id> "description of progress"  # at milestones
bor close <id>                              # when done
```

If `bor next` returns nothing, proceed with the user's direct request. If the daemon is not reachable, verify `TRACKER_HOST` is correct. Do not create issues unless the user explicitly asks.

Other useful commands: `bor list [--status S]`, `bor create "title" [-p N] [-t TYPE] [-d D]`, `bor update <id> --status S [--comment C]`. All output is JSON. Statuses: `open`, `in_progress`, `blocked`, `in_review`, `closed`. Types: `task`, `bug`, `feature`, `epic`. Priority: integer, lower = higher.
