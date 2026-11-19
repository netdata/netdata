# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of zstd

include_guard()

# Handle setup of zstd for the build.
#
# Finds the system copy with pkg-config; the result is reported in the
# LIBZSTD_* variables the rest of the build already consumes. zstd stays
# optional, and USE_ZSTD=Off declines a copy that was found.
macro(netdata_detect_zstd)
        pkg_check_modules(LIBZSTD libzstd)

        # A normal variable shadows the cached result for the rest of the run.
        if(NOT USE_ZSTD)
                set(LIBZSTD_FOUND FALSE)
        endif()
endmacro()
