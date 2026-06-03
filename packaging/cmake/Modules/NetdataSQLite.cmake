# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of SQLite
#
# Handle bundling of SQLite.
#
# This fetches the SQLite source tree (via tarball or git clone),
# generates the amalgamation at build time (with --enable-update-limit
# so the parser supports UPDATE/DELETE ... LIMIT), then compiles it as
# a separate static library that links into libnetdata.

include(ExternalProject)

set(SQLITE_VERSION "3.50.4")
set(SQLITE_VERSION_NUMBER "3500400")
set(SQLITE_VERSION_YEAR "2025")

set(SQLITE_TARBALL_SHA256 "b7b4dc060f36053902fb65b344bbbed592e64b2291a26ac06fe77eec097850e9")
set(SQLITE_GIT_SHA "8ed5e7365e6f12f427910188bbf6b254daad2ef6") # tag is: version-${SQLITE_VERSION}

option(SQLITE_USE_GIT "Fetch SQLite sources via git clone instead of tarball" OFF)

function(netdata_bundle_sqlite3)
        message(STATUS "Preparing SQLite ${SQLITE_VERSION}")

        set(sqlite_SOURCE_DIR "${CMAKE_BINARY_DIR}/sqlite-src")
        set(sqlite_BINARY_DIR "${CMAKE_BINARY_DIR}/sqlite-build")
        set(sqlite_OUTPUT_DIR "${CMAKE_BINARY_DIR}/sqlite-output")

        file(MAKE_DIRECTORY "${sqlite_OUTPUT_DIR}")

        if(SQLITE_USE_GIT)
                message(STATUS "Fetching SQLite via git clone")
                set(SQLITE_FETCH_ARGS
                        GIT_REPOSITORY https://github.com/sqlite/sqlite.git
                        GIT_TAG "${SQLITE_GIT_SHA}"
                )
        else()
                message(STATUS "Fetching SQLite via tarball")
                set(SQLITE_FETCH_ARGS
                        URL "https://www.sqlite.org/${SQLITE_VERSION_YEAR}/sqlite-src-${SQLITE_VERSION_NUMBER}.zip"
                        URL_HASH "SHA256=${SQLITE_TARBALL_SHA256}"
                )
        endif()

        ExternalProject_Add(
                sqlite_project
                ${SQLITE_FETCH_ARGS}
                SOURCE_DIR "${sqlite_SOURCE_DIR}"
                BINARY_DIR "${sqlite_BINARY_DIR}"
                CONFIGURE_COMMAND "${sqlite_SOURCE_DIR}/configure" --enable-update-limit
                BUILD_COMMAND make sqlite3.c sqlite3.h
                INSTALL_COMMAND ${CMAKE_COMMAND} -E copy
                        "${sqlite_BINARY_DIR}/sqlite3.c"
                        "${sqlite_BINARY_DIR}/sqlite3.h"
                        "${sqlite_SOURCE_DIR}/ext/recover/sqlite3recover.c"
                        "${sqlite_SOURCE_DIR}/ext/recover/sqlite3recover.h"
                        "${sqlite_SOURCE_DIR}/ext/recover/dbdata.c"
                        "${sqlite_OUTPUT_DIR}"
                BUILD_BYPRODUCTS
                        "${sqlite_OUTPUT_DIR}/sqlite3.c"
                        "${sqlite_OUTPUT_DIR}/sqlite3.h"
                        "${sqlite_OUTPUT_DIR}/sqlite3recover.c"
                        "${sqlite_OUTPUT_DIR}/sqlite3recover.h"
                        "${sqlite_OUTPUT_DIR}/dbdata.c"
                EXCLUDE_FROM_ALL 1
                UPDATE_DISCONNECTED ON
        )

        set(SQLITE_SOURCES
                "${sqlite_OUTPUT_DIR}/sqlite3.c"
                "${sqlite_OUTPUT_DIR}/sqlite3recover.c"
                "${sqlite_OUTPUT_DIR}/dbdata.c"
        )

        set_source_files_properties(${SQLITE_SOURCES} PROPERTIES GENERATED TRUE)

        add_library(sqlite3 STATIC ${SQLITE_SOURCES})

        target_compile_definitions(sqlite3 PRIVATE
                SQLITE_ENABLE_UPDATE_DELETE_LIMIT
                SQLITE_ENABLE_MEMORY_MANAGEMENT
                SQLITE_OMIT_LOAD_EXTENSION
                SQLITE_ENABLE_DBSTAT_VTAB
                SQLITE_ENABLE_DBPAGE_VTAB
        )

        target_compile_options(sqlite3 PRIVATE -Wno-unused-parameter)

        target_include_directories(sqlite3 PUBLIC "${sqlite_OUTPUT_DIR}")

        add_dependencies(sqlite3 sqlite_project)

        # Export variables to parent scope
        set(NETDATA_SQLITE_INCLUDE_DIRS "${sqlite_OUTPUT_DIR}" PARENT_SCOPE)
        set(NETDATA_SQLITE_LIBRARIES sqlite3 PARENT_SCOPE)

        message(STATUS "Finished preparing SQLite ${SQLITE_VERSION}")
endfunction()

function(netdata_add_sqlite3_to_target _target)
        target_include_directories(${_target} BEFORE PUBLIC "${NETDATA_SQLITE_INCLUDE_DIRS}")
        target_link_libraries(${_target} PUBLIC ${NETDATA_SQLITE_LIBRARIES})
        add_dependencies(${_target} sqlite3)
endfunction()
