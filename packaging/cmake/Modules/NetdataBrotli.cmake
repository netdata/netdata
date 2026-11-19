# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of brotli

include_guard()

# Handle setup of brotli for the build.
#
# Finds the system copy with pkg-config; the result is reported in the
# LIBBROTLI_* variables the rest of the build already consumes. brotli
# stays optional, and USE_BROTLI=Off declines a copy that was found.
macro(netdata_detect_brotli)
        pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)

        # A normal variable shadows the cached result for the rest of the run.
        if(NOT USE_BROTLI)
                set(LIBBROTLI_FOUND FALSE)
        endif()
endmacro()
