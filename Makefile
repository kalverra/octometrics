goreleaser:
	goreleaser release --clean --snapshot

test:
	OCTOMETRICS_TEST_LOG_LEVEL=disabled gotestsum -- -cover ./...

test-race:
	OCTOMETRICS_TEST_LOG_LEVEL=disabled gotestsum -- -cover -race ./...

test-verbose:
	gotestsum -- -cover ./...

lint:
	golangci-lint run --fix

bench:
	go test -bench=. -benchmem -run=^$$ ./... -cpu=2,4,8
