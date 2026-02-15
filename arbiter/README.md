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
  schedule:
    - cron: '*/15 * * * *'
  workflow_dispatch:
jobs:
  reconcile:
    runs-on: ubuntu-latest
    steps:
      - name: Download reconcile binary
        run: |
          curl -sL https://github.com/jmaddaus/boxofrocks/releases/latest/download/reconcile-linux-amd64 -o /tmp/reconcile
          chmod +x /tmp/reconcile
      - name: Reconcile all boxofrocks issues
        env:
          GH_TOKEN: ${{ github.token }}
          GITHUB_TOKEN: ${{ github.token }}
          GITHUB_REPOSITORY: ${{ github.repository }}
        run: |
          gh issue list --label boxofrocks --state open --json number --jq '.[].number' | while read -r num; do
            echo "Reconciling issue #${num}"
            ISSUE_NUMBER="${num}" /tmp/reconcile
          done
```

This workflow runs every 15 minutes (and on manual dispatch), reconciling all open issues with the `boxofrocks` label. Adjust the cron schedule to match your needs.

The composite action is still available for reconciling a single issue on demand.

## Version Pinning

Pin to a release tag to lock the reconcile binary version:

```yaml
# Pinned — uses the v1.2.0 binary
- uses: jmaddaus/boxofrocks/arbiter@v1.2.0

# Floating — always uses the latest release binary
- uses: jmaddaus/boxofrocks/arbiter@main
```

When referencing a `v*` tag, the action downloads the binary from that specific release. Any other ref (branch, SHA) falls back to the latest release.

To upgrade: change the tag in your workflow file (e.g., `@v1.2.0` → `@v1.3.0`).

For the scheduled workflow above, replace the download URL to pin a version:

```
https://github.com/jmaddaus/boxofrocks/releases/download/v1.2.0/reconcile-linux-amd64
```

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
