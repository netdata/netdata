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
