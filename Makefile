VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN := hew

.DEFAULT_GOAL := build
.PHONY: build build-core build-all install clean test vet fmt lint check run help setup man check-man

build: ## Build TUI binary
	cd cmd/hew && go build -trimpath -ldflags '$(LDFLAGS)' -o ../../$(BIN) .

build-core: ## Build plain CLI binary
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)-core ./cmd/hew-core/

build-all: build build-core ## Build both binaries

install: ## Install TUI binary
	cd cmd/hew && go install -ldflags '$(LDFLAGS)' .

clean: ## Remove built binaries
	rm -f $(BIN) $(BIN)-core

test: ## Run all tests
	go test ./... -v
	cd cmd/hew && go test ./... -v

vet: ## Run go vet
	go vet ./...
	cd cmd/hew && go vet ./...

fmt: ## Format source
	go fmt ./...
	cd cmd/hew && go fmt ./...

lint: ## Run linters
	golangci-lint run ./...
	cd cmd/hew && golangci-lint run ./...

check: lint test ## Run lint and tests

run: build ## Build and run
	./$(BIN)

man: ## Generate man page from markdown
	go-md2man -in doc/hew.1.md -out doc/hew.1

check-man: ## Verify committed man page is up-to-date
	@cp doc/hew.1 doc/hew.1.bak 2>/dev/null || true
	@go-md2man -in doc/hew.1.md -out doc/hew.1
	@diff -q doc/hew.1 doc/hew.1.bak >/dev/null 2>&1 || (echo "error: doc/hew.1 is out of date; run 'make man'" && mv doc/hew.1.bak doc/hew.1 && exit 1)
	@mv doc/hew.1.bak doc/hew.1
	@echo "man page is up-to-date"

help: ## List available targets
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | sort | awk -F ':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

setup: ## Install git hooks
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
