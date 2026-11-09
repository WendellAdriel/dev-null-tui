GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GOHAWK ?= gohawk

.PHONY: fmt fmt-check lint hawk test test-race coverage vuln verify build build-all check

fmt:
	$(GOLANGCI_LINT) fmt

fmt-check:
	$(GOLANGCI_LINT) fmt --diff

lint:
	$(GOLANGCI_LINT) run ./...

hawk:
	$(GOHAWK) -enable-all ./...

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

check: fmt-check lint hawk verify test vuln
