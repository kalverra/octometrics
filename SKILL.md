---
name: octometrics
description: Profile GitHub Actions workflows, inspect job durations and step timelines, compare runs/commits.
---

`octometrics` is a Go CLI for profiling GitHub Actions workflows. It fetches workflow run data via GitHub API, calculates costs and step timelines, and produces structured Markdown output.

Always pass `--format md` or `--stdout` to print raw Markdown directly to stdout.

```bash
# Get data on specific workflow run, PR, or commit
octometrics -o <owner> -r <repo> -w <workflow_run_id> --format md
octometrics -o <owner> -r <repo> -p <pr_number> --format md
octometrics -o <owner> -r <repo> -c <commit_sha> --format md

# Compare two workflow runs or commits against each other
octometrics compare -o <owner> -r <repo> --workflow-runs <run_id_1>,<run_id_2> --format md
octometrics compare -o <owner> -r <repo> --commits <sha_1>,<sha_2> --format md
```
