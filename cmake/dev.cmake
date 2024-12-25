# ./cmake/env/dev.cmake
set(APP_NAME "Pickleicious-Dev")
set(APP_PORT "8080")
set(DB_PATH ":memory:")
set(LOG_LEVEL "debug")
set(ENABLE_PROFILING "true")
set(ENABLE_DEBUG_TOOLS "true")

# ./cmake/env/staging.cmake
set(APP_NAME "Pickleicious-Staging")
set(APP_PORT "8080")
set(DB_PATH "/data/pickleicious.db")
set(LOG_LEVEL "info")
set(ENABLE_PROFILING "true")
set(ENABLE_DEBUG_TOOLS "false")

# ./cmake/env/prod.cmake
set(APP_NAME "Pickleicious")
set(APP_PORT "8080")
set(DB_PATH "/data/pickleicious.db")
set(LOG_LEVEL "warn")
set(ENABLE_PROFILING "false")
set(ENABLE_DEBUG_TOOLS "false")
