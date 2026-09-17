.DEFAULT_GOAL := help

.PHONY: help build test test-race test-e2e vet fmt test-full

PLATFORM_BINARY := planesync-$(shell go env GOOS)-$(shell go env GOARCH)

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the executable.
	go build -o $(PLATFORM_BINARY) ./cmd/planesync

test: ## Run unit tests.
	go test ./...

test-race: ## Run unit tests with the race detector.
	go test -race ./...

test-e2e: build ## Run the offline end-to-end fixture.
	PLANESYNC_E2E_BINARY="$(CURDIR)/$(PLATFORM_BINARY)" ./tests/e2e.sh

vet: ## Run static analysis.
	go vet ./...

fmt: ## Format Go source files.
	gofmt -w $$(find . -type f -name '*.go')

test-full: vet test-race test-e2e ## Run all validation checks.
