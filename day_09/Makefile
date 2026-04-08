# Makefile for AI Agent Platform (day_08)
# Build, test, and development automation

.PHONY: help build test test-race clean run lint

# Default target
help:
	@echo "AI Agent Platform - Build and Development Tools"
	@echo ""
	@echo "Available targets:"
	@echo "  help       - Show this help message"
	@echo "  build      - Build the application (output: bin/server)"
	@echo "  run        - Build and run the server"
	@echo "  test       - Run all tests"
	@echo "  test-race  - Run tests with race detector"
	@echo "  clean      - Remove build artifacts"
	@echo "  lint       - Run golangci-lint (if installed)"
	@echo "  deps       - Download dependencies"
	@echo ""

# Build the application
build:
	@echo "Building AI Agent Platform..."
	go build -o bin/server ./server/main.go
	@echo "Build complete: bin/server"

# Run the application (builds first)
run: build
	@echo "Starting AI Agent Platform server..."
	@PORT=$${PORT:-8080} ./bin/server

# Run all tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

# Run integration tests
integration-test:
	@echo "Running integration tests..."
	go test -v ./integration/...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f bin/server
	@echo "Clean complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	@echo "Dependencies updated"

# Run golangci-lint (if installed)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Development target: build and run tests
dev: deps build test
	@echo "Development build complete"

# Full CI pipeline
ci: deps lint test-race build
	@echo "CI pipeline completed successfully"

# Install dependencies for development
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed"