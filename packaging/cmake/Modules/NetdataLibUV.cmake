# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of libuv

include_guard()

# Handle setup of libuv for the build.
#
# Finds the system copy with pkg-config; the result is reported in the
# LIBUV_* variables the rest of the build already consumes, and libuv is
# required.
macro(netdata_detect_libuv)
        pkg_check_modules(LIBUV libuv)

        if(NOT LIBUV_FOUND)
                message(FATAL_ERROR "libuv is required for building Netdata, but could not be found.")
        endif()
endmacro()
