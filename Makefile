.DEFAULT_GOAL := help

.PHONY: help build release test test-race test-e2e vet fmt test-full

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the executable.
	mkdir -p bin
	go build -o bin/planesync ./cmd/planesync

release: ## Build the macOS arm64 release artifact.
	mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -o bin/planesync-darwin-arm64 ./cmd/planesync

test: ## Run unit tests.
	go test ./...

test-race: ## Run unit tests with the race detector.
	go test -race ./...

test-e2e: build ## Run the offline end-to-end fixture.
	./tests/e2e.sh

vet: ## Run static analysis.
	go vet ./...

fmt: ## Format Go source files.
	gofmt -w $$(find . -type f -name '*.go')

test-full: vet test-race test-e2e ## Run all validation checks.
