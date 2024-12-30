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
            tailwindcss 
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
    DEPENDS generate_templ tailwind
    COMMENT "Building server binary for ${ENV} environment"
)

# Development server
add_custom_target(dev
    COMMAND ${CMAKE_COMMAND} -E env 
        APP_ENV=${ENV}
        CONFIG_PATH=${CONFIG_DIR}/app.yaml
        ${GO_EXECUTABLE} run ./cmd/server/...
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ tailwind
    COMMENT "Running development server in ${ENV} environment"
)

# Watch mode targets
add_custom_target(watch_templ
    COMMAND templ generate --watch
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    COMMENT "Watching and generating Templ templates"
)

add_custom_target(watch_tailwind
    COMMAND tailwindcss 
        -i ./web/styles/main.css 
        -o ${BINARY_DIR}/static/css/main.css 
        --watch
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    COMMENT "Watching and building Tailwind CSS"
)

# Development target that runs everything in parallel
add_custom_target(dev_watch
    COMMAND ${CMAKE_COMMAND} -E env 
        APP_ENV=${ENV}
        CONFIG_PATH=${CONFIG_DIR}/app.yaml
        ${CMAKE_COMMAND} -E copy_directory 
        ${CMAKE_SOURCE_DIR}/web/static 
        ${BINARY_DIR}/static
    COMMAND ${CMAKE_COMMAND} -E echo "Starting development server with file watching..."
    COMMAND ${CMAKE_COMMAND} -E echo "Use Ctrl+C to stop all processes"
    COMMAND ${CMAKE_SOURCE_DIR}/scripts/dev_watch.sh
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS generate_templ tailwind
    COMMENT "Running development server with file watching"
)
