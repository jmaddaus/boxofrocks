# Box of Rocks Reconciler (Arbiter)

The arbiter is a GitHub Action that reconciles event-sourced state on GitHub Issues. It reads all boxofrocks event comments on an issue, replays them through the state engine, and updates the issue body metadata to reflect the current derived state.

## How It Works

1. Fetches all comments on the specified GitHub issue
2. Filters comments to those prefixed with `[boxofrocks]`
3. Parses each matching comment into a structured event
4. Replays all events through the state engine to derive current issue state
5. Updates the issue body with the reconciled metadata (status, priority, owner, labels, issue type)

Human-written text in the issue body is preserved; only the hidden metadata comment block is updated.

## Installation

Add a workflow file to your repository at `.github/workflows/boxofrocks-reconcile.yml`:

```yaml
name: Box of Rocks Reconciler
on:
  issue_comment:
    types: [created]
jobs:
  reconcile:
    if: startsWith(github.event.comment.body, '[boxofrocks]')
    runs-on: ubuntu-latest
    steps:
      - uses: jmaddaus/boxofrocks/arbiter@main
        with:
          issue-number: ${{ github.event.issue.number }}
```

This workflow triggers whenever a new comment is created on an issue. It only runs the reconciliation if the comment starts with the `[boxofrocks]` prefix.

## Building from Source

```bash
cd arbiter/cmd/reconcile
go build -o reconcile .
```

Cross-compile for Linux (used in GitHub Actions):

```bash
GOOS=linux GOARCH=amd64 go build -o reconcile-linux-amd64 ./arbiter/cmd/reconcile
```

## Environment Variables

| Variable             | Description                                      | Required |
|----------------------|--------------------------------------------------|----------|
| `GITHUB_TOKEN`       | GitHub API token with issue read/write permission | Yes      |
| `GITHUB_REPOSITORY`  | Repository in `owner/repo` format                | Yes      |
| `ISSUE_NUMBER`       | The issue number to reconcile                    | Yes      |

`GITHUB_TOKEN` and `GITHUB_REPOSITORY` are automatically provided by GitHub Actions. `ISSUE_NUMBER` is passed via the action input.
