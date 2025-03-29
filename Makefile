test:
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
	set -euo pipefail
	go test -json -v ./... 2>&1 | tee /tmp/gotest.log | gotestfmt
