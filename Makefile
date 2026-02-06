.PHONY: build clean install help test

help:
	@echo "Vorzela Migration Tool - Commands"
	@echo "=================================="
	@echo ""
	@echo "Build and Install:"
	@echo "  make build       - Build the vc binary"
	@echo "  make install     - Build and install vc to /usr/local/bin"
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
	@echo "Building vc binary..."
	@go mod tidy
	@go build -o vc main.go
	@echo "✓ Build successful! Binary: ./vc"

install: build
	@echo "Installing vc to /usr/local/bin..."
	@sudo cp vc /usr/local/bin/vc
	@sudo chmod +x /usr/local/bin/vc
	@echo "✓ Installation successful! You can now use 'vc' from anywhere"

clean:
	@echo "Cleaning up..."
	@rm -f vc
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
