# Utility functions used by other modules.
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include_guard()

# Determine the version of the host kernel.
#
# Only works on UNIX-like systems, stores the version in the cache
# variable HOST_KERNEL_VERSION.
function(netdata_detect_host_kernel_version)
    if(DEFINED HOST_KERNEL_VERSION)
        return()
    endif()

    message(CHECK_START "Determining host kernel version")

    execute_process(COMMAND uname -r
                    RESULT_VARIABLE _uname_result
                    OUTPUT_VARIABLE _uname_output)

    if(NOT _uname_result)
        message(CHECK_FAIL "unknown")
        set(HOST_KERNEL_VERSION "0.0.0" CACHE STRING "Detected host kernel version")
        return()
    endif()

    string(REGEX REPLACE "-.+$" "" _kversion "${_uname_output}")
    message(CHECK_PASS "${_kversion}")
    set(HOST_KERNEL_VERSION "${_kversion}" CACHE STRING "Detected host kernel version")
endfunction()

# Check what libc we're using.
#
# Sets the specified variable to the name of the libc or "unknown"
function(netdata_identify_libc _libc_name)
    if(NOT DEFINED _ND_DETECTED_LIBC)
        message(INFO "Detecting libc implementation")

        execute_process(COMMAND ldd --version
                        COMMAND grep -q -i -E "glibc|gnu libc"
                        RESULT_VARIABLE LDD_IS_GLIBC
                        OUTPUT_VARIABLE LDD_OUTPUT
                        ERROR_VARIABLE LDD_OUTPUT)

        if(LDD_IS_GLIBC)
            set(${_libc_name} glibc PARENT_SCOPE)
            set(_ND_DETECTED_LIBC glibc CACHE INTERNAL "")
            return()
        endif()

        execute_process(COMMAND ldd --version
                        COMMAND grep -q -i -E "musl"
                        RESULT_VARIABLE LDD_IS_MUSL
                        OUTPUT_VARIABLE LDD_OUTPUT
                        ERROR_VARIABLE LDD_OUTPUT)

        if(LDD_IS_MUSL)
            set(${_libc_name} musl PARENT_SCOPE)
            set(_ND_DETECTED_LIBC musl CACHE INTERNAL "")
            return()
        endif()

        set(${_libc_name} unknown PARENT_SCOPE)
        set(_ND_DETECTED_LIBC unknown CACHE INTERNAL "")
    else()
        set(${_libc_name} ${_ND_DETECTED_LIBC} PARENT_SCOPE)
    endif()
endfunction()
