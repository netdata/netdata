# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of snappy

include_guard()

# Handle setup of snappy for the build.
#
# Finds the system copy with pkg-config, with NetdataDaemon.cmake's
# check_library_exists fallback still covering systems whose snappy ships
# no .pc file. The result is reported in the SNAPPY_* variables the rest
# of the build already consumes. The Prometheus remote-write exporter
# decides whether a missing snappy is fatal.
macro(netdata_detect_snappy)
        pkg_check_modules(SNAPPY snappy)
endmacro()
