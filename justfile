goreleaser:
	goreleaser release --clean --snapshot

lint:
	golangci-lint run ./... --fix

test:
	go tool gotestsum -- -cover ./...

test_race:
	go tool gotestsum -- -race ./...

bench:
	go test -bench=. -benchmem -run=^$$ ./... -cpu=2,4,8

mcp:
	go run . mcp --github-token $GITHUB_TOKEN

mocks:
	go generate ./...
