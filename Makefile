BIN := pwrq
VIZ_BIN := pwrq-viz
VERSION := 0.1.0
CURRENT_REVISION := $(shell git rev-parse --short HEAD 2>/dev/null || echo "HEAD")
BUILD_LDFLAGS := -s -w -X github.com/xen0bit/pwrq/cli.revision=$(CURRENT_REVISION)

# Grammars for structural search (select_ast). gotreesitter embeds all 206 of
# its grammars unless told otherwise, which costs 23MB of binary; naming the
# languages costs a few MB and covers what anyone actually greps. Add a
# language by adding it here - the cmdlets read the registry, so
# get_ast_language reports whatever this list says without anything else
# changing.
#
# Every language a shipped rule is written for has to be in here, or that rule
# is in the binary and can never fire. TestEveryRuleLanguageIsInTheBuild reads
# the corpus and says so.
GRAMMARS := go python javascript typescript tsx rust java c cpp c_sharp ruby php \
            bash powershell sql json yaml toml hcl xml html css markdown dockerfile \
            apex clojure dart elixir kotlin lua ocaml scala solidity swift
GRAMMAR_TAGS := grammar_subset $(addprefix grammar_subset_,$(GRAMMARS))
BUILD_TAGS := $(GRAMMAR_TAGS)
# The page reports the revision it was built from, so a shared link's behaviour
# can be traced back to a commit.
WEB_LDFLAGS := -X github.com/xen0bit/pwrq/pkg/webapi.Version=$(VERSION)-$(CURRENT_REVISION)
GOBIN ?= $(shell go env GOPATH)/bin
SHELL := /bin/bash

.PHONY: all
all: build

.PHONY: build
build:
	@echo "Building $(BIN)..."
	go build -tags "$(BUILD_TAGS)" -ldflags="$(BUILD_LDFLAGS)" -o $(BIN) ./cmd/$(BIN)

# pwrq-viz carries the query diagramming and the browser IDE. It is separate
# because d2 pulls in a JavaScript engine, a syntax highlighter and a PDF
# writer - roughly 35MB the everyday pwrq has no use for.
.PHONY: build-viz
build-viz:
	@echo "Building $(VIZ_BIN)..."
	go build -tags "viz $(BUILD_TAGS)" -ldflags="$(BUILD_LDFLAGS)" -o $(VIZ_BIN) ./cmd/$(VIZ_BIN)

.PHONY: build-viz-with-ide
build-viz-with-ide: web.build
	@echo "Building $(VIZ_BIN) with embedded web assets..."
	go build -tags "viz embed_web $(BUILD_TAGS)" -ldflags="$(BUILD_LDFLAGS)" -o $(VIZ_BIN) ./cmd/$(VIZ_BIN)

.PHONY: build-all
build-all: build build-viz

.PHONY: install
install:
	@echo "Installing $(BIN)..."
	go install -tags "$(BUILD_TAGS)" -ldflags="$(BUILD_LDFLAGS)" ./cmd/$(BIN)

.PHONY: test
test:
	@echo "Running tests..."
	go test -race ./...
	@echo "Running tests for the viz build..."
	go test -race -tags viz ./cli/...
	@echo "Running the editor's browser-side tests..."
	@$(MAKE) --no-print-directory web.test

.PHONY: test-short
test-short:
	@echo "Running short tests..."
	go test -short ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: lint
lint:
	@echo "Running linters..."
	go vet ./...
	@echo "Running linters for the viz build..."
	go vet -tags viz ./...
	@# The viz files sit behind a build tag, so a default run never sees them.
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./... && \
		golangci-lint run --build-tags viz ./...; \
	else \
		echo "golangci-lint not found, skipping..."; \
	fi

.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

.PHONY: web.wasm
web.wasm:
	@echo "Building web.wasm..."
	@mkdir -p pkg/web/src/wasm
	GOOS=js GOARCH=wasm go build -tags viz -ldflags="$(BUILD_LDFLAGS) $(WEB_LDFLAGS)" -o pkg/web/src/wasm/web.wasm ./cmd/web
	@echo "Copying wasm_exec.js..."
	@cp $$(go env GOROOT)/lib/wasm/wasm_exec.js pkg/web/src/js/wasm_exec.js 2>/dev/null || cp $$(go env GOROOT)/misc/wasm/wasm_exec.js pkg/web/src/js/wasm_exec.js 2>/dev/null || echo "Warning: wasm_exec.js not found, you may need to copy it manually"

