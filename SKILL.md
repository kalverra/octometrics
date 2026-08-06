---
name: octometrics
description: Profile GitHub Actions workflows, inspect job durations and step timelines, compare runs/commits, and output raw Markdown for AI agent analysis.
---

# Octometrics Agent Guide

`octometrics` is a Go CLI for profiling GitHub Actions workflows. It fetches workflow run data via GitHub API, calculates costs and step timelines, and produces structured Markdown output.

## Recommended Agent Flow

Always pass `--format md` or `--stdout` to print raw Markdown directly to standard output without launching an interactive HTTP server or opening a browser.

### 1. Inspect Workflow Run, PR, or Commit
Gather GitHub Actions data and output raw Markdown directly:

```bash
# Specific workflow run
octometrics -o <owner> -r <repo> -w <workflow_run_id> --format md

# Specific pull request or commit
octometrics -o <owner> -r <repo> -p <pr_number> --format md
octometrics -o <owner> -r <repo> -c <commit_sha> --format md
```

### 2. Inspect All Cached Metrics
Read and render previously gathered observations without target flags:

```bash
octometrics --format md
```

### 3. Compare Workflow Runs or Commits
Compare duration, job steps, and cost deltas side-by-side:

```bash
# Compare two workflow run IDs
octometrics compare -o <owner> -r <repo> --workflow-runs <run_id_1>,<run_id_2> --format md

# Compare two commit SHAs
octometrics compare -o <owner> -r <repo> --commits <sha_1>,<sha_2> --format md
```

### 4. Generate GitHub Action Post-Run Report
Analyze local monitor JSONL log and produce job step timing and Mermaid reports:

```bash
octometrics report -f octometrics.monitor.jsonl --skip-comment --skip-summary
```

## Key Options for Agents

- `--format md`: Renders output as raw Markdown instead of launching HTML browser session.
- `--stdout`: Forces raw output to `stdout`.
- `-t <token>` / `GITHUB_TOKEN`: GitHub API token for authentication.
- `--data-dir <path>`: Directory where cached workflow data is stored.
