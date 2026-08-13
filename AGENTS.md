# Agents Instructions

## Project Overview

Octometrics is a Go CLI that profiles GitHub Actions workflows. Read `design.md` for architecture diagrams and key design decisions.

## Testing and Linting

- Don't use `go vet`, `gofmt`, or `go fmt`.
- `go generate ./...` to generate mocks

Run these commands after each change

- `golangci-lint run ./... --fix` for linting
- `go test ./...` for testing

Analyze the outputs and fix issues you introduced. **Do not change a test unless it is necessary to comply with new changes or implementations**.

## PR Review Instructions

When performing a PR review, do your typical PR analysis, and:

### 1. Risk Assessment

Provide a **Risk Rating** at the top of the review summary:

- **HIGH:** Changes to core logic, fundamental architectural patterns, or critical shared utilities.
- **MEDIUM:** Significant feature additions or modifications to established business logic.
- **LOW:** Documentation, styling, minor bug fixes in non-critical paths, or boilerplate.

### 2. Targeted Review Areas

Identify specific code blocks that could benefit from **scrupulous human review**. Focus on:

- Complex conditional logic or concurrency-prone areas.
- Potential breaking changes in internal or external APIs.
- Logic that lacks sufficient unit test coverage within the PR.

## Coding Conventions

- **Functional options pattern**: Commands and packages use `Option` funcs (e.g. `monitor.WithOutputFile()`, `gather.ForceUpdate()`). Follow this pattern when adding configurable behavior.
- **Logging**: Use `zerolog`. Pass `zerolog.Logger` as the first parameter to package-level functions. Log API calls at Trace, operational events at Debug/Info, and problems at Warn/Error.
- **Error handling**: Wrap errors with `fmt.Errorf("context: %w", err)`. Deferred `Close()` calls must check the error (see `errcheck` linter).
- **GitHub API**: Use `github.com/google/go-github/v89`. Rate limiting is handled by `go-github-ratelimit`. See `gather/gather.go` for the client setup pattern.
- **No unnecessary comments**: Do not add comments that merely narrate what the code does. Comments should explain non-obvious intent or constraints.

## Local UI Development and Inspection

- **Live Reload**: Run `mise run dev` (or `air`) to start server on `http://localhost:8080` with proxy live reloading.
  - To target specific view: `air -build.args_bin "--no-open,--port,8081,--format,html,https://github.com/owner/repo/pull/123"`
  - Target page URL: `http://localhost:8080/owner/repo/pull_requests/123.html`
- **Open Browser / VS Code**:
  - Chrome: `open -a "Google Chrome" "http://localhost:8080/owner/repo/pull_requests/123.html"`
  - VS Code Simple Browser: `open "vscode://vscode.simple-browser/open?url=http%3A%2F%2Flocalhost%3A8080%2Fowner%2Frepo%2Fpull_requests%2F123.html"`
- **UI Inspection & Verification**:
  - Fetch HTML/DOM: `read_url_content` for `http://localhost:8080/owner/repo/pull_requests/123.html`
  - Visual layout screenshot: Either use native browser viewing tool, or playwright: `npx -y playwright screenshot http://localhost:8080 UI_PREVIEW.png` then view `UI_PREVIEW.png` via `view_file`.
