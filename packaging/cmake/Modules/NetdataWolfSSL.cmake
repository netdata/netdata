# Macros and functions for handling of WolfSSL
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

function(netdata_is_there_wolfssl)
        pkg_check_modules(TLS IMPORTED_TARGET wolfssl)

        list(APPEND CMAKE_REQUIRED_LIBRARIES wolfssl)
        check_function_exists(wolfSSL_set_alpn_protos HAVE_WOLFSSL_SET_ALPN_PROTOS)
        if(NOT HAVE_WOLFSSL_SET_ALPN_PROTOS)
                message(FATAL_ERROR "Your WolfSSL library has not been compiled with the OPENSSL_EXTRA flag, which is necessary to create symbols for the OpenSSL API that Netdata uses.")
        endif()
endfunction()

