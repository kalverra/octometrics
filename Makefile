test:
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
	set -euo pipefail
	go test -json -v ./... -cover 2>&1 | tee /tmp/gotest.log | gotestfmt

lint:
	golangci-lint run --fix --timeout 5m
