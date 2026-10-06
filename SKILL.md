---
name: octometrics
description: Profile, audit, and inspect GitHub Actions workflows, jobs, and runner sizing. Triage CI failures, audit runner utilization and rightsizing costs, fetch clean job logs, or compare workflow runs side-by-side.
---

# Octometrics Agent Guide

`octometrics` is a CLI tool for profiling and analyzing GitHub Actions workflows, jobs, and runner steps. It calculates execution durations, identifies critical paths, downloads clean logs, and outputs Markdown or JSON.

## Core Rules

1. **URL-First**: Always prefer passing a GitHub URL (`https://github.com/...`). It auto-detects owner, repo, run ID, PR number, or commit SHA.
2. **Output Format**:
   - Non-interactive shells (scripts, agents) default to Markdown (`--format md`).
   - Use `--json` for machine-parseable JSON stdout.
   - Use `-f <path>` to save output directly to a file.
3. **In-Progress Runs**: By default, `octometrics` waits for active runs to finish (`--wait=true`, polling every 10s up to 30m). Pass `--wait=false` to inspect immediately available completed jobs without waiting.
4. **Authentication**: Set `GITHUB_TOKEN` in the environment to avoid GitHub API rate limits.

---

## Workflows

### 1. Triage CI Failures

When a PR check or workflow run fails, inspect the overall failure state, then grab clean logs for the specific failed job.

1. **Profile the run**:
   ```sh
   octometrics <pr-or-run-url>
   ```
2. **Locate failed jobs**:
   Inspect the rendered Markdown summary table for jobs with `failure` status and their run durations.
3. **Fetch clean logs**:
   Run `log` with the job URL or ID:
   ```sh
   octometrics log https://github.com/owner/repo/actions/runs/123/job/456
   ```
   Or if given owner, repo, and job ID:
   ```sh
   octometrics log 456 -o owner -r repo
   ```
4. **Identify root cause**:
   The `log` command strips ANSI escape codes and normalizes timestamps. Read the error lines to determine why the step failed.

**Completion Criterion**: Failing job and step identified with specific error message cited.

---

### 2. Profile Workflow Bottlenecks

When optimizing CI speed or investigating why a workflow is slow.

1. **Profile the PR or run**:
   ```sh
   octometrics https://github.com/owner/repo/pull/123
   ```
2. **Analyze step timeline**:
   - Review job duration totals and step breakdowns.
   - Look for setup steps taking excessive time (e.g. cold caches, dependency downloads).
   - Check runner queue time vs active execution time.
3. **Rank slow steps**:
   Identify the top 3 longest-running steps across the workflow.

**Completion Criterion**: Slowest steps ranked by duration with optimization recommendations.

---

### 3. Compare Workflow Performance

When verifying an optimization (before vs after), or comparing a feature branch against `main`.

1. **Compare by URL**:
   ```sh
   octometrics <target-run-url> --vs <baseline-run-url-or-sha>
   ```
2. **Compare by Run IDs or Commits**:
   ```sh
   octometrics compare -o owner -r repo --workflow-runs 123,456
   octometrics compare -o owner -r repo --commits sha1,sha2
   ```
3. **Review duration deltas**:
   Examine the comparison table to verify duration reductions and ensure no unintended status changes.

**Completion Criterion**: Duration delta table reviewed; performance improvement or regression quantified.

---

### 4. Audit Workflow Performance & Runner Sizing

**When to Use**:
- **Runner Rightsizing**: Check if jobs are over-provisioned (e.g. provisioned with 16 vCPUs / 64 GB RAM but only using 15 GB RAM and 15% CPU).
- **Cost Optimization**: Calculate projected dollar and percentage savings per run from downgrading instance tiers, enabling spot instances (`spot=true` for ~60% savings), or switching to AWS Graviton (`arch=arm64` for ~20% savings).
- **OOM Prevention**: Identify jobs dangerously close to runner memory limits (>85% peak RAM usage) before they fail with Out-Of-Memory errors.
- **Workflow Bottlenecks**: Identify critical-path jobs that dominate total workflow execution time (>60% of total duration).
- **Multi-Run Baseline Auditing**: Gather statistical distributions (average, min, max, P95 duration and cost) across 10-50 historical runs instead of looking at a single run in isolation.

