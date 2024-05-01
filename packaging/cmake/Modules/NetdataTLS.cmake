# Handling for TLS implementation and libcrypto
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(NetdataUtil)

set(ENABLE_OPENSSL True)

# Locate a usable TLS implementation and libcrypto equivalent
macro(netdata_detect_tls_and_crypto)
    pkg_check_modules(TLS openssl)
    pkg_check_modules(CRYPTO libcrypto)

    if(NOT TLS_FOUND)
        if(MACOS)
            execute_process(COMMAND
                            brew --prefix --installed openssl
                            RESULT_VARIABLE BREW_OPENSSL
                            OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                            OUTPUT_STRIP_TRAILING_WHITESPACE)

            if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
            endif()

            set(TLS_INCLUDE_DIRS "${BREW_OPENSSL_PREFIX}/include")
            set(TLS_LIBRARIES "ssl;crypto")
            set(TLS_LIBRARY_DIRS "${BREW_OPENSSL_PREFIX}/lib")
            set(CRYPTO_INCLUDE_DIRS "${TLS_INCLUDE_DIRS}")
            set(CRYPTO_LIBRARIES "crypto")
            set(CRYPTO_LIBRARY_DIRS "${TLS_LIBRARY_DIRS}")
        else()
            message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
        endif()
    endif()
endmacro()

# Add TLS implementation to a target with the given scope
function(netdata_add_tls_to_target _target _scope)
    netdata_add_lib_to_target(${_target} ${_scope} TLS)
endfunction()

# Add libcrypto to a target with the given scope
function(netdata_add_crypto_to_target _target _scope)
    netdata_add_lib_to_target(${_target} ${_scope} CRYPTO)
endfunction()
