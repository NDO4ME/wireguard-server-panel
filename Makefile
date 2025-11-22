.PHONY: build run test clean install deps

# Application name
APP_NAME := wgeasygo

# Build directory
BUILD_DIR := build

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Build the application
build:
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=1 $(GOBUILD) -ldflags="-s -w" -o $(APP_NAME) ./cmd/api

# Build for production (static binary)
build-prod:
	@echo "Building $(APP_NAME) for production..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-s -w" -o $(APP_NAME) ./cmd/api

# Run the application
run: build
	@echo "Running $(APP_NAME)..."
	./$(APP_NAME)

# Run with development config
dev:
	@echo "Running in development mode..."
	GIN_MODE=debug ./$(APP_NAME)

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(APP_NAME)
	rm -f coverage.out coverage.html
	rm -rf $(BUILD_DIR)

# Install to system
install: build-prod
	@echo "Installing..."
	sudo bash scripts/install.sh

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Generate API documentation
docs:
	@echo "Generating API documentation..."
	swag init -g cmd/api/main.go

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  build-prod   - Build for production"
	@echo "  run          - Build and run"
	@echo "  dev          - Run in development mode"
	@echo "  deps         - Download dependencies"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  clean        - Clean build artifacts"
	@echo "  install      - Install to system"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
