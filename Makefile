BIN := pwrq
VERSION := 0.1.0
CURRENT_REVISION := $(shell git rev-parse --short HEAD 2>/dev/null || echo "HEAD")
BUILD_LDFLAGS := -s -w -X github.com/xen0bit/pwrq/cli.revision=$(CURRENT_REVISION)
GOBIN ?= $(shell go env GOPATH)/bin
SHELL := /bin/bash

.PHONY: all
all: build-with-ide

.PHONY: build
build:
	@echo "Building $(BIN)..."
	go build -ldflags="$(BUILD_LDFLAGS)" -o $(BIN) ./cmd/$(BIN)

.PHONY: build-with-ide
build-with-ide: web.build
	@echo "Building $(BIN) with embedded web assets..."
	go build -tags embed_web -ldflags="$(BUILD_LDFLAGS)" -o $(BIN) ./cmd/$(BIN)

.PHONY: install
install:
	@echo "Installing $(BIN)..."
	go install -ldflags="$(BUILD_LDFLAGS)" ./cmd/$(BIN)

.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race ./...

.PHONY: test-short
test-short:
	@echo "Running short tests..."
	go test -v ./...

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
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
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
	GOOS=js GOARCH=wasm go build -ldflags="$(BUILD_LDFLAGS)" -o pkg/web/src/wasm/web.wasm ./cmd/web
	@echo "Copying wasm_exec.js..."
	@cp $$(go env GOROOT)/lib/wasm/wasm_exec.js pkg/web/src/wasm/ 2>/dev/null || cp $$(go env GOROOT)/misc/wasm/wasm_exec.js pkg/web/src/wasm/ 2>/dev/null || echo "Warning: wasm_exec.js not found, you may need to copy it manually"

.PHONY: web.build
web.build: web.wasm
	@echo "Building web with bun..."
	@cd pkg/web && bun build src/web_example.html --outdir dist --target browser

.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -f $(BIN)
	rm -f web.wasm
	rm -rf pkg/web/src/wasm
	rm -rf pkg/web/dist
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
	@echo "  make                - Build WASM, web assets, and CLI with embedded IDE (default)"
	@echo "  make build          - Build the $(BIN) binary (without embedded web assets)"
	@echo "  make build-with-ide - Build $(BIN) with embedded web assets for IDE"
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
	@echo "  make web.build      - Build web.wasm and compile pkg/web/ with bun"
	@echo "  make build-with-ide - Build $(BIN) with embedded web assets for IDE"
	@echo "  make help           - Show this help message"

.PHONY: version
version:
	@echo "$(VERSION) (rev: $(CURRENT_REVISION))"

