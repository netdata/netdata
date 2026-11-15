# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of SQLite
#
# Handle bundling of SQLite.
#
# This fetches the SQLite source tree (via tarball, git clone, or a local
# source directory), generates the amalgamation at build time (with
# --enable-update-limit so the parser supports UPDATE/DELETE ... LIMIT), then
# compiles it as a separate static library that links into libnetdata.
#
# With ENABLE_BUNDLED_SQLITE turned off none of that happens and the SQLite
# library of the system is used instead, see netdata_detect_sqlite3().

include_guard()

include(ExternalProject)

set(SQLITE_VERSION "3.53.4")
set(SQLITE_VERSION_NUMBER "3530400")
set(SQLITE_VERSION_YEAR "2026")

set(SQLITE_TARBALL_SHA256 "d18fa15aec74d8c17e1463f861095adc01b5ad190256acb4f91d22f0368d232b")
set(SQLITE_GIT_SHA "b09c88c14082339b66c7b7158d609a771e64ca69") # tag is: version-${SQLITE_VERSION}

# Oldest system SQLite that is accepted. The queries use UNIXEPOCH(), which
# needs 3.38.0, so anything older cannot work.
set(SQLITE_SYSTEM_MIN_VERSION "3.38.0")

function(netdata_bundle_sqlite3)
        message(STATUS "Preparing SQLite ${SQLITE_VERSION}")

        set(sqlite_SOURCE_DIR "${CMAKE_BINARY_DIR}/sqlite-src")
        set(sqlite_BINARY_DIR "${CMAKE_BINARY_DIR}/sqlite-build")
        set(sqlite_OUTPUT_DIR "${CMAKE_BINARY_DIR}/sqlite-output")

        file(MAKE_DIRECTORY "${sqlite_OUTPUT_DIR}")

        if(NETDATA_SQLITE_SOURCE_DIR)
                # Offline builds: an already-unpacked SQLite source tree replaces
                # the download step entirely (#23510).
                message(STATUS "Using local SQLite sources: ${NETDATA_SQLITE_SOURCE_DIR}")
                set(sqlite_SOURCE_DIR "${NETDATA_SQLITE_SOURCE_DIR}")
                set(SQLITE_FETCH_ARGS "")
        elseif(SQLITE_USE_GIT)
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
                # GNU Make jobserver flags are not understood by FreeBSD make.
                BUILD_COMMAND "${CMAKE_COMMAND}" -E env MAKEFLAGS= make sqlite3.c sqlite3.h
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

        set(NETDATA_SQLITE_INCLUDE_DIRS "${sqlite_OUTPUT_DIR}" PARENT_SCOPE)
        set(NETDATA_SQLITE_LIBRARIES sqlite3 PARENT_SCOPE)

        message(STATUS "Finished preparing SQLite ${SQLITE_VERSION}")
endfunction()

# Handle setup of SQLite for the build.
#
# By default this builds the bundled copy. With ENABLE_BUNDLED_SQLITE turned
# off it finds the system copy with pkg-config instead, fetches nothing, and
# fails if there is none. Unlike the other ENABLE_BUNDLED_* options there is
# no fallback to the bundled copy, so the result does not depend on the host.
#
# A system library has no recover extension, so recovery of the metadata
# database is not available with it. It is usually built without
# UPDATE/DELETE ... LIMIT too, which only the unit test relies on.
macro(netdata_detect_sqlite3)
        if(ENABLE_BUNDLED_SQLITE)
                netdata_bundle_sqlite3()
        else()
                pkg_check_modules(SQLITE3 sqlite3>=${SQLITE_SYSTEM_MIN_VERSION})

                if(NOT SQLITE3_FOUND)
                        message(FATAL_ERROR "ENABLE_BUNDLED_SQLITE is off, but SQLite >= ${SQLITE_SYSTEM_MIN_VERSION} could not be found.")
                endif()

                message(STATUS "Using system SQLite ${SQLITE3_VERSION}")

                set(NETDATA_SQLITE_INCLUDE_DIRS ${SQLITE3_INCLUDE_DIRS})
                set(NETDATA_SQLITE_CFLAGS_OTHER ${SQLITE3_CFLAGS_OTHER})
                set(NETDATA_SQLITE_LIBRARIES ${SQLITE3_LDFLAGS})
        endif()
endmacro()

function(netdata_add_sqlite3_to_target _target)
        if(ENABLE_BUNDLED_SQLITE)
                target_include_directories(${_target} BEFORE PUBLIC "${NETDATA_SQLITE_INCLUDE_DIRS}")
                target_link_libraries(${_target} PUBLIC ${NETDATA_SQLITE_LIBRARIES})
                add_dependencies(${_target} sqlite3)
        else()
                target_include_directories(${_target} PUBLIC ${NETDATA_SQLITE_INCLUDE_DIRS})
                target_compile_options(${_target} PUBLIC ${NETDATA_SQLITE_CFLAGS_OTHER})
                target_link_libraries(${_target} PUBLIC ${NETDATA_SQLITE_LIBRARIES})
        endif()
endfunction()
