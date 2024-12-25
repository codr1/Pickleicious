# Makefile

# Configuration
BUILD_DIR := build
ENV ?= dev
BUILD_TYPE ?= Debug

# Colors for output
GREEN  := $(shell tput setaf 2)
YELLOW := $(shell tput setaf 3)
RESET  := $(shell tput sgr0)

.PHONY: all build clean dev test tools help

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

# Help target
help:
	@echo "Available targets:"
	@echo "  make              - Build the server (default)"
	@echo "  make dev          - Run the development server"
	@echo "  make test         - Run tests"
	@echo "  make tools        - Build development tools"
	@echo "  make clean        - Clean build artifacts"
	@echo ""
	@echo "Configuration:"
	@echo "  make ENV=prod     - Build for production"
	@echo "  make ENV=staging  - Build for staging"
	@echo "  make ENV=dev      - Build for development (default)"
	@echo ""
	@echo "Example usage:"
	@echo "  make dev ENV=staging    - Run development server with staging config"
