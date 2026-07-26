# LazyAzure Makefile

# Version info - uses git tags if available, otherwise 'dev'
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%d)

# ldflags for version injection
LDFLAGS := -X main.version=$(VERSION) \
           -X main.commit=$(COMMIT) \
           -X main.date=$(DATE)

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o lazyazure .

.PHONY: build-ci
build-ci:
	go build -v -ldflags "$(LDFLAGS)" -o lazyazure .

.PHONY: test
test:
	go test ./...

.PHONY: test-ci
test-ci:
	go test ./... -v -timeout 60s

.PHONY: test-integration
test-integration: build
	@echo "Running integration tests..."
	@which tmux >/dev/null 2>&1 || (echo "Error: tmux is required for integration tests" && exit 1)
	@test -f ./lazyazure || (echo "Error: lazyazure binary not found. Run 'make build' first." && exit 1)
	@echo "Testing with tmux version: $$(tmux -V)"
	@./scripts/test-scrolling.sh
	@./scripts/test-search.sh
	@./scripts/test-panel-switch.sh
	@printf "\033[0;32m✓\033[0m All integration tests passed!\n"

.PHONY: clean
clean:
	rm -f lazyazure

.PHONY: all
all: test build

.PHONY: test-coverage
test-coverage:
	go test -cover ./...

.PHONY: test-race
test-race:
	go test -race ./...

.PHONY: lint
lint:
	go vet ./...

.PHONY: check
check: fmt lint test

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: update-api-versions
update-api-versions:
	@echo "Updating API versions from bicep-types-az..."
	@which curl >/dev/null 2>&1 || (echo "Error: curl is required" && exit 1)
	@curl -sL https://raw.githubusercontent.com/Azure/bicep-types-az/main/generated/index.json -o /tmp/bicep-index.json
	@go run tools/update-api-versions/main.go /tmp/bicep-index.json
	@rm -f /tmp/bicep-index.json
	@echo "API versions updated successfully!"
	@echo "Review changes and commit: pkg/azure/api_versions_curated.json"
