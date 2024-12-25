# ./tools/CMakeLists.txt
cmake_minimum_required(VERSION 3.20)

# Function to add a Go tool
function(add_go_tool TOOL_NAME TOOL_PATH)
    add_custom_target(${TOOL_NAME}
        COMMAND ${GO_EXECUTABLE} build -o ${BINARY_DIR}/${TOOL_NAME} ${TOOL_PATH}
        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
        COMMENT "Building ${TOOL_NAME}"
    )
endfunction()

# Add tools here
add_go_tool(dbmigrate ./tools/dbmigrate)
add_go_tool(testdata ./tools/testdata)

# Aggregate target for all tools
add_custom_target(tools
    DEPENDS dbmigrate testdata
    COMMENT "Building all tools"
)
