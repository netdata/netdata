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

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fexceptions")
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_C_FLAGS}")

set(EXTRA_HARDENING_C_FLAGS "")
set(EXTRA_HARDENING_CXX_FLAGS "")

set(EXTRA_OPT_C_FLAGS "")
set(EXTRA_OPT_CXX_FLAGS "")

if(NOT ${DISABLE_HARDENING})
        add_double_extra_compiler_flag("stack-protector" "-fstack-protector-strong" "-fstack-protector" EXTRA_HARDENING)
        add_double_extra_compiler_flag("_FORTIFY_SOURCE" "-D_FORTIFY_SOURCE=3" "-D_FORTIFY_SOURCE=2" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("stack-clash-protection" "-fstack-clash-protection" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("-fcf-protection" "-fcf-protection=full" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("branch-protection" "-mbranch-protection=standard" EXTRA_HARDENING)
endif()

foreach(FLAG function-sections data-sections)
        add_simple_extra_compiler_flag("${FLAG}" "-f${FLAG}" EXTRA_OPT)
endforeach()

add_simple_extra_compiler_flag("-Wbuiltin-macro-redefined" "-Wno-builtin-macro-redefined" EXTRA_OPT)

foreach(RELTYP RELEASE DEBUG RELWITHDEBINFO MINSIZEREL)
        foreach(L C CXX)
                set(CMAKE_${L}_FLAGS_${RELTYP} "${CMAKE_${L}_FLAGS_${RELTYP}} ${EXTRA_HARDENING_C_FLAGS} ${EXTRA_OPT_C_FLAGS}")
        endforeach()
endforeach()
