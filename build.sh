#!/bin/bash

# build.sh - Intelligent build script for Pickleicious

# Default values
BUILD_TYPE="Debug"
FORCE_REBUILD=false
CLEAN=false
TARGET="all"
VERBOSE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print usage
print_usage() {
    echo "Usage: $0 [options] [target]"
    echo "Options:"
    echo "  -h, --help          Show this help message"
    echo "  -c, --clean         Clean before building"
    echo "  -f, --force         Force rebuild all targets"
    echo "  -v, --verbose       Verbose output"
    echo "  -t, --type TYPE     Build type (Debug|Release)"
    echo ""
    echo "Targets:"
    echo "  all                 Build everything (default)"
    echo "  tools               Build only tools"
    echo "  server              Build only server"
    echo "  <tool-name>         Build specific tool"
}

# Function to check if rebuild is needed
needs_rebuild() {
    local target=$1
    local source_dir=$2
    local binary="${PWD}/bin/${target}"

    # If binary doesn't exist, rebuild needed
    if [ ! -f "$binary" ]; then
        return 0
    }

    # If force rebuild is set, rebuild needed
    if [ "$FORCE_REBUILD" = true ]; then
        return 0
    }

    # Check if any source files are newer than binary
    if [ -n "$(find "$source_dir" -type f -newer "$binary" -name '*.go')" ]; then
        return 0
    }

    return 1
}

# Function to ensure build directory exists
ensure_build_dir() {
    if [ ! -d "build" ]; then
        mkdir build
    }
}

# Function to run CMake
run_cmake() {
    local cmake_args="-DCMAKE_BUILD_TYPE=${BUILD_TYPE}"
    
    if [ "$VERBOSE" = true ]; then
        cmake_args="$cmake_args -DCMAKE_VERBOSE_MAKEFILE=ON"
    fi

    if [ "$CLEAN" = true ]; then
        echo -e "${BLUE}Cleaning build directory...${NC}"
        rm -rf build/*
    fi

    echo -e "${BLUE}Configuring CMake...${NC}"
    cmake -B build $cmake_args

    if [ $? -ne 0 ]; then
        echo -e "${RED}CMake configuration failed${NC}"
        exit 1
    fi
}

# Function to build specific target
build_target() {
    local target=$1
    local source_dir=$2

    if needs_rebuild "$target" "$source_dir"; then
        echo -e "${BLUE}Building ${target}...${NC}"
        cmake --build build --target "$target"
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}Successfully built ${target}${NC}"
        else
            echo -e "${RED}Failed to build ${target}${NC}"
            exit 1
        fi
    else
        echo -e "${GREEN}${target} is up to date${NC}"
    fi
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
        -v|--verbose)
            VERBOSE=true
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
if [ ! -f "CMakeLists.txt" ]; then
    echo -e "${RED}Error: Must be run from project root${NC}"
    exit 1
fi

# Create build directory if needed
ensure_build_dir

# Run CMake configuration
run_cmake

# Build based on target
case $TARGET in
    "all")
        echo -e "${BLUE}Building all targets...${NC}"
        cmake --build build
        ;;
    "tools")
        echo -e "${BLUE}Building all tools...${NC}"
        cmake --build build --target tools
        ;;
    "server")
        build_target "server" "cmd/server"
        ;;
    *)
        if [ -d "tools/$TARGET" ]; then
            build_target "$TARGET" "tools/$TARGET"
        else
            echo -e "${RED}Unknown target: $TARGET${NC}"
            exit 1
        fi
        ;;
esac

echo -e "${GREEN}Build complete!${NC}"
