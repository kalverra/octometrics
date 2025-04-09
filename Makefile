test:
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
	set -euo pipefail
	go test -json -v ./... -silence-test-logs -cover 2>&1 | tee /tmp/gotest.log | gotestfmt

test-verbose:
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
	set -euo pipefail
	go test -json -v ./... -cover 2>&1 | tee /tmp/gotest.log | gotestfmt

lint:
	golangci-lint run --fix

bench:
	go test -bench=. -benchmem -run=^$$ ./...
