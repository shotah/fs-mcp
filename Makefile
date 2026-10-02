# fs-mcp Makefile
#
# Run `make` or `make help` to see everything.

.DEFAULT_GOAL := help

.PHONY: help fmt vet lint test test-race test-short coverage check \
	build cli install tidy deps clean install-hooks tools \
	release version

# Build-time version stamp (git describe). Release tags are tracked in ./VERSION.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -X github.com/shotah/fs-mcp/server.ServerVersion=$(VERSION)

# Release bump: patch (default), minor, or major. Or set TAG=v0.2.0 explicitly.
BUMP ?= patch

PKG ?= ./...

BINARY := fs-mcp
ifeq ($(OS),Windows_NT)
EXE := .exe
else
EXE :=
endif
GOBIN_DIR := $(shell go env GOBIN)
ifeq ($(strip $(GOBIN_DIR)),)
GOBIN_DIR := $(shell go env GOPATH)/bin
endif
ifneq ($(OS),Windows_NT)
export PATH := $(GOBIN_DIR):$(PATH)
endif

##@ Getting oriented

help: ## Show this help
	@echo ""
	@echo "Usage:  make <target>"
	@echo ""
	@echo "Getting oriented"
	@echo "  help                   Show this help"
	@echo ""
	@echo "Daily loop (format -> lint -> test)"
	@echo "  fmt                    Format imports/code (goimports-reviser)"
	@echo "  vet                    Static analysis (go vet)"
	@echo "  lint                   Full lint suite (golangci-lint)"
	@echo "  test                   Unit tests"
	@echo "  test-short             Unit tests with -short"
	@echo "  test-race              Unit tests with the race detector"
	@echo "  coverage               Library coverage (fails below 70%)"
	@echo "  check                  Autofix, lint, coverage gate"
	@echo ""
	@echo "Build & run"
	@echo "  build                  Compile all packages (sanity check)"
	@echo "  cli                    Build the MCP binary into ./bin/$(BINARY)"
	@echo "  install                Install binary into GOPATH/bin"
	@echo "  run                    go run  (make run ARGS=\"--help\")"
	@echo ""
	@echo "Modules & cleanup"
	@echo "  tidy                   Sync go.mod / go.sum with imports"
	@echo "  deps                   Download module deps"
	@echo "  clean                  Remove binaries and coverage artifacts"
	@echo ""
	@echo "Project-specific"
	@echo "  install-hooks          Install git pre-commit (autofix + lint + test)"
	@echo "  version                Show VERSION file + next tag (dry-run)"
	@echo "  release                Bump tag + latest, update VERSION, push (BUMP=patch|minor|major)"
	@echo ""
	@echo "Tooling"
	@echo "  tools                  Install goimports-reviser + golangci-lint v2"
	@echo ""

##@ Daily loop (format → lint → test)

fmt: ## Autofix imports/code (goimports-reviser + golangci-lint fmt/fix)
	goimports-reviser -format -recursive .
	-golangci-lint fmt ./...
	-golangci-lint run --fix ./...

vet: ## Static analysis (go vet)
	go vet ./...

lint: ## Full lint suite (golangci-lint; no write)
	golangci-lint run ./...

test: ## Unit tests (PKG=./path/... for one package)
	go test $(PKG)

test-short: ## Unit tests with -short
	go test -short $(PKG)

test-race: ## Unit tests with the race detector (slower, worth it)
	go test -race $(PKG)

# Library packages only. main is the stdio process.
COVERAGE_PKG ?= ./server/... ./tools/...
COVERAGE_MIN ?= 70

coverage: ## Tests + coverage; fails if total is below COVERAGE_MIN (default 70)
	go test -cover "-coverprofile=coverage.out" -covermode=atomic $(COVERAGE_PKG)
	go tool cover "-func=coverage.out"
	$(SHELL) scripts/check-coverage.sh coverage.out $(COVERAGE_MIN)

check: fmt lint coverage ## Autofix, lint, coverage gate (matches pre-commit)

##@ Build & run

build: ## Compile all packages (sanity check; no binary kept)
	go build ./...

cli: ## Build the MCP binary into ./bin/fs-mcp
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)$(EXE) .

install: ## Install binary into $$GOPATH/bin (or $$GOBIN) as fs-mcp
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$(GOBIN_DIR)/$(BINARY)$(EXE)" .

run: ## Build & run — e.g. make run ARGS="--help"
	go run -ldflags "$(LDFLAGS)" . $(ARGS)

##@ Modules & cleanup

tidy: ## Sync go.mod / go.sum with imports
	go mod tidy

deps: ## Download module deps into the module cache
	go mod download

clean: ## Remove built binaries and coverage artifacts
	go clean ./...
ifeq ($(OS),Windows_NT)
	-cmd /C "rmdir /S /Q bin 2>NUL & del /Q $(BINARY) $(BINARY).exe coverage coverage.out coverage.txt 2>NUL"
else
	rm -rf bin
	rm -f $(BINARY) $(BINARY).exe coverage coverage.out coverage.txt
endif

##@ Project-specific

install-hooks: ## Install git pre-commit hook (autofix + lint + test)
ifeq ($(OS),Windows_NT)
	copy /Y scripts\pre-commit .git\hooks\pre-commit
else
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
endif
	@echo "Installed .git/hooks/pre-commit"

version: ## Show VERSION file and latest git tag / next patch
	@go run ./cmd/release -dry-run

# Bump semver, commit VERSION, annotated-tag (v* + floating latest), push (triggers GoReleaser).
# Examples:
#   make release
#   make release BUMP=minor
#   make release BUMP=major
#   make release TAG=v0.2.0
#   make release DRY_RUN=1
release: ## Bump version + latest tags, update VERSION, push (BUMP=patch|minor|major)
	go run ./cmd/release \
		$(if $(TAG),-version=$(TAG),-bump=$(BUMP)) \
		$(if $(DRY_RUN),-dry-run,) \
		$(if $(SKIP_PUSH),-skip-push,) \
		$(if $(ALLOW_DIRTY),-allow-dirty,)

##@ Tooling

tools: ## Install goimports-reviser + golangci-lint v2 into $$GOBIN
	go install github.com/incu6us/goimports-reviser/v3@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo Installed to $(GOBIN_DIR). make fmt / the pre-commit hook prepend that dir to PATH.
