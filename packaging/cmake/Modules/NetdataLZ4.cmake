# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of lz4

include_guard()

# Handle bundling of lz4.
#
# This pulls it in as a sub-project using FetchContent functionality,
# building only the static library.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_lz4)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of lz4")

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        set(LZ4_BUILD_CLI OFF)
        set(LZ4_BUILD_LEGACY_LZ4C OFF)
        set(BUILD_STATIC_LIBS ON)
        set(BUILD_SHARED_LIBS OFF)

        set(repo https://github.com/lz4/lz4)
        set(tag ebb370ca83af193212df4dcbadcc5d87bc0de2f0) # v1.10.0

        # The SOURCE_SUBDIR argument only reaches add_subdirectory() through
        # FetchContent_MakeAvailable on CMake >= 3.28; the pkg kind that
        # activates bundling enforces that floor.
        FetchContent_Declare(lz4
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                SOURCE_SUBDIR build/cmake
                EXCLUDE_FROM_ALL
        )

        FetchContent_MakeAvailable_NoInstall(lz4)

        message(STATUS "Finished preparing vendored copy of lz4")
endfunction()

# Handle setup of lz4 for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config.
#
# Either way the result is reported in the LIBLZ4_* variables the rest of
# the build already consumes, and lz4 is required.
macro(netdata_detect_lz4)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                include(FetchContent)
                netdata_bundle_lz4()
                FetchContent_GetProperties(lz4)
                set(LIBLZ4_FOUND TRUE)
                set(LIBLZ4_LDFLAGS lz4_static)
                set(LIBLZ4_INCLUDE_DIRS "${lz4_SOURCE_DIR}/lib")
                set(LIBLZ4_CFLAGS_OTHER "")
        else()
                pkg_check_modules(LIBLZ4 liblz4>=1.7.1)
        endif()

        if(NOT LIBLZ4_FOUND)
                message(FATAL_ERROR "liblz4 >= 1.7.1 is required for building Netdata, but could not be found.")
        endif()
endmacro()