.PHONY: web.build
web.build: web.wasm
	@echo "Building web assets with bun..."
	@if ! command -v bun >/dev/null 2>&1; then \
		echo "Error: bun is not installed. Please install it from https://bun.sh"; \
		exit 1; \
	fi
	@mkdir -p pkg/web/dist
	@cd pkg/web/src && bun install --no-save 2>/dev/null || true
	@cd pkg/web/src && bun run build
	@echo "Copying the runtime files bundling must not touch..."
	@# wasm_exec.js and worker.js are loaded by URL rather than imported, so
	@# they are copied verbatim; the module itself is fetched at runtime.
	@cp pkg/web/src/js/wasm_exec.js pkg/web/dist/wasm_exec.js
	@cp pkg/web/src/worker.js pkg/web/dist/worker.js
	@cp pkg/web/src/wasm/web.wasm pkg/web/dist/web.wasm
	@echo "Pre-compressing web.wasm (the server prefers it when the browser does)..."
	@gzip -9 -f -k -c pkg/web/dist/web.wasm > pkg/web/dist/web.wasm.gz 2>/dev/null || echo "Warning: gzip not found; the module will be served uncompressed"
	@ls -lh pkg/web/dist/web.wasm pkg/web/dist/web.wasm.gz 2>/dev/null | awk '{print "  " $$9 " " $$5}'

.PHONY: web.test
web.test:
	@echo "Testing the editor's browser-side code..."
	@if ! command -v bun >/dev/null 2>&1; then \
		echo "Error: bun is not installed. Please install it from https://bun.sh"; \
		exit 1; \
	fi
	@cd pkg/web/src && bun test

.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -f $(BIN) $(VIZ_BIN)
	rm -f web.wasm
	rm -rf pkg/web/src/wasm
	rm -rf pkg/web/dist
	rm -f pkg/web/src/js/wasm_exec.js
	rm -f coverage.out coverage.html
	go clean ./...

.PHONY: run
run: build
	@./$(BIN) $(ARGS)

.PHONY: example
example: build
	@echo "Running example: find function"
	@echo 'null' | ./$(BIN) '[find("pkg")] | .[0:3]'

.PHONY: examples
examples: build
	@echo "=== Example 1: Basic find ==="
	@echo 'null' | ./$(BIN) '[find("pkg")] | length'
	@echo ""
	@echo "=== Example 2: Find files only ==="
	@echo 'null' | ./$(BIN) '[find("pkg"; "file")] | .[0:3]'
	@echo ""
	@echo "=== Example 3: Find with options ==="
	@echo 'null' | ./$(BIN) '[find("pkg"; {"type": "dir", "maxdepth": 2})] | .[0:5]'

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make                - Build $(BIN) (default)"
	@echo "  make build          - Build the $(BIN) binary"
	@echo "  make build-viz      - Build $(VIZ_BIN) (adds query diagrams and the IDE)"
	@echo "  make build-all      - Build both binaries"
	@echo "  make install        - Install $(BIN) to $$GOPATH/bin"
	@echo "  make test           - Run all tests with race detector"
	@echo "  make test-short     - Run tests without race detector"
	@echo "  make test-coverage  - Run tests and generate coverage report"
	@echo "  make lint           - Run linters (requires golangci-lint)"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make run ARGS=...   - Build and run with arguments"
	@echo "  make example        - Run a simple example"
	@echo "  make examples       - Run multiple examples"
	@echo "  make web.wasm       - Build web.wasm into pkg/web/src/wasm/"
	@echo "  make web.build      - Build the browser editor into pkg/web/dist/"
	@echo "  make web.test       - Run the editor's browser-side tests (needs bun)"
	@echo "  make build-viz-with-ide - Build $(VIZ_BIN) with embedded web assets"
	@echo "  make help           - Show this help message"

.PHONY: version
version:
	@echo "$(VERSION) (rev: $(CURRENT_REVISION))"

