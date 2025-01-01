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

# Utility rule file for dev_watch.

# Include any custom commands dependencies for this target.
include CMakeFiles/dev_watch.dir/compiler_depend.make

# Include the progress variables for this target.
include CMakeFiles/dev_watch.dir/progress.make

CMakeFiles/dev_watch:
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --blue --bold --progress-dir=/Users/vess/dev/Pickleicious/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Running development server with file watching"
	cd /Users/vess/dev/Pickleicious && /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake -E env APP_ENV=dev CONFIG_PATH=/Users/vess/dev/Pickleicious/build/bin/config/app.yaml /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake -E make_directory /Users/vess/dev/Pickleicious/build/bin/static/css
	cd /Users/vess/dev/Pickleicious && /opt/homebrew/Cellar/cmake/3.30.2/bin/cmake -E copy_directory /Users/vess/dev/Pickleicious/web/static /Users/vess/dev/Pickleicious/build/bin/static
	cd /Users/vess/dev/Pickleicious && tailwindcss -i /Users/vess/dev/Pickleicious/web/styles/input.css -o /Users/vess/dev/Pickleicious/build/bin/static/css/main.css --watch &
	cd /Users/vess/dev/Pickleicious && templ generate --watch &
	cd /Users/vess/dev/Pickleicious && /opt/homebrew/bin/go run github.com/air-verse/air@latest -c /Users/vess/dev/Pickleicious/.air.toml

dev_watch: CMakeFiles/dev_watch
dev_watch: CMakeFiles/dev_watch.dir/build.make
.PHONY : dev_watch

# Rule to build all files generated by this target.
CMakeFiles/dev_watch.dir/build: dev_watch
.PHONY : CMakeFiles/dev_watch.dir/build

CMakeFiles/dev_watch.dir/clean:
	$(CMAKE_COMMAND) -P CMakeFiles/dev_watch.dir/cmake_clean.cmake
.PHONY : CMakeFiles/dev_watch.dir/clean

CMakeFiles/dev_watch.dir/depend:
	cd /Users/vess/dev/Pickleicious/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /Users/vess/dev/Pickleicious /Users/vess/dev/Pickleicious /Users/vess/dev/Pickleicious/build /Users/vess/dev/Pickleicious/build /Users/vess/dev/Pickleicious/build/CMakeFiles/dev_watch.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : CMakeFiles/dev_watch.dir/depend

