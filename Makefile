.PHONY: build clean install help test dist

help:
	@echo "Vorzela Migration Tool - Commands"
	@echo "=================================="
	@echo ""
	@echo "Build and Install:"
	@echo "  make build       - Build the vm binary"
	@echo "  make install     - Build and install vm to /usr/local/bin"
	@echo "  make clean       - Remove built binary"
	@echo ""
	@echo "Development:"
	@echo "  make dev-migration - Create a dev migration (usage: make dev-migration NAME=create_my_table)"
	@echo "  make server-migration - Create a server migration (usage: make server-migration NAME=create_my_table)"
	@echo ""
	@echo "Testing:"
	@echo "  make fmt         - Format code"
	@echo "  make lint        - Run golangci-lint"
	@echo ""

build:
	@echo "Building vm binary..."
	@go mod tidy
	@go build -o vm main.go
	@echo "✓ Build successful! Binary: ./vm"

install: build
	@echo "Installing vm to /usr/local/bin..."
	@sudo cp vm /usr/local/bin/vm
	@sudo chmod +x /usr/local/bin/vm
	@echo "✓ Installation successful! You can now use 'vm' from anywhere"

clean:
	@echo "Cleaning up..."
	@rm -f vm
	@echo "✓ Cleaned"

# Build prebuilt binaries for common platforms and place them in dist/
# Usage: make dist VERSION=v1.0.3
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
DIST_DIR := dist

.PHONY: dist
dist: clean
	@echo "Creating $(DIST_DIR) and building prebuilt binaries (VERSION=$(VERSION))"
	@rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@echo "Building linux/amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=$(VERSION)'" -o $(DIST_DIR)/vm-linux-amd64 main.go || { echo "linux build failed"; exit 1; }
	@echo "Building darwin/amd64 (may fail on non-macos hosts)..."
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=$(VERSION)'" -o $(DIST_DIR)/vm-macos-amd64 main.go || echo "darwin/amd64 build skipped"
	@echo "Building darwin/arm64 (may fail on non-macos hosts)..."
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=$(VERSION)'" -o $(DIST_DIR)/vm-macos-arm64 main.go || echo "darwin/arm64 build skipped"
	@echo "✓ Dist build complete. Files in $(DIST_DIR):" && ls -lh $(DIST_DIR)

dev-migration:
	@if [ -z "$(NAME)" ]; then echo "Error: NAME is required. Usage: make dev-migration NAME=create_my_table"; exit 1; fi
	@./vc make migration $(NAME) -e dev

server-migration:
	@if [ -z "$(NAME)" ]; then echo "Error: NAME is required. Usage: make server-migration NAME=create_my_table"; exit 1; fi
	@./vc make migration $(NAME) -e server

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Code formatted"

lint:
	@echo "Running linter..."
	@golangci-lint run ./...
