# CMAKE generated file: DO NOT EDIT!
# Generated by "Unix Makefiles" Generator, CMake Version 3.30

# Delete rule output on recipe failure.
.DELETE_ON_ERROR:

#=============================================================================
# Special targets provided by cmake.

# Disable implicit rules so canonical targets will work.
.SUFFIXES:

# Disable VCS-based implicit rules.
% : %,v

# Disable VCS-based implicit rules.
% : RCS/%

# Disable VCS-based implicit rules.
% : RCS/%,v

# Disable VCS-based implicit rules.
% : SCCS/s.%

# Disable VCS-based implicit rules.
% : s.%

.SUFFIXES: .hpux_make_needs_suffix_list

# Command-line flag to silence nested $(MAKE).
$(VERBOSE)MAKESILENT = -s

#Suppress display of executed commands.
$(VERBOSE).SILENT:

# A target that is always out of date.
cmake_force:
.PHONY : cmake_force

#=============================================================================
# Set environment variables for the build.

# The shell in which to execute make rules.
SHELL = /bin/sh

# The CMake executable.
CMAKE_COMMAND = /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake

# The command to remove a file.
RM = /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake -E rm -f

# Escaping for special characters.
EQUALS = =

# The top-level source directory on which CMake was run.
CMAKE_SOURCE_DIR = /Users/vess/dev/Pickleicious

# The top-level build directory on which CMake was run.
CMAKE_BINARY_DIR = /Users/vess/dev/Pickleicious/build

# Utility rule file for server.

# Include any custom commands dependencies for this target.
include CMakeFiles/server.dir/compiler_depend.make

# Include the progress variables for this target.
include CMakeFiles/server.dir/progress.make

CMakeFiles/server:
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --blue --bold --progress-dir=/Users/vess/dev/Pickleicious/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building server binary for dev environment"
	cd /Users/vess/dev/Pickleicious && /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake -E env APP_ENV=dev CONFIG_PATH=/Users/vess/dev/Pickleicious/build/bin/config/app.yaml /opt/homebrew/bin/go build -o /Users/vess/dev/Pickleicious/build/bin/server ./cmd/server

server: CMakeFiles/server
server: CMakeFiles/server.dir/build.make
.PHONY : server

# Rule to build all files generated by this target.
CMakeFiles/server.dir/build: server
.PHONY : CMakeFiles/server.dir/build

CMakeFiles/server.dir/clean:
	$(CMAKE_COMMAND) -P CMakeFiles/server.dir/cmake_clean.cmake
.PHONY : CMakeFiles/server.dir/clean

CMakeFiles/server.dir/depend:
	cd /Users/vess/dev/Pickleicious/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /Users/vess/dev/Pickleicious /Users/vess/dev/Pickleicious /Users/vess/dev/Pickleicious/build /Users/vess/dev/Pickleicious/build /Users/vess/dev/Pickleicious/build/CMakeFiles/server.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : CMakeFiles/server.dir/depend
