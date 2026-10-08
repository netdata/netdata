# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of lz4

include_guard()

# Handle setup of lz4 for the build.
#
# Finds the system copy with pkg-config; the result is reported in the
# LIBLZ4_* variables the rest of the build already consumes, and lz4 is
# required.
macro(netdata_detect_lz4)
        pkg_check_modules(LIBLZ4 liblz4>=1.7.1)

        if(NOT LIBLZ4_FOUND)
                message(FATAL_ERROR "liblz4 >= 1.7.1 is required for building Netdata, but could not be found.")
        endif()
endmacro()
