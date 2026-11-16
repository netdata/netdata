# SPDX-License-Identifier: GPL-3.0-or-later
# Bundled libbacktrace: cloned, built and installed into the build tree via
# ExternalProject, then exposed as an imported static library.

include_guard()

include(ExternalProject)

function(netdata_bundle_libbacktrace)
        message(STATUS "Preparing libbacktrace")

        set(libbacktrace_SOURCE_DIR "${CMAKE_BINARY_DIR}/libbacktrace-src")
        set(libbacktrace_BINARY_DIR "${CMAKE_BINARY_DIR}/libbacktrace-build")
        set(libbacktrace_INSTALL_DIR "${CMAKE_BINARY_DIR}/libbacktrace-install")
        set(libbacktrace_LIBRARY "${libbacktrace_INSTALL_DIR}/lib/libbacktrace.a")

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

        add_library(libbacktrace_library STATIC IMPORTED GLOBAL)
        set_property(
                TARGET libbacktrace_library
                PROPERTY IMPORTED_LOCATION "${libbacktrace_LIBRARY}"
        )
        add_dependencies(libbacktrace_library libbacktrace)

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
