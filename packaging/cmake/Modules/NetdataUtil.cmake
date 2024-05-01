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

# Handle adding a library to a target with the given scope.
#
# Expects the target name, scope, and variable prefix in that order.
function(netdata_add_lib_to_target _target _scope _prefix)
    target_link_libraries(${_target} ${_scope} ${${_prefix}_LIBRARIES})

    if(DEFINED ${_prefix}_LIBRARY_DIRS)
        target_link_directories(${_target} ${_scope} ${${_prefix}_LIBRARY_DIRS})
    endif()

    if(DEFINED ${_prefix}_LDFLAGS_OTHER)
        target_link_options(${_target} ${_scope} ${${_prefix}_LDFLAGS_OTHER})
    endif()

    if(DEFINED ${_prefix}_INCLUDE_DIRS)
        target_include_directories(${_target} BEFORE ${_scope} ${${_prefix}_INCLUDE_DIRS})
    endif()

    if(DEFINED ${_prefix}_CFLAGS_OTHER)
        target_compile_options(${_target} ${_scope} ${${_prefix}_CFLAGS_OTHER})
    endif()

    if(DEFINED ${_prefix}_DEFINITIONS)
        target_compile_definitions(${_target} ${_scope} ${${_prefix}_DEFINITIONS})
    endif()
endfunction()
