# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of brotli

include_guard()

# Handle bundling of brotli.
#
# This pulls it in as a sub-project using FetchContent functionality,
# building only the static libraries.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_brotli)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        message(STATUS "Preparing vendored copy of brotli")

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        # brotli's targets follow BUILD_SHARED_LIBS; the root sets it OFF, but
        # the bundling mode must not depend on that ordering staying true.
        set(BUILD_SHARED_LIBS OFF)
        set(BROTLI_DISABLE_TESTS ON)

        set(repo https://github.com/google/brotli)
        set(tag 028fb5a23661f123017c060daa546b55cf4bde29) # v1.2.0

        FetchContent_Declare(brotli
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                EXCLUDE_FROM_ALL
        )

        FetchContent_MakeAvailable_NoInstall(brotli)

        message(STATUS "Finished preparing vendored copy of brotli")
endfunction()

# Handle setup of brotli for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config.
#
# Either way the result is reported in the LIBBROTLI_* variables the rest
# of the build already consumes. brotli stays optional outside the pkg kind.
macro(netdata_detect_brotli)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                include(FetchContent)
                netdata_bundle_brotli()
                FetchContent_GetProperties(brotli)
                set(LIBBROTLI_FOUND TRUE)
                set(LIBBROTLI_LDFLAGS brotlienc brotlidec brotlicommon)
                set(LIBBROTLI_INCLUDE_DIRS "${brotli_SOURCE_DIR}/c/include")
                set(LIBBROTLI_CFLAGS_OTHER "")
        else()
                pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)
        endif()
endmacro()
