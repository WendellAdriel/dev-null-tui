GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck

.PHONY: fmt fmt-check lint test test-race coverage vuln verify build build-all check

fmt:
	$(GOLANGCI_LINT) fmt

fmt-check:
	$(GOLANGCI_LINT) fmt --diff

lint:
	$(GOLANGCI_LINT) run ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

coverage:
	$(GO) test -coverprofile=coverage.out ./...

vuln:
	$(GOVULNCHECK) ./...

verify:
	$(GO) mod verify

build:
	$(GO) build -trimpath -o bin/ ./cmd/devnull

build-all:
	$(GO) run ./tools/build

check: fmt-check lint verify test vuln
