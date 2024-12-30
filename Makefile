# Makefile

# Configuration
BUILD_DIR := build
ENV ?= dev
BUILD_TYPE ?= Debug

# Colors for output
GREEN  := $(shell tput setaf 2)
YELLOW := $(shell tput setaf 3)
RESET  := $(shell tput sgr0)


# TODO: START HERE !
.PHONY: all build clean dev test tools help db-setup db-migrate db-reset generate-sqlc

# Default target
all: build

# Ensure build directory exists and run CMake
$(BUILD_DIR)/Makefile:
	@echo "$(GREEN)Configuring CMake for $(ENV) environment...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	@cd $(BUILD_DIR) && cmake .. -DENV=$(ENV) -DCMAKE_BUILD_TYPE=$(BUILD_TYPE)

# Build the server
build: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Building server...$(RESET)"
	@cmake --build $(BUILD_DIR) --target server

# Run the development server
dev: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Starting development server...$(RESET)"
	@cmake --build $(BUILD_DIR) --target dev

# Clean build artifacts
clean:
	@echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	@cmake --build $(BUILD_DIR) --target clean_all 2>/dev/null || true
	@rm -rf $(BUILD_DIR)

# Run tests
test: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Running tests...$(RESET)"
	@cmake --build $(BUILD_DIR) --target test

# Build tools
tools: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Building tools...$(RESET)"
	@cmake --build $(BUILD_DIR) --target tools

# Development with file watching
watch: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Starting development server with file watching...$(RESET)"
	@cmake --build $(BUILD_DIR) --target dev_watch

# Generate templates only
templates: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Generating templates...$(RESET)"
	@cmake --build $(BUILD_DIR) --target generate_templ

# Build CSS only
css: $(BUILD_DIR)/Makefile
	@echo "$(GREEN)Building CSS...$(RESET)"
	@cmake --build $(BUILD_DIR) --target tailwind

# Creates database and runs all migrations
db-setup: $(BUILD_DIR)/Makefile
	@echo "${GREEN}Setting up database...${RESET}"
	@cmake --build $(BUILD_DIR) --target db_migrate_up

# Runs any pending migrations
db-migrate: $(BUILD_DIR)/Makefile
	@echo "${GREEN}Running database migrations...${RESET}"
	@cmake --build $(BUILD_DIR) --target db_migrate_up

# Wipes database and runs all migrations fresh
db-reset: $(BUILD_DIR)/Makefile
	@echo "${GREEN}Resetting database...${RESET}"
	@cmake --build $(BUILD_DIR) --target db_reset

# Generates Go code from SQL queries using sqlc
generate-sqlc: $(BUILD_DIR)/Makefile   # Generates type-safe DB code from SQL
	@echo "${GREEN}Generating SQLC code...${RESET}"
	@cmake --build $(BUILD_DIR) --target generate_sqlc

# Development server with database setup
.PHONY: dev-server
dev-server: $(BUILD_DIR)/Makefile db-setup generate-sqlc  # Runs with hot reload, debug logging, local SQLite
	@echo "${GREEN}Starting development server...${RESET}"
	@cmake --build $(BUILD_DIR) --target dev_watch

# Production server with database setup
.PHONY: prod
prod: $(BUILD_DIR)/Makefile db-migrate  # No hot reload, optimized, proper DB config, etc
	@echo "${GREEN}Starting production server...${RESET}"
	@ENV=prod cmake --build $(BUILD_DIR) --target server


# Help target
help:
	@echo "Available targets:"
	@echo "  make               - Build the server (default)"
	@echo "  make prod          - Run the production server"
	@echo "  make dev           - Run the development server"
	@echo "  make test          - Run tests"
	@echo "  make tools         - Build development tools"
	@echo "  make db-setup 	    - Creates database and runs all migrations" 
	@echo "  make db-migrate    - Runs any pending migrations"
	@echo "  make db-reset      - Wipes database and runs all migrations fresh"
	@echo "  make generate_sqlc - Generates Go code from SQL queries using sqlc"
	@echo "  make clean         - Clean build artifacts"
	@echo ""
	@echo "Configuration:"
	@echo "  make ENV=prod      - Build for production"
	@echo "  make ENV=staging   - Build for staging"
	@echo "  make ENV=dev      - Build for development (default)"
	@echo ""
	@echo "Example usage:"
	@echo "  make dev ENV=staging    - Run development server with staging config"
