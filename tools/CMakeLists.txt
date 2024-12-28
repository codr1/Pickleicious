# ./tools/CMakeLists.txt
cmake_minimum_required(VERSION 3.20)

# Find required tools
find_program(GO_EXECUTABLE go REQUIRED)
find_program(SQLC_EXECUTABLE sqlc)

if(NOT SQLC_EXECUTABLE)
    message(WARNING "sqlc not found. Please install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest")
    message(FATAL_ERROR "sqlc is required for database code generation")
endif()

message(STATUS "Found Go: ${GO_EXECUTABLE}")
message(STATUS "Found SQLC: ${SQLC_EXECUTABLE}")

# Function to add a Go tool
function(add_go_tool TOOL_NAME TOOL_PATH)
    add_custom_target(${TOOL_NAME}
        COMMAND ${GO_EXECUTABLE} build -o ${BINARY_DIR}/${TOOL_NAME} ${TOOL_PATH}
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "Building ${TOOL_NAME}"
    )
endfunction()

# Add tools here
add_go_tool(dbmigrate ./cmd/tools/dbmigrate)
add_go_tool(testdata ./tools/testdata)

# SQLC generation target
add_custom_target(generate_sqlc
    COMMAND sqlc generate
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}/internal/db
    COMMENT "Generating SQLC code"
)

# Database migration targets
add_custom_target(db_migrate_up
    COMMAND ${BINARY_DIR}/dbmigrate 
        -db ${CMAKE_BINARY_DIR}/data/pickleicious.db 
        -migrations ${CMAKE_SOURCE_DIR}/internal/db/migrations 
        -command up
    DEPENDS dbmigrate
    COMMENT "Running database migrations up"
)

add_custom_target(db_migrate_down
    COMMAND ${BINARY_DIR}/dbmigrate 
        -db ${CMAKE_BINARY_DIR}/data/pickleicious.db 
        -migrations ${CMAKE_SOURCE_DIR}/internal/db/migrations 
        -command down
    DEPENDS dbmigrate
    COMMENT "Running database migrations down"
)

# Database reset target
add_custom_target(db_reset
    COMMAND ${CMAKE_COMMAND} -E remove -f ${CMAKE_BINARY_DIR}/data/pickleicious.db
    COMMAND ${CMAKE_COMMAND} --build ${CMAKE_BINARY_DIR} --target db_migrate_up
    COMMENT "Resetting database"
)


# Aggregate target for all tools
add_custom_target(tools
    DEPENDS 
        dbmigrate
        generate_sqlc
        testdata
    COMMENT "Building all tools"
)
