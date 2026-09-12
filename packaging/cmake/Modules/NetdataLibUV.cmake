# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of libuv

include_guard()

# Handle bundling of libuv.
#
# This pulls it in as a sub-project using FetchContent functionality,
# building only the static library.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_libuv)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of libuv")

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        set(LIBUV_BUILD_SHARED OFF)
        set(BUILD_TESTING OFF)

        set(repo https://github.com/libuv/libuv)
        set(tag 1cfa32ff59c076ffb6ed735bbc8c18361558661f) # v1.52.1

        FetchContent_Declare(libuv
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                EXCLUDE_FROM_ALL
        )

        FetchContent_MakeAvailable_NoInstall(libuv)

        message(STATUS "Finished preparing vendored copy of libuv")
endfunction()

# Handle setup of libuv for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config.
#
# Either way the result is reported in the LIBUV_* variables the rest of
# the build already consumes, and libuv is required.
macro(netdata_detect_libuv)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                include(FetchContent)
                netdata_bundle_libuv()
                FetchContent_GetProperties(libuv)
                set(LIBUV_FOUND TRUE)
                set(LIBUV_LDFLAGS uv_a)
                set(LIBUV_INCLUDE_DIRS "${libuv_SOURCE_DIR}/include")
                set(LIBUV_CFLAGS_OTHER "")
        else()
                pkg_check_modules(LIBUV libuv)
        endif()

        if(NOT LIBUV_FOUND)
                message(FATAL_ERROR "libuv is required for building Netdata, but could not be found.")
        endif()
endmacro()
