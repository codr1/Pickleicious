# ./clean-all.cmake
# Remove build directory
file(REMOVE_RECURSE ${CMAKE_BINARY_DIR}/bin)
file(REMOVE_RECURSE ${CMAKE_BINARY_DIR}/generated)

# Remove generated template files
file(GLOB_RECURSE TEMPLATE_FILES "${CMAKE_SOURCE_DIR}/internal/**/*_templ.go")
foreach(TEMPLATE ${TEMPLATE_FILES})
    file(REMOVE ${TEMPLATE})
endforeach()

# Remove Go build cache
execute_process(
    COMMAND ${GO_EXECUTABLE} clean -cache -testcache
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
)
