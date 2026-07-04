BINARY_DIR := deploy/dist
BINARY := $(BINARY_DIR)/databuff-diag
MAIN_PKG := ./cmd/databuff-diag
VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X github.com/databufflabs/databuff-diag/internal/version.Version=$(VERSION)"

.PHONY: build test lint clean release snapshot size

build:
	@mkdir -p $(BINARY_DIR)
	go build $(LDFLAGS) -o $(BINARY) $(MAIN_PKG)

test:
	go test ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed; skipping"; exit 0; }
	golangci-lint run ./...

release:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed; see https://goreleaser.com"; exit 1; }
	@if goreleaser release --help 2>&1 | grep -q -- '--clean'; then \
		goreleaser release --clean; \
	else \
		goreleaser release --rm-dist; \
	fi

snapshot:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed; see https://goreleaser.com"; exit 1; }
	@if goreleaser build --help 2>&1 | grep -q -- '--clean'; then \
		goreleaser build --snapshot --clean; \
	else \
		goreleaser build --snapshot --rm-dist; \
	fi

size: build
	@ls -lh $(BINARY) | awk '{print $$5 "\t" $$9}'
	@bytes=$$(stat -f%z $(BINARY) 2>/dev/null || stat -c%s $(BINARY)); \
	  mb=$$(echo "scale=2; $$bytes / 1048576" | bc); \
	  echo "$$mb MB"; \
	  test "$$bytes" -lt 31457280 || (echo "binary exceeds 30MB limit" >&2; exit 1)

clean:
	rm -rf deploy/dist
	rm -rf dist/
	rm -f databuff-diag databuff-diag-*
