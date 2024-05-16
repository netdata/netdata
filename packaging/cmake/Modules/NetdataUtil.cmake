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

    if(NOT CMAKE_CROSSCOMPILING)
        include(CheckIncludeFile)

        check_include_file("linux/version.h" CAN_USE_VERSION_H)

        if(CAN_USE_VERSION_H)
            message(CHECK_START "Checking version using linux/version.h")
            file(WRITE "${CMAKE_BINARY_DIR}/kversion-test.c" "
            #include <stdio.h>
            #include <linux/version.h>

            int main() {
                printf(\"%i.%i.%i\", LINUX_VERSION_MAJOR, LINUX_VERSION_PATCHLEVEL, LINUX_VERSION_SUBLEVEL);
            }
            ")

            try_run(_run_success _compile_success
                    ${CMAKE_BINARY_DIR}
                    SOURCES ${CMAKE_BINARY_DIR}/kversion-test.c
                    RUN_OUTPUT_VARIABLE _kversion_output)

            if(_compile_success AND _run_success EQUAL 0)
                message(CHECK_PASS "success")
                set(_kversion_value "${_kversion_output}")
            else()
                message(CHECK_FAIL "failed")
            endif()
        endif()
    endif()

    if(NOT DEFINED _kversion_value)
        message(CHECK_START "Checking version using uname")
        execute_process(COMMAND uname -r
                        RESULT_VARIABLE _uname_result
                        OUTPUT_VARIABLE _uname_output)

        if(NOT _uname_result EQUAL 0)
            message(CHECK_FAIL "failed")
            message(CHECK_FAIL "unknown")
            set(HOST_KERNEL_VERSION "0.0.0" CACHE STRING "Detected host kernel version")
            return()
        else()
            message(CHECK_PASS "success")
        endif()

        set(_kversion_value "${_uname_output}")
    endif()

    string(REGEX REPLACE "-.+$" "" _kversion "${_kversion_value}")
    message(CHECK_PASS "${_kversion}")
    set(HOST_KERNEL_VERSION "${_kversion}" CACHE STRING "Detected host kernel version")
endfunction()

# Check what libc we're using.
#
# Sets the specified variable to the name of the libc or "unknown"
function(netdata_identify_libc _libc_name)
    if(NOT DEFINED _ND_DETECTED_LIBC)
        message(CHECK_START "Detecting libc implementation using ldd")

        execute_process(COMMAND ldd --version
                        COMMAND grep -q -i -E "glibc|gnu libc"
                        RESULT_VARIABLE LDD_RESULT
                        OUTPUT_VARIABLE LDD_OUTPUT
                        ERROR_VARIABLE LDD_OUTPUT)

        if(NOT LDD_RESULT)
            set(${_libc_name} glibc PARENT_SCOPE)
            set(_ND_DETECTED_LIBC glibc CACHE INTERNAL "")
            message(CHECK_PASS "glibc")
            return()
        endif()

        execute_process(COMMAND sh -c "ldd --version 2>&1 | grep -q -i 'musl'"
                        RESULT_VARIABLE LDD_RESULT
                        OUTPUT_VARIABLE LDD_OUTPUT
                        ERROR_VARIABLE LDD_OUTPUT)

        if(NOT LDD_RESULT)
            set(${_libc_name} musl PARENT_SCOPE)
            set(_ND_DETECTED_LIBC musl CACHE INTERNAL "")
            message(CHECK_PASS "musl")
            return()
        endif()

        message(CHECK_FAIL "unknown")

        message(CHECK_START "Looking for libc.so.6")
        find_program(LIBC_PATH libc.so.6
                     PATHS /lib /lib64 /usr/lib /usr/lib64
                     NO_DEFAULT_PATH
                     NO_PACKAGE_ROOT_PATH
                     NO_CMAKE_PATH
                     NO_CMAKE_ENVIRONMENT_PATH
                     NO_SYSTEM_ENVIRONMENT_PATH
                     NO_CMAKE_SYSTEM_PATH
                     NO_CMAKE_INSTALL_PREFIX
                     NO_CMAKE_FIND_ROOT_PATH)

        if(NOT "${LIBC_PATH}" EQUAL "LIBC_PATH-NOTFOUND")
            message(CHECK_PASS "found")
            message(CHECK_START "Detecting libc implementation using libc.so.6")

            execute_process(COMMAND "${LIBC_PATH}"
                            COMMAND head -n 1
                            COMMAND grep -q -i -E "gnu libc|gnu c library"
                            RESULT_VARIABLE LIBC_RESULT
                            OUTPUT_VARIABLE LIBC_OUTPUT
                            ERROR_VARIABLE LIBC_ERROR)

            if(NOT LIBC_RESULT)
                set(${_libc_name} glibc PARENT_SCOPE)
                set(_ND_DETECTED_LIBC glibc CACHE INTERNAL "")
                message(CHECK_PASS "glibc")
                return()
            else()
                message(CHECK_FAIL "unknown")
            endif()
        else()
            message(CHECK_FAIL "not found")
        endif()

        set(${_libc_name} unknown PARENT_SCOPE)
        set(_ND_DETECTED_LIBC unknown CACHE INTERNAL "")
    else()
        set(${_libc_name} ${_ND_DETECTED_LIBC} PARENT_SCOPE)
    endif()
endfunction()
