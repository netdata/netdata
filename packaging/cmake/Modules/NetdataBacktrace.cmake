# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of libbacktrace
#
# Handle bundling of libbacktrace.
#
# This clones and builds libbacktrace using ExternalProject functionality.

include(ExternalProject)

function(netdata_bundle_libbacktrace)
        message(STATUS "Preparing libbacktrace")

        if(OS_WINDOWS)
                include(FetchContent)
                include(CheckCSourceCompiles)
                check_c_source_compiles("#include <unwind.h>\nint main(void) { struct _Unwind_Context *c = 0; int b = 0; return (int)_Unwind_GetIPInfo(c, &b); }"
                        NETDATA_HAVE_UNWIND_GETIPINFO)
                if(NOT NETDATA_HAVE_UNWIND_GETIPINFO)
                        message(FATAL_ERROR "The Windows compiler must provide _Unwind_GetIPInfo for libbacktrace")
                endif()

                # Match the source revision already used by the current Windows build.
                FetchContent_Declare(netdata_libbacktrace_source
                        GIT_REPOSITORY https://github.com/ianlancetaylor/libbacktrace.git
                        GIT_TAG 0b9b49cf4a2c9229fc052d6716e1528b2f23e91a)
                FetchContent_GetProperties(netdata_libbacktrace_source)
                if(NOT netdata_libbacktrace_source_POPULATED)
                        FetchContent_Populate(netdata_libbacktrace_source)
                endif()

                set(_backtrace_source "${netdata_libbacktrace_source_SOURCE_DIR}")
                set(_backtrace_generated "${CMAKE_BINARY_DIR}/libbacktrace-generated")
                file(MAKE_DIRECTORY "${_backtrace_generated}")
                file(WRITE "${_backtrace_generated}/config.h"
                        "#define BACKTRACE_ELF_SIZE unused\n"
                        "#define BACKTRACE_XCOFF_SIZE unused\n"
                        "#define HAVE_ATOMIC_FUNCTIONS 1\n"
                        "#define HAVE_DECL_GETPAGESIZE 0\n"
                        "#define HAVE_DECL_STRNLEN 1\n"
                        "#define HAVE_DECL__PGMPTR 0\n"
                        "#define HAVE_GETIPINFO 1\n"
                        "#define HAVE_SYNC_FUNCTIONS 1\n"
                        "#define HAVE_WINDOWS_H 1\n"
                        "#define HAVE_TLHELP32_H 1\n")
                file(WRITE "${_backtrace_generated}/backtrace-supported.h"
                        "#define BACKTRACE_SUPPORTED 1\n"
                        "#define BACKTRACE_USES_MALLOC 1\n"
                        "#define BACKTRACE_SUPPORTS_THREADS 1\n"
                        "#define BACKTRACE_SUPPORTS_DATA 0\n"
                        "#define BACKTRACE_SUPPORTS_MOREDATA 1\n")

                add_library(libbacktrace_library STATIC
                        "${_backtrace_source}/atomic.c"
                        "${_backtrace_source}/backtrace.c"
                        "${_backtrace_source}/dwarf.c"
                        "${_backtrace_source}/fileline.c"
                        "${_backtrace_source}/pecoff.c"
                        "${_backtrace_source}/posix.c"
                        "${_backtrace_source}/print.c"
                        "${_backtrace_source}/read.c"
                        "${_backtrace_source}/simple.c"
                        "${_backtrace_source}/sort.c"
                        "${_backtrace_source}/state.c"
                        "${_backtrace_source}/alloc.c")
                target_compile_definitions(libbacktrace_library PRIVATE HAVE_CONFIG_H)
                target_include_directories(libbacktrace_library PUBLIC
                        "${_backtrace_generated}" "${_backtrace_source}")
                add_custom_target(libbacktrace DEPENDS libbacktrace_library)
                set(NETDATA_LIBBACKTRACE_INCLUDE_DIRS "${_backtrace_generated};${_backtrace_source}" PARENT_SCOPE)
                set(NETDATA_LIBBACKTRACE_LIBRARIES libbacktrace_library PARENT_SCOPE)
                set(HAVE_LIBBACKTRACE TRUE PARENT_SCOPE)
                message(STATUS "Finished preparing native Windows libbacktrace")
                return()
        endif()

        set(libbacktrace_SOURCE_DIR "${CMAKE_BINARY_DIR}/libbacktrace-src")
        set(libbacktrace_BINARY_DIR "${CMAKE_BINARY_DIR}/libbacktrace-build")
        set(libbacktrace_INSTALL_DIR "${CMAKE_BINARY_DIR}/libbacktrace-install")
        set(libbacktrace_LIBRARY "${libbacktrace_INSTALL_DIR}/lib/libbacktrace.a")

        set(_BT_MAKE_EXECUTABLE make)
        set(_bt_configure_cmd "${libbacktrace_SOURCE_DIR}/configure" --prefix=${libbacktrace_INSTALL_DIR} --enable-static)
        set(_bt_build_cmd ${_BT_MAKE_EXECUTABLE} install)

        # Clone and build libbacktrace
        ExternalProject_Add(
                libbacktrace
                GIT_REPOSITORY https://github.com/ianlancetaylor/libbacktrace.git
                SOURCE_DIR "${libbacktrace_SOURCE_DIR}"
                BINARY_DIR "${libbacktrace_BINARY_DIR}"
                CONFIGURE_COMMAND ${_bt_configure_cmd}
                BUILD_COMMAND ${_bt_build_cmd}
                INSTALL_COMMAND ""
                BUILD_BYPRODUCTS "${libbacktrace_LIBRARY}"
                EXCLUDE_FROM_ALL 1
                UPDATE_DISCONNECTED ON
        )

        # Create an imported library target
        add_library(libbacktrace_library STATIC IMPORTED GLOBAL)
        set_property(
                TARGET libbacktrace_library
                PROPERTY IMPORTED_LOCATION "${libbacktrace_LIBRARY}"
        )
        add_dependencies(libbacktrace_library libbacktrace)

        # Export variables to parent scope
        set(NETDATA_LIBBACKTRACE_INCLUDE_DIRS "${libbacktrace_INSTALL_DIR}/include" PARENT_SCOPE)
        set(NETDATA_LIBBACKTRACE_LIBRARIES libbacktrace_library PARENT_SCOPE)
        set(HAVE_LIBBACKTRACE TRUE PARENT_SCOPE)

        message(STATUS "Finished preparing libbacktrace")
endfunction()

function(netdata_add_libbacktrace_to_target _target)
        target_include_directories(${_target} BEFORE PUBLIC "${NETDATA_LIBBACKTRACE_INCLUDE_DIRS}")
        target_link_libraries(${_target} PUBLIC ${NETDATA_LIBBACKTRACE_LIBRARIES})
        add_dependencies(${_target} libbacktrace)
endfunction()
