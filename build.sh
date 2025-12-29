#!/bin/bash

# build.sh - Go build script for Pickleicious

set -euo pipefail

# Default values
BUILD_TYPE="Debug"
FORCE_REBUILD=false
CLEAN=false
TARGET="all"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

BIN_DIR="bin"
WEB_DIR="web"

generate_code() {
    echo -e "${BLUE}Generating templ and sqlc code...${NC}"
    templ generate
    (cd internal/db && sqlc generate)
}

build_css() {
    echo -e "${BLUE}Building CSS...${NC}"
    mkdir -p "${WEB_DIR}/static/css"
    (cd "${WEB_DIR}" && npx tailwindcss -i ./styles/input.css -o ./static/css/main.css)
}

# Function to print usage
print_usage() {
    echo "Usage: $0 [options] [target]"
    echo "Options:"
    echo "  -h, --help          Show this help message"
    echo "  -c, --clean         Clean before building"
    echo "  -f, --force         Force rebuild all targets"
    echo "  -t, --type TYPE     Build type (Debug|Release)"
    echo ""
    echo "Targets:"
    echo "  all                 Build everything (default)"
    echo "  server              Build only server"
}

# Function to build server
build_server() {
    local ldflags=""
    local build_flags=()

    if [ "$BUILD_TYPE" = "Release" ]; then
        ldflags="-s -w"
    fi

    if [ "$FORCE_REBUILD" = true ]; then
        build_flags+=("-a")
    fi

    if [ -n "$ldflags" ]; then
        build_flags+=("-ldflags" "$ldflags")
    fi

    generate_code
    build_css
    echo -e "${BLUE}Building server...${NC}"
    go build "${build_flags[@]}" -o "${BIN_DIR}/server" ./cmd/server
    echo -e "${GREEN}Successfully built server${NC}"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            print_usage
            exit 0
            ;;
        -c|--clean)
            CLEAN=true
            shift
            ;;
        -f|--force)
            FORCE_REBUILD=true
            shift
            ;;
        -t|--type)
            BUILD_TYPE=$2
            shift 2
            ;;
        *)
            TARGET=$1
            shift
            ;;
    esac
done

# Ensure we're in the project root
if [ ! -f "go.mod" ]; then
    echo -e "${RED}Error: Must be run from project root${NC}"
    exit 1
fi

# Clean if requested
if [ "$CLEAN" = true ]; then
    echo -e "${YELLOW}Cleaning build artifacts...${NC}"
    rm -rf "${BIN_DIR}"
fi

# Create bin directory if needed
mkdir -p "${BIN_DIR}"

# Build based on target
case $TARGET in
    "all")
        build_server
        ;;
    "server")
        build_server
        ;;
    *)
        echo -e "${RED}Unknown target: ${TARGET}${NC}"
        exit 1
        ;;
esac

echo -e "${GREEN}Build complete!${NC}"
