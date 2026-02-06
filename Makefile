.PHONY: build clean install help test

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
