.DEFAULT_GOAL := help

.PHONY: help test test-race vet fmt test-full

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests.
	go test ./...

test-race: ## Run unit tests with the race detector.
	go test -race ./...

vet: ## Run static analysis.
	go vet ./...

fmt: ## Format Go source files.
	gofmt -w $$(find . -type f -name '*.go')

test-full: vet test-race ## Run all validation checks.

