# CMake configuration for libsensors
cmake_minimum_required(VERSION 3.16.0...3.30)

# Add static library for libsensors
add_library(vendored_libsensors STATIC
        vendored/lib/access.c
        vendored/lib/data.c
        vendored/lib/error.c
        vendored/lib/general.c
        vendored/lib/init.c
        vendored/lib/sysfs.c
)

# Add Flex and Bison dependencies
find_package(FLEX REQUIRED)
find_package(BISON REQUIRED)

# Generate Bison parser
bison_target(LibsensorsParser vendored/lib/conf-parse.y ${CMAKE_CURRENT_BINARY_DIR}/conf-parse.c
        COMPILE_FLAGS "-p sensors_yy --defines=${CMAKE_CURRENT_BINARY_DIR}/conf-parse.h"
)

# Generate Flex scanner
flex_target(LibsensorsLexer vendored/lib/conf-lex.l ${CMAKE_CURRENT_BINARY_DIR}/conf-lex.c
        COMPILE_FLAGS "-Psensors_yy -Cfe -8"
)

# Add dependency to ensure correct build order
add_flex_bison_dependency(LibsensorsLexer LibsensorsParser)

# Include generated files
target_sources(vendored_libsensors PRIVATE
        ${BISON_LibsensorsParser_OUTPUTS}
        ${FLEX_LibsensorsLexer_OUTPUTS}
)

# Include paths
target_include_directories(vendored_libsensors PUBLIC
        vendored/lib
        ${CMAKE_CURRENT_BINARY_DIR} # For generated conf-parse.h
)

# Compile definitions
target_compile_definitions(vendored_libsensors PRIVATE
        ETCDIR="/etc"       # Define ETCDIR
)

# Suppress warnings for Flex and Bison files
set_source_files_properties(${FLEX_LibsensorsLexer_OUTPUTS} PROPERTIES
        COMPILE_FLAGS "-Wno-shadow -Wno-undef -Wno-unused -Wno-missing-prototypes -Wno-sign-compare"
)
set_source_files_properties(${BISON_LibsensorsParser_OUTPUTS} PROPERTIES
        COMPILE_FLAGS "-DYYLTYPE_IS_DECLARED=1"
)

# Link against required libraries
target_link_libraries(vendored_libsensors PRIVATE m)

# Additional compile options to match Makefile
target_compile_options(vendored_libsensors PRIVATE
        -fPIC -D_REENTRANT -Wall -O2 -Wstrict-prototypes -Wshadow
        -Wpointer-arith -Wcast-qual -Wcast-align -Wwrite-strings
        -Wnested-externs -Winline -Wmissing-prototypes -Wno-format-truncation
)
