# Makefile

# Configuration
BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/server
DB_DIR := build/db
DB_PATH ?= $(DB_DIR)/pickleicious.db
MIGRATIONS_DIR := ./internal/db/migrations

# Colors for output
GREEN  := $(shell tput setaf 2)
YELLOW := $(shell tput setaf 3)
RESET  := $(shell tput sgr0)

# Add to top of Makefile
REQUIRED_TOOLS := air templ tailwindcss sqlc

.PHONY: install-tools
install-tools:
	@echo "$(GREEN)Installing required development tools...$(RESET)"
	@go install github.com/air-verse/air@latest
	@go install github.com/a-h/templ/cmd/templ@latest
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@if ! command -v tailwindcss >/dev/null 2>&1; then \
		echo "Installing tailwindcss..."; \
		npm install -g tailwindcss; \
	fi

.PHONY: all build build-dev build-staging build-prod clean dev dev-watch test help db-setup db-migrate db-reset generate-sqlc static-assets templates css generate

# Default target
all: build

# Generate templ and sqlc code
generate:
	@echo "$(GREEN)Generating templ and sqlc code...$(RESET)"
	@templ generate
	@cd internal/db && sqlc generate

# Build CSS assets
css:
	@echo "$(GREEN)Building CSS...$(RESET)"
	@mkdir -p web/static/css
	@cd web && npx tailwindcss -i ./styles/input.css -o ./static/css/main.css

# Build the server
build: generate css
	@echo "$(GREEN)Building server...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -o $(SERVER_BIN) ./cmd/server

# Build variants
build-dev: generate css
	@echo "$(GREEN)Building dev server...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -tags dev -o $(SERVER_BIN) ./cmd/server

build-staging: generate css
	@echo "$(GREEN)Building staging server...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -tags staging -o $(SERVER_BIN) ./cmd/server

build-prod: generate css
	@echo "$(GREEN)Building prod server...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@go build -tags prod -ldflags "-s -w" -o $(SERVER_BIN) ./cmd/server

# Run the development server
dev: generate css
	@echo "$(GREEN)Starting development server...$(RESET)"
	@go run -tags dev ./cmd/server

# Run the development server with live reload
dev-watch: generate css
	@echo "$(GREEN)Starting development server with live reload...$(RESET)"
	@air -c .air.toml

# Clean build artifacts
clean:
	@echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	@rm -rf $(BIN_DIR)

# Run tests
test:
	@echo "$(GREEN)Running tests...$(RESET)"
	@go test ./...

# Generate templates only
templates:
	@echo "$(GREEN)Generating templates...$(RESET)"
	@templ generate

# Creates database and runs all migrations
db-setup:
	@echo "${GREEN}Setting up database...${RESET}"
	@$(MAKE) db-migrate-up

# Runs any pending migrations
db-migrate:
	@echo "${GREEN}Running database migrations...${RESET}"
	@$(MAKE) db-migrate-up

# Wipes database and runs all migrations fresh
db-reset:
	@echo "${GREEN}Resetting database...${RESET}"
	@rm -f "$(DB_PATH)"
	@mkdir -p "$$(dirname "$(DB_PATH)")"
	@$(MAKE) db-migrate-up

# Generates Go code from SQL queries using sqlc
generate-sqlc:
	@echo "${GREEN}Generating SQLC code...${RESET}"
	@cd internal/db && sqlc generate

# Run database migrations using the Go tool
.PHONY: db-migrate-up
db-migrate-up:
	@echo "${GREEN}Running database migrations...${RESET}"
	@echo "DB_PATH: $(DB_PATH)"
	@echo "MIGRATIONS_DIR: $(MIGRATIONS_DIR)"
	@go run cmd/tools/dbmigrate/main.go \
		-command up \
		-db $(DB_PATH) \
		-migrations $(MIGRATIONS_DIR)

# Copy static assets
static-assets:
	@echo "$(GREEN)Copying static assets...$(RESET)"
	@mkdir -p $(BIN_DIR)/static
	@cp -R web/static/* $(BIN_DIR)/static/

# Help target
help:
	@echo "Available targets:"
	@echo "  make               - Build the server (default)"
	@echo "  make build-dev      - Build the server for development"
	@echo "  make build-staging  - Build the server for staging"
	@echo "  make build-prod     - Build the server for production"
	@echo "  make dev            - Run the development server (no file watching)"
	@echo "  make dev-watch      - Run the development server with live reload"
	@echo "  make test           - Run tests"
	@echo "  make db-setup       - Creates database and runs all migrations" 
	@echo "  make db-migrate     - Runs any pending migrations"
	@echo "  make db-reset       - Wipes database and runs all migrations fresh"
	@echo "  make generate-sqlc  - Generates Go code from SQL queries using sqlc"
	@echo "  make clean          - Clean build artifacts"
	@echo ""
	@echo "Example usage:"
	@echo "  make dev"
