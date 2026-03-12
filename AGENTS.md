# Agents Instructions

## Project Overview

Octometrics is a Go CLI that profiles GitHub Actions workflows. Read `design.md` for architecture diagrams and key design decisions. The main commands are:

| Command   | Purpose                                                                                        |
| --------- | ---------------------------------------------------------------------------------------------- |
| `monitor` | Collects system metrics (CPU, memory, disk, I/O) during a GHA job, writes JSONL                |
| `gather`  | Fetches workflow/job/step data from the GitHub REST & GraphQL APIs, stores as JSON             |
| `observe` | Renders gathered data as interactive HTML (Mermaid Gantt charts, Plotly metric charts)         |
| `report`  | Analyzes monitor JSONL and posts Mermaid-based summaries to GHA step summaries and PR comments |

Key packages: `cmd/` (Cobra CLI), `monitor/` (system metrics), `gather/` (GitHub API), `observe/` (HTML visualization), `report/` (in-action reporting), `internal/config/` (Viper config), `logging/` (zerolog setup).

## Testing and Linting

After making changes, always run:

```sh
make lint
make test
```

Analyze the outputs and fix issues you introduced. **Do not change a test unless it is necessary to comply with new changes or implementations**.

- `make lint` runs `golangci-lint run ./... --fix` with the config in `.golangci.yaml`.
- `make test` runs `go tool gotestsum -- -cover ./...`.
- Tests use `github.com/stretchr/testify` (`require` for fatal checks, `assert` for non-fatal).
- Use the `internal/testhelpers.Setup(t)` helper to create a temp directory and logger for tests. It auto-cleans on success and preserves on failure.
- Test data goes in `<package>/testdata/` directories.
- You can run `pre-commit` using the `.pre-commit-config.yaml` file for extensive checks.

## Coding Conventions

- **Functional options pattern**: Commands and packages use `Option` funcs (e.g. `monitor.WithOutputFile()`, `gather.ForceUpdate()`). Follow this pattern when adding configurable behavior.
- **Logging**: Use `zerolog`. Pass `zerolog.Logger` as the first parameter to package-level functions. Log API calls at Trace, operational events at Debug/Info, and problems at Warn/Error.
- **Error handling**: Wrap errors with `fmt.Errorf("context: %w", err)`. Deferred `Close()` calls must check the error (see `errcheck` linter).
- **GitHub API**: Use `github.com/google/go-github/v84`. Rate limiting is handled by `go-github-ratelimit`. See `gather/gather.go` for the client setup pattern.
- **No unnecessary comments**: Do not add comments that merely narrate what the code does. Comments should explain non-obvious intent or constraints.

## PR Review Instructions

When performing a Pull Request review, do your typical PR analysis, and:

### 1. Risk Assessment
Provide a **Risk Rating** at the top of the review summary:
- **HIGH:** Changes to core logic, fundamental architectural patterns, or critical shared utilities.
- **MEDIUM:** Significant feature additions or modifications to established business logic.
- **LOW:** Documentation, styling, minor bug fixes in non-critical paths, or boilerplate.

### 2. Targeted Review Areas
Identify and call out specific code blocks that require **scrupulous human review**. Focus on:
- Complex conditional logic or concurrency-prone areas.
- Potential breaking changes in internal or external APIs.
- Logic that lacks sufficient unit test coverage within the PR.

If you find any, list them and give a brief description of why they deserve extra attention.

### 3. Reviewer Recommendations
Analyze the git history (recent editors) to suggest the most qualified reviewers.
- Prioritize individuals who have made significant recent contributions to the specific files modified.
