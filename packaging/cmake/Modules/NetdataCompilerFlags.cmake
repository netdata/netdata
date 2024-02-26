# Functions to simplify handling of extra compiler flags.
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(CheckCCompilerFlag)
include(CheckCXXCompilerFlag)

# Construct a pre-processor safe name
#
# This takes a specified value, and assigns the generated name to the
# specified target.
function(make_cpp_safe_name value target)
        string(REPLACE "-" "_" tmp "${value}")
        string(REPLACE "=" "_" tmp "${tmp}")
        set(${target} "${tmp}" PARENT_SCOPE)
endfunction()

# Conditionally add an extra compiler flag to C and C++ flags.
#
# If the language flags already match the `match` argument, skip this flag.
# Otherwise, check for support for `flag` and if support is found, add it to
# the language-specific `target` flag group.
function(add_simple_extra_compiler_flag match flag target)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag}" flag_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
                if(HAVE_C_${flag_name})
                        set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag}" PARENT_SCOPE)
                endif()
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})
                if(HAVE_CXX_${flag_name})
                        set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag}" PARENT_SCOPE)
                endif()
        endif()
endfunction()

# Same as add_simple_extra_compiler_flag, but check for a second flag if the
# first one is unsupported.
function(add_double_extra_compiler_flag match flag1 flag2 target)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag1}" flag1_name)
        make_cpp_safe_name("${flag2}" flag2_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag1}" HAVE_C_${flag1_name})
                if(HAVE_C_${flag1_name})
                        set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag1}" PARENT_SCOPE)
                else()
                        check_c_compiler_flag("${flag2}" HAVE_C_${flag2_name})
                        if(HAVE_C_${flag2_name})
                                set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag2}" PARENT_SCOPE)
                        endif()
                endif()
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag1}" HAVE_CXX_${flag1_name})
                if(HAVE_CXX_${flag1_name})
                        set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag1}" PARENT_SCOPE)
                else()
                        check_cxx_compiler_flag("${flag2}" HAVE_CXX_${flag2_name})
                        if(HAVE_CXX_${flag2_name})
                                set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag2}" PARENT_SCOPE)
                        endif()
                endif()
        endif()
endfunction()