**Data Sources**:
- Automatically extracts `system.cpu.utilization`, `system.memory.utilization`, `system.cpu.load_average.1m/5m`, and network I/O from RunsOn agent logs (`📊 Job Metrics`).
- Falls back to `octometrics-action` measurements if RunsOn logs are not present.

**Steps**:
1. **Audit by Workflow URL**:
   Always pass `--ai-output` in non-interactive/agent sessions to print terminal Markdown and suppress browser launch:
   ```sh
   octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --ai-output
   ```
   *Aliases*: `octometrics workflow` or `octometrics wf`. Direct root URL dispatch also works: `octometrics https://.../workflows/ci.yaml --ai-output`.

2. **Control Sample Size & Filter Runs**:
   Pass run count as a positional argument or flag (`--runs`, `-n`, `--limit`), and selectively include or exclude specific run IDs or URLs:
   ```sh
   # Sample 25 recent runs
   octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml 25 --ai-output

   # Audit only specific run IDs or URLs
   octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --include-runs 12345,67890 --ai-output

   # Exclude outlier, flaky, or cancelled runs
   octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml 20 --exclude-runs 11111 --ai-output

   # Filter by branch or status
   octometrics audit ci.yaml 15 -o owner -r repo --branch main --status completed --ai-output
   ```

3. **Audit with Structured JSON Output**:
   ```sh
   octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml --json
   ```

4. **Tune Rightsizing Thresholds**:
   Adjust underutilization triggers if more or less aggressive rightsizing is desired:
   ```sh
   # Downgrade if peak RAM < 40% or average CPU < 25%
   octometrics audit https://.../ci.yaml --threshold-mem 40 --threshold-cpu 25 --ai-output
   ```

5. **Review Recommendations & Actionable YAML**:
   - Check proposed runner configuration downgrades (e.g. `cpu=16/ram=64/family=m6i` $\to$ `cpu=8/ram=32/family=m6i`) leaving $\ge 30\%$ safety headroom.
   - Copy the generated `runs-on:` YAML snippets directly into the workflow file.

**Completion Criterion**: Peak memory and average CPU utilization evaluated; rightsizing recommendation, actionable YAML, and cost savings quantified.

---

## Command Reference

| Command / Flag | Purpose | Example |
| :--- | :--- | :--- |
| `octometrics <url>` | Profile PR, run, or commit | `octometrics https://github.com/owner/repo/pull/42` |
| `octometrics audit <url> [runs] --ai-output` | Audit workflow sizing & costs (terminal markdown) | `octometrics audit https://github.com/owner/repo/actions/workflows/ci.yaml 20 --ai-output` |
| `octometrics audit <url> --include-runs <ids>` | Audit specific run IDs/URLs | `octometrics audit https://.../ci.yaml --include-runs 123,456` |
| `octometrics audit <url> --exclude-runs <ids>` | Exclude specific run IDs/URLs | `octometrics audit https://.../ci.yaml --exclude-runs 123` |
| `octometrics audit <url> --threshold-mem <pct>` | Custom RAM rightsizing trigger | `octometrics audit https://.../ci.yaml --threshold-mem 40` |
| `octometrics audit <url> --json` | Output structured audit JSON | `octometrics audit https://.../actions/workflows/ci.yaml --json` |
| `octometrics log <job>` | Fetch ANSI-stripped logs | `octometrics log https://github.com/owner/repo/actions/runs/1/job/2` |
| `octometrics <url> --vs <target>` | Compare two runs or commits | `octometrics https://.../runs/2 --vs 1` |
| `octometrics <url> --json` | Output structured JSON | `octometrics https://.../pull/42 --json` |
| `octometrics <url> --wait=false` | Skip waiting for active runs | `octometrics https://.../pull/42 --wait=false` |
| `octometrics <url> -u` | Force refresh cache | `octometrics https://.../pull/42 -u` |
| `octometrics skill` | Output this agent guide | `octometrics skill` |
