# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of zstd

include_guard()

# Handle bundling of zstd.
#
# This pulls it in as a sub-project using FetchContent functionality,
# building only the static library.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_zstd)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of zstd")

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        set(ZSTD_BUILD_STATIC ON)
        set(ZSTD_BUILD_SHARED OFF)
        set(ZSTD_BUILD_PROGRAMS OFF)
        set(ZSTD_BUILD_TESTS OFF)
        set(ZSTD_BUILD_CONTRIB OFF)

        set(repo https://github.com/facebook/zstd)
        set(tag f8745da6ff1ad1e7bab384bd1f9d742439278e99) # v1.5.7

        # The SOURCE_SUBDIR argument only reaches add_subdirectory() through
        # FetchContent_MakeAvailable on CMake >= 3.28; the pkg kind that
        # activates bundling enforces that floor.
        FetchContent_Declare(zstd
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                SOURCE_SUBDIR build/cmake
                EXCLUDE_FROM_ALL
        )

        FetchContent_MakeAvailable_NoInstall(zstd)

        message(STATUS "Finished preparing vendored copy of zstd")
endfunction()

# Handle setup of zstd for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config.
#
# Either way the result is reported in the LIBZSTD_* variables the rest of
# the build already consumes. zstd stays optional outside the pkg kind.
macro(netdata_detect_zstd)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                include(FetchContent)
                netdata_bundle_zstd()
                FetchContent_GetProperties(zstd)
                set(LIBZSTD_FOUND TRUE)
                set(LIBZSTD_LDFLAGS libzstd_static)
                set(LIBZSTD_INCLUDE_DIRS "${zstd_SOURCE_DIR}/lib")
                set(LIBZSTD_CFLAGS_OTHER "")
        else()
                pkg_check_modules(LIBZSTD libzstd)
        endif()
endmacro()
