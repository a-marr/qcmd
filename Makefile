# Makefile for qcmd
# Build and development tasks

# Version from git tag, commit, or default
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build metadata
LDFLAGS := -X main.version=$(VERSION)
BUILD_DIR := bin
BINARY := qcmd

# Go build settings
GO := go
GOFLAGS := -trimpath
CGO_ENABLED := 0

# Cross-compilation targets
PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64

# Install location
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: all build build-all test test-coverage lint clean install help

# Default target
all: build

# Build binary for current platform
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/qcmd

# Cross-compile for all platforms
build-all: $(PLATFORMS)

darwin-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/qcmd

darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/qcmd

linux-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/qcmd

linux-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/qcmd

# Run tests with race detector
test:
	$(GO) test -race ./...

# Run tests with coverage report
test-coverage:
	@mkdir -p $(BUILD_DIR)
	$(GO) test -race -coverprofile=$(BUILD_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"
	$(GO) tool cover -func=$(BUILD_DIR)/coverage.out

# Run linter
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	$(GO) clean -cache -testcache

# Install to ~/.local/bin
install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

# Uninstall from ~/.local/bin
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Removed $(BINARY) from $(INSTALL_DIR)"

# Show help
help:
	@echo "qcmd Makefile targets:"
	@echo ""
	@echo "  build          Build binary for current platform (bin/qcmd)"
	@echo "  build-all      Cross-compile for darwin/linux amd64/arm64"
	@echo "  test           Run tests with race detector"
	@echo "  test-coverage  Generate test coverage report"
	@echo "  lint           Run golangci-lint"
	@echo "  clean          Remove build artifacts"
	@echo "  install        Install to ~/.local/bin"
	@echo "  uninstall      Remove from ~/.local/bin"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION        Override version string (default: git describe)"
	@echo ""
	@echo "Current version: $(VERSION)"
