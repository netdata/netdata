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
# the compiler flags for the run.
function(add_simple_extra_compiler_flag match flag)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag}" flag_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})
        endif()

        if(HAVE_C_${flag_name} AND HAVE_CXX_${flag_name})
            add_compile_options("${flag}")
            add_link_options("${flag}")
        endif()
endfunction()

# Same as add_simple_extra_compiler_flag, but check for a second flag if the
# first one is unsupported.
function(add_double_extra_compiler_flag match flag1 flag2)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag1}" flag1_name)
        make_cpp_safe_name("${flag2}" flag2_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag1}" HAVE_C_${flag1_name})
                if(NOT HAVE_C_${flag1_name})
                        check_c_compiler_flag("${flag2}" HAVE_C_${flag2_name})
                endif()
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag1}" HAVE_CXX_${flag1_name})
                if(NOT HAVE_CXX_${flag1_name})
                        check_cxx_compiler_flag("${flag2}" HAVE_CXX_${flag2_name})
                endif()
        endif()

        if(HAVE_C_${flag1_name} AND HAVE_CXX_${flag1_name})
            add_compile_options("${flag1}")
            add_link_options("${flag1}")
        elseif(HAVE_C_${flag2_name} AND HAVE_CXX${flag2_name})
            add_compile_options("${flag2}")
            add_link_options("${flag2}")
        endif()
endfunction()

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" TRUE)
else()
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" FALSE)
endif()

option(ENABLE_ADDRESS_SANITIZER "Build with address sanitizer enabled" False)
mark_as_advanced(ENABLE_ADDRESS_SANITIZER)

if(ENABLE_ADDRESS_SANITIZER)
        set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address")
endif()

set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_C_FLAGS}")

if(NOT ${DISABLE_HARDENING})
        add_double_extra_compiler_flag("stack-protector" "-fstack-protector-strong" "-fstack-protector")
        add_double_extra_compiler_flag("_FORTIFY_SOURCE" "-D_FORTIFY_SOURCE=3" "-D_FORTIFY_SOURCE=2")
        add_simple_extra_compiler_flag("stack-clash-protection" "-fstack-clash-protection")
        add_simple_extra_compiler_flag("-fcf-protection" "-fcf-protection=full")
        add_simple_extra_compiler_flag("branch-protection" "-mbranch-protection=standard")
endif()

foreach(FLAG function-sections data-sections)
        add_simple_extra_compiler_flag("${FLAG}" "-f${FLAG}")
endforeach()

add_simple_extra_compiler_flag("-Wbuiltin-macro-redefined" "-Wno-builtin-macro-redefined")
add_simple_extra_compiler_flag("-fexecptions" "-fexceptions")
