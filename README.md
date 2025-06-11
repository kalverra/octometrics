# Octometrics

A simple CLI tool to visualize and profile your GitHub Actions workflows. See all the processes that run as part of a PR, workflow, or job in a simple, interactive chart.

![Example PR run](example.png)

## Run

```sh
# Show help menu
go run . -h
```

Octometrics breaks down into 3 different subcommands: `monitor`, `gather`, and `observe`. Append the `-h` flag to any of them for more detailed information.

### Gather

This gathers data about GitHub actions into Octometrics for later observation. You can gather data for a specific workflow run, commit, or a full PR.

```sh
# Gather all data for this PR: https://github.com/kalverra/octometrics/pull/14
go run . gather -o kalverra -r octometrics -p 14
```

### Observe

Once data is gathered, you can observe it in an interactive browser session.

```sh
go run . observe
```

This will bring up a visualization in your browser to go through all the data you have collected and click the links for more detailed info. You can also click on each bar in a timeline to drill deeper down into the detail of what happened in your CI run.

### Monitor ⚠️ Under Construction

This will launch a background process to monitor stats like CPU and memory usage. This can be run on GHA runners so that when you later `gather` and `observe` the data, you will also have detailed profiling info.

```sh
go run . monitor
```

Highly inspired by the [workflow-telemetry-action](https://github.com/catchpoint/workflow-telemetry-action/tree/master).
