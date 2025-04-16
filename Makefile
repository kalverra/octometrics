goreleaser:
	goreleaser release --clean --snapshot

test:
	OCTOMETRICS_TEST_LOG_LEVEL=disabled go tool gotestsum -- -cover ./...

test-race:
	OCTOMETRICS_TEST_LOG_LEVEL=disabled go tool gotestsum -- -cover -race ./...

test-verbose:
	OCTOMETRICS_TEST_LOG_LEVEL=debug go tool gotestsum -- -cover ./...

lint:
	golangci-lint run --fix

bench:
	OCTOMETRICS_TEST_LOG_LEVEL=disabled go test -bench=. -benchmem -run=^$$ ./... -cpu=2,4,8
