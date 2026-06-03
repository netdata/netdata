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
                # On Windows we use the system-packaged libbacktrace
                # (mingw-w64-ucrt-x86_64-libbacktrace from MSYS2). The
                # ExternalProject_Add fallback below relies on an autotools
                # `configure` shell script that cannot be dispatched by the
                # native-Windows cmake/ninja stack (cmd.exe has no concept
                # of shebangs), so source-bundling is not viable here.
                find_package(PkgConfig REQUIRED)
                pkg_check_modules(LIBBACKTRACE REQUIRED libbacktrace)

                find_library(libbacktrace_LIBRARY
                        NAMES libbacktrace.a backtrace
                        HINTS ${LIBBACKTRACE_LIBRARY_DIRS}
                        REQUIRED
                )

                add_library(libbacktrace_library STATIC IMPORTED GLOBAL)
                set_property(TARGET libbacktrace_library
                        PROPERTY IMPORTED_LOCATION "${libbacktrace_LIBRARY}")

                set(NETDATA_LIBBACKTRACE_INCLUDE_DIRS "${LIBBACKTRACE_INCLUDE_DIRS}" PARENT_SCOPE)
                set(NETDATA_LIBBACKTRACE_LIBRARIES libbacktrace_library PARENT_SCOPE)
                set(HAVE_LIBBACKTRACE TRUE PARENT_SCOPE)

                message(STATUS "Using system libbacktrace from ${libbacktrace_LIBRARY}")
                return()
        endif()

        set(libbacktrace_SOURCE_DIR "${CMAKE_BINARY_DIR}/libbacktrace-src")
        set(libbacktrace_BINARY_DIR "${CMAKE_BINARY_DIR}/libbacktrace-build")
        set(libbacktrace_INSTALL_DIR "${CMAKE_BINARY_DIR}/libbacktrace-install")
        set(libbacktrace_LIBRARY "${libbacktrace_INSTALL_DIR}/lib/libbacktrace.a")

        # Clone and build libbacktrace
        ExternalProject_Add(
                libbacktrace
                GIT_REPOSITORY https://github.com/ianlancetaylor/libbacktrace.git
                SOURCE_DIR "${libbacktrace_SOURCE_DIR}"
                BINARY_DIR "${libbacktrace_BINARY_DIR}"
                CONFIGURE_COMMAND "${libbacktrace_SOURCE_DIR}/configure" --prefix=${libbacktrace_INSTALL_DIR} --enable-static
                BUILD_COMMAND make install
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

        # The `libbacktrace` target only exists when we bundle from source
        # via ExternalProject_Add (non-Windows path). On Windows we use the
        # system package and there is no source build to depend on.
        if(TARGET libbacktrace)
                add_dependencies(${_target} libbacktrace)
        endif()
endfunction()
