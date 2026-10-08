# SPDX-License-Identifier: GPL-3.0-or-later
#
# Handle bundling of SQLite.

include_guard()

include(ExternalProject)

set(SQLITE_VERSION "3.53.4")
set(SQLITE_VERSION_NUMBER "3530400")
set(SQLITE_VERSION_YEAR "2026")
set(SQLITE_TARBALL_SHA256 "d18fa15aec74d8c17e1463f861095adc01b5ad190256acb4f91d22f0368d232b")
set(SQLITE_GIT_SHA "b09c88c14082339b66c7b7158d609a771e64ca69")

option(SQLITE_USE_GIT "Fetch SQLite sources via git clone instead of tarball" OFF)

function(netdata_bundle_sqlite3)
        message(STATUS "Preparing SQLite ${SQLITE_VERSION}")

        set(sqlite_OUTPUT_DIR "${CMAKE_BINARY_DIR}/sqlite-output")
        file(MAKE_DIRECTORY "${sqlite_OUTPUT_DIR}")
        set(_sqlite_outputs
                "${sqlite_OUTPUT_DIR}/sqlite3.c"
                "${sqlite_OUTPUT_DIR}/sqlite3.h"
                "${sqlite_OUTPUT_DIR}/sqlite3recover.c"
                "${sqlite_OUTPUT_DIR}/sqlite3recover.h"
                "${sqlite_OUTPUT_DIR}/dbdata.c")

        if(OS_WINDOWS)
                # SQLite's UPDATE/DELETE LIMIT grammar is selected when Lemon
                # generates parse.c, so compiling the amalgamation with the
                # feature define alone is insufficient.
                if(NETDATA_SQLITE_SOURCE_DIR)
                        message(STATUS "Using local SQLite sources: ${NETDATA_SQLITE_SOURCE_DIR}")
                        set(sqlite_SOURCE_DIR "${NETDATA_SQLITE_SOURCE_DIR}")
                else()
                        # Populate the source only; Netdata runs SQLite's generators.
                        include(FetchContent)
                        include(NetdataFetchContentExtra)
                        set(_sqlite_fetch_only_options)
                        if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.18)
                                list(APPEND _sqlite_fetch_only_options SOURCE_SUBDIR _netdata_fetch_only)
                        endif()
                        if(SQLITE_USE_GIT)
                                FetchContent_Declare(netdata_sqlite_source
                                        GIT_REPOSITORY https://github.com/sqlite/sqlite.git
                                        GIT_TAG "${SQLITE_GIT_SHA}"
                                        ${_sqlite_fetch_only_options})
                        else()
                                FetchContent_Declare(netdata_sqlite_source
                                        URL "https://www.sqlite.org/${SQLITE_VERSION_YEAR}/sqlite-src-${SQLITE_VERSION_NUMBER}.zip"
                                        URL_HASH "SHA256=${SQLITE_TARBALL_SHA256}"
                                        ${_sqlite_fetch_only_options})
                        endif()
                        FetchContent_Populate_Only(netdata_sqlite_source)
                        set(sqlite_SOURCE_DIR "${netdata_sqlite_source_SOURCE_DIR}")
                endif()

                get_filename_component(_sqlite_ucrt_bin "${CMAKE_C_COMPILER}" DIRECTORY)
                find_program(SQLITE_TCLSH_EXECUTABLE NAMES tclsh.exe HINTS "${_sqlite_ucrt_bin}" NO_DEFAULT_PATH)
                if(NOT SQLITE_TCLSH_EXECUTABLE)
                        message(FATAL_ERROR "UCRT64 tclsh.exe is required to generate SQLite; install mingw-w64-ucrt-x86_64-tcl")
                endif()

                add_executable(sqlite_lemon "${sqlite_SOURCE_DIR}/tool/lemon.c")
                add_executable(sqlite_mkkeywordhash "${sqlite_SOURCE_DIR}/tool/mkkeywordhash.c")
                add_executable(sqlite_mksourceid "${sqlite_SOURCE_DIR}/tool/mksourceid.c")

                set(_sqlite_work_dir "${CMAKE_BINARY_DIR}/sqlite-native-build")
                file(MAKE_DIRECTORY "${_sqlite_work_dir}")
                add_custom_command(
                        OUTPUT ${_sqlite_outputs}
                        COMMAND "${CMAKE_COMMAND}" -E make_directory "${_sqlite_work_dir}"
                        COMMAND "${CMAKE_COMMAND}"
                                "-DSQLITE_SOURCE_DIR=${sqlite_SOURCE_DIR}"
                                "-DSQLITE_WORK_DIR=${_sqlite_work_dir}"
                                "-DSQLITE_OUTPUT_DIR=${sqlite_OUTPUT_DIR}"
                                "-DSQLITE_TCLSH=${SQLITE_TCLSH_EXECUTABLE}"
                                "-DSQLITE_LEMON=$<TARGET_FILE:sqlite_lemon>"
                                "-DSQLITE_MKKEYWORDHASH=$<TARGET_FILE:sqlite_mkkeywordhash>"
                                "-DSQLITE_MKSOURCEID=$<TARGET_FILE:sqlite_mksourceid>"
                                -P "${CMAKE_SOURCE_DIR}/packaging/cmake/GenerateSQLite.cmake"
                        DEPENDS sqlite_lemon sqlite_mkkeywordhash sqlite_mksourceid
                                "${CMAKE_SOURCE_DIR}/packaging/cmake/GenerateSQLite.cmake"
                                "${sqlite_SOURCE_DIR}/main.mk"
                                "${sqlite_SOURCE_DIR}/tool/mksqlite3c.tcl"
                                "${sqlite_SOURCE_DIR}/src/parse.y"
                        BYPRODUCTS "${_sqlite_work_dir}/parse.c" "${_sqlite_work_dir}/parse.h"
                                   "${_sqlite_work_dir}/fts5parse.c" "${_sqlite_work_dir}/fts5parse.h"
                                   "${_sqlite_work_dir}/sqlite3.c"
                        COMMENT "Generating SQLite ${SQLITE_VERSION} amalgamation with UPDATE/DELETE LIMIT grammar")
                add_custom_target(sqlite_project DEPENDS ${_sqlite_outputs})
        else()
                set(sqlite_SOURCE_DIR "${CMAKE_BINARY_DIR}/sqlite-src")
                set(sqlite_BINARY_DIR "${CMAKE_BINARY_DIR}/sqlite-build")

                if(NETDATA_SQLITE_SOURCE_DIR)
                        message(STATUS "Using local SQLite sources: ${NETDATA_SQLITE_SOURCE_DIR}")
                        set(sqlite_SOURCE_DIR "${NETDATA_SQLITE_SOURCE_DIR}")
                        set(SQLITE_FETCH_ARGS "")
                elseif(SQLITE_USE_GIT)
                        set(SQLITE_FETCH_ARGS
                                GIT_REPOSITORY https://github.com/sqlite/sqlite.git
                                GIT_TAG "${SQLITE_GIT_SHA}")
                else()
                        set(SQLITE_FETCH_ARGS
                                URL "https://www.sqlite.org/${SQLITE_VERSION_YEAR}/sqlite-src-${SQLITE_VERSION_NUMBER}.zip"
                                URL_HASH "SHA256=${SQLITE_TARBALL_SHA256}")
                endif()

                ExternalProject_Add(sqlite_project
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
                        BUILD_BYPRODUCTS ${_sqlite_outputs}
                        EXCLUDE_FROM_ALL 1
                        UPDATE_DISCONNECTED ON)
        endif()

        set(SQLITE_SOURCES "${sqlite_OUTPUT_DIR}/sqlite3.c"
                "${sqlite_OUTPUT_DIR}/sqlite3recover.c" "${sqlite_OUTPUT_DIR}/dbdata.c")
        set_source_files_properties(${SQLITE_SOURCES} PROPERTIES GENERATED TRUE)
        add_library(sqlite3 STATIC ${SQLITE_SOURCES})
        target_compile_definitions(sqlite3 PRIVATE
                SQLITE_ENABLE_UPDATE_DELETE_LIMIT SQLITE_ENABLE_MEMORY_MANAGEMENT
                SQLITE_OMIT_LOAD_EXTENSION SQLITE_ENABLE_DBSTAT_VTAB SQLITE_ENABLE_DBPAGE_VTAB)
        target_compile_options(sqlite3 PRIVATE -Wno-unused-parameter)
        target_include_directories(sqlite3 PUBLIC "${sqlite_OUTPUT_DIR}")
        add_dependencies(sqlite3 sqlite_project)

        set(NETDATA_SQLITE_INCLUDE_DIRS "${sqlite_OUTPUT_DIR}" PARENT_SCOPE)
        set(NETDATA_SQLITE_LIBRARIES sqlite3 PARENT_SCOPE)
        message(STATUS "Finished preparing SQLite ${SQLITE_VERSION}")
endfunction()

function(netdata_add_sqlite3_to_target _target)
        target_include_directories(${_target} BEFORE PUBLIC "${NETDATA_SQLITE_INCLUDE_DIRS}")
        target_link_libraries(${_target} PUBLIC ${NETDATA_SQLITE_LIBRARIES})
        add_dependencies(${_target} sqlite3)
endfunction()
