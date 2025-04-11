goreleaser:
	goreleaser release --clean --snapshot

test:
	go install gotest.tools/gotestsum@latest
	gotestsum -- -coverprofile=cover.out ./... -silence-test-logs

test-race:
	go install gotest.tools/gotestsum@latest
	gotestsum -- -race ./... -silence-test-logs

test-verbose:
	go install gotest.tools/gotestsum@latest
	gotestsum -- -coverprofile=cover.out ./...

lint:
	golangci-lint run --fix

bench:
	go test -bench=. -benchmem -run=^$$ ./...
