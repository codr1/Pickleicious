# cmd/CMakeLists.txt

# Server binary
set(SERVER_NAME "pickleicious")
set(SERVER_SRC_DIR ${CMAKE_CURRENT_SOURCE_DIR}/server)

# Function to check if rebuild is needed based on source file changes
function(check_rebuild_needed target src_dir var)
    set(rebuild_needed TRUE)
    if(EXISTS ${CMAKE_BINARY_DIR}/${target})
        file(GLOB_RECURSE source_files 
            ${src_dir}/*.go
        )
        set(binary_time 0)
        file(TIMESTAMP ${CMAKE_BINARY_DIR}/${target} binary_time %s)
        
        set(newest_source 0)
        foreach(source ${source_files})
            file(TIMESTAMP ${source} source_time %s)
            if(source_time GREATER newest_source)
                set(newest_source ${source_time})
            endif()
        endforeach()
        
        if(binary_time GREATER newest_source)
            set(rebuild_needed FALSE)
        endif()
    endif()
    
    set(${var} ${rebuild_needed} PARENT_SCOPE)
endfunction()

# Main server target
check_rebuild_needed(${SERVER_NAME} ${SERVER_SRC_DIR} SHOULD_BUILD_SERVER)

if(SHOULD_BUILD_SERVER)
    build_go_binary(${SERVER_NAME} ${SERVER_SRC_DIR})
    
    # Add dependencies if needed
    # add_dependencies(${SERVER_NAME} other_target)
    
    # Custom clean target for server
    add_custom_target(clean_server
        COMMAND ${CMAKE_COMMAND} -E remove ${CMAKE_BINARY_DIR}/${SERVER_NAME}
        COMMENT "Cleaning server binary"
    )
endif()

# Development server target (with different flags/tags)
set(DEV_SERVER_NAME "${SERVER_NAME}_dev")
check_rebuild_needed(${DEV_SERVER_NAME} ${SERVER_SRC_DIR} SHOULD_BUILD_DEV)

if(SHOULD_BUILD_DEV)
    add_custom_target(${DEV_SERVER_NAME} ALL
        COMMAND go build -tags dev -o ${CMAKE_BINARY_DIR}/${DEV_SERVER_NAME}
        WORKING_DIRECTORY ${SERVER_SRC_DIR}
        COMMENT "Building development server"
        VERBATIM
    )
    
    # Custom clean target for dev server
    add_custom_target(clean_server_dev
        COMMAND ${CMAKE_COMMAND} -E remove ${CMAKE_BINARY_DIR}/${DEV_SERVER_NAME}
        COMMENT "Cleaning development server binary"
    )
endif()

# Add other binaries here as needed
# For example, CLI tools, migration utilities, etc.

# Custom target to clean all cmd binaries
add_custom_target(clean_cmd
    DEPENDS clean_server clean_server_dev
    COMMENT "Cleaning all command binaries"
)
