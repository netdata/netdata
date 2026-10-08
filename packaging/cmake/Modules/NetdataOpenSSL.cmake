# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of OpenSSL

include_guard()

# Handle setup of OpenSSL for the build.
#
# Finds the system copy with pkg-config. The result is the PkgConfig::TLS
# and PkgConfig::CRYPTO targets the rest of the build links, and both are
# required.
macro(netdata_detect_openssl)
        pkg_check_modules(TLS IMPORTED_TARGET openssl)
        pkg_check_modules(CRYPTO IMPORTED_TARGET libcrypto)

        if(NOT TARGET PkgConfig::TLS)
                message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
        endif()

        if(NOT TARGET PkgConfig::CRYPTO)
                message(FATAL_ERROR "libcrypto is required for building Netdata, but could not be found.")
        endif()
endmacro()
