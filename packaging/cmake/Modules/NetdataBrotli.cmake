# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of brotli

include_guard()

# Handle setup of brotli for the build.
#
# Finds the system copy with pkg-config; the result is reported in the
# LIBBROTLI_* variables the rest of the build already consumes. brotli
# stays optional.
macro(netdata_detect_brotli)
        pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)
endmacro()
