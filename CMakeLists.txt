# ./CMakeLists.txt (updated)
cmake_minimum_required(VERSION 3.20)
project(Pickleicious LANGUAGES NONE)

# Environment Configuration
set(ENV_OPTIONS "dev" "staging" "prod")
if(NOT DEFINED ENV)
    set(ENV "dev")
endif()

if(NOT ENV IN_LIST ENV_OPTIONS)
    message(FATAL_ERROR "ENV must be one of: ${ENV_OPTIONS}")
endif()

if(ENV STREQUAL "prod")
    add_definitions(-DPRODUCTION_BUILD)
    set(TAILWIND_FLAGS "--minify")
    # Other production-specific settings
else()
    set(TAILWIND_FLAGS "")
    # Development defaults
endif()

# Load environment-specific configuration
include(cmake/env/${ENV}.cmake)

# Global settings
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)
set(CMAKE_BUILD_TYPE ${CMAKE_BUILD_TYPE})

# Output directories
set(BINARY_DIR ${CMAKE_BINARY_DIR}/bin)
set(GENERATED_DIR ${CMAKE_BINARY_DIR}/generated)
set(CONFIG_DIR ${BINARY_DIR}/config)
set(PROJECT_DB_DIR ${CMAKE_BINARY_DIR}/db)

# Create necessary directories
file(MAKE_DIRECTORY ${BINARY_DIR})
file(MAKE_DIRECTORY ${GENERATED_DIR})
file(MAKE_DIRECTORY ${CONFIG_DIR})
file(MAKE_DIRECTORY ${PROJECT_DB_DIR})

# Configure environment files
configure_file(
    ${CMAKE_SOURCE_DIR}/config/app.yaml.in
    ${CONFIG_DIR}/app.yaml
    @ONLY
)

# Find required tools
find_program(GO_EXECUTABLE go REQUIRED)
message(STATUS "Found Go: ${GO_EXECUTABLE}")

# Include subdirectories
add_subdirectory(tools)

# Main targets (updated with environment)
add_custom_target(generate_templ
    COMMAND templ generate
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    COMMENT "Generating Templ templates"
)

add_custom_target(tailwind
    COMMAND cd ${CMAKE_SOURCE_DIR}/web &&
            npx tailwindcss
            -i ./styles/input.css
            -o ${BINARY_DIR}/static/css/main.css
            $<$<CONFIG:Release>:--minify>
    COMMENT "Building Tailwind CSS"
)

# Server with environment configuration
add_custom_target(server
    COMMAND ${CMAKE_COMMAND} -E env 
        APP_ENV=${ENV}
        CONFIG_PATH=${CONFIG_DIR}/app.yaml
        ${GO_EXECUTABLE} build -o ${BINARY_DIR}/server ./cmd/server
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ tailwind static_assets
    COMMENT "Building server binary for ${ENV} environment"
)

# Development server
add_custom_target(dev
    COMMAND ${CMAKE_COMMAND} -E env 
        APP_ENV=${ENV}
        CONFIG_PATH=${CONFIG_DIR}/app.yaml
        ${GO_EXECUTABLE} run ./cmd/server/...
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ tailwind static_assets db_migrate_up generate_sqlc
    COMMENT "Running development server in ${ENV} environment"
)

# Development target that runs everything in parallel
add_custom_target(dev_watch
    COMMAND ${CMAKE_COMMAND} -E env 
        APP_ENV=${ENV}
        CONFIG_PATH=${CONFIG_DIR}/app.yaml
        ${CMAKE_COMMAND} -E make_directory ${BINARY_DIR}/static/css
    COMMAND ${CMAKE_COMMAND} -E copy_directory 
        ${CMAKE_SOURCE_DIR}/web/static
        ${BINARY_DIR}/static
    # Start Tailwind watcher
    COMMAND cd ${CMAKE_SOURCE_DIR}/web && npx tailwindcss
        -i ./styles/input.css
        -o ${BINARY_DIR}/static/css/main.css
        --watch &
    # Start Templ watcher
    COMMAND templ generate --watch &
    # Start Air for Go hot reloading
    COMMAND ${GO_EXECUTABLE} run github.com/air-verse/air@latest -c ${CMAKE_SOURCE_DIR}/.air.toml
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ tailwind static_assets db_migrate_up generate_sqlc
    COMMENT "Running development server with file watching"
)

# Add this after the existing directory creation
add_custom_target(static_assets
    COMMAND ${CMAKE_COMMAND} -E make_directory ${BINARY_DIR}/static
    COMMAND ${CMAKE_COMMAND} -E copy_directory
        ${CMAKE_SOURCE_DIR}/web/static
        ${BINARY_DIR}/static
    COMMENT "Copying static assets"
)

add_custom_target(test
    COMMAND ${GO_EXECUTABLE} test ./...
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ generate_sqlc
    COMMENT "Running Go tests"
)
