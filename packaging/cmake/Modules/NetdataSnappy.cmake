# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of snappy

include_guard()

# Handle bundling of snappy.
#
# This pulls it in as a sub-project using FetchContent functionality,
# building only the static library.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_snappy)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of snappy")

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        set(SNAPPY_BUILD_TESTS OFF)
        set(SNAPPY_BUILD_BENCHMARKS OFF)
        set(BUILD_SHARED_LIBS OFF)

        set(repo https://github.com/google/snappy)
        set(tag 6af9287fbdb913f0794d0148c6aa43b58e63c8e3) # 1.2.2

        FetchContent_Declare(snappy
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                EXCLUDE_FROM_ALL
        )

        FetchContent_MakeAvailable_NoInstall(snappy)

        message(STATUS "Finished preparing vendored copy of snappy")
endfunction()

# Handle setup of snappy for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config,
# with NetdataDaemon.cmake's check_library_exists fallback still covering
# systems whose snappy ships no .pc file.
#
# Either way the result is reported in the SNAPPY_* variables the rest of
# the build already consumes. The Prometheus remote-write exporter decides
# whether a missing snappy is fatal.
macro(netdata_detect_snappy)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                include(FetchContent)
                netdata_bundle_snappy()
                set(SNAPPY_FOUND TRUE)
                set(SNAPPY_LIBRARIES snappy)
                set(SNAPPY_INCLUDE_DIRS "")
                set(SNAPPY_CFLAGS_OTHER "")
        else()
                pkg_check_modules(SNAPPY snappy)
        endif()
endmacro()
