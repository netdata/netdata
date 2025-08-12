# SPDX-License-Identifier: GPL-3.0-or-later
# Functions to simplify handling of extra compiler flags.

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

# Add a required extra compiler flag to C and C++ flags.
#
# Similar logic as add_simple_extra_compiler_flag, but ignores existing
# instances and throws an error if the flag is not supported.
function(add_required_compiler_flag flag)
  set(CMAKE_REQUIRED_FLAGS "-Werror")

  make_cpp_safe_name("${flag}" flag_name)

  check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
  check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})

  if(HAVE_C_${flag_name} AND HAVE_CXX_${flag_name})
    add_compile_options("${flag}")
    add_link_options("${flag}")
  else()
    message(FATAL_ERROR "${flag} support is required to build Netdata")
  endif()
endfunction()

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" TRUE)
        option(USE_LTO "Attempt to use of LTO when building. Defaults to being enabled if supported for release builds." FALSE)
else()
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" FALSE)
        option(USE_LTO "Attempt to use of LTO when building. Defaults to being enabled if supported for release builds." TRUE)
endif()

option(ENABLE_ADDRESS_SANITIZER "Build with address sanitizer enabled" False)
mark_as_advanced(ENABLE_ADDRESS_SANITIZER)

if(ENABLE_ADDRESS_SANITIZER)
        set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address")
endif()

if(USE_LTO)
  if(OS_WINDOWS)
    message(WARNING "LTO not supported on Windows, not checking for it")
  else()
    include(CheckIPOSupported)

    message(CHECK_START "Checking for LTO support")
    check_ipo_supported(RESULT HAVE_LTO)

    if(HAVE_LTO)
      message(CHECK_PASS "supported")
      set(CMAKE_INTERPROCEDURAL_OPTIMIZATION TRUE)

      if(CMAKE_C_COMPILER_ID STREQUAL "GNU")
        if(CMAKE_C_COMPILER_VERSION VERSION_GREATER_EQUAL 10.0)
          list(APPEND CMAKE_C_COMPILE_OPTIONS_IPO "-flto=auto")
          list(APPEND CMAKE_CXX_COMPILE_OPTIONS_IPO "-flto=auto")
        endif()
      #elseif(CMAKE_C_COMPILER_ID STREQUAL "Clang")
        #list(APPEND CMAKE_C_COMPILE_OPTIONS_IPO "-flto=thin")
        #list(APPEND CMAKE_CXX_COMPILE_OPTIONS_IPO "-flto=thin")
      endif()
    else()
      message(CHECK_FAIL "not supported")
    endif()
  endif()
else()
  message(STATUS "Not checking for LTO support as it has been explicitly disabled")
endif()

set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_C_FLAGS}")

add_required_compiler_flag("-fexceptions")

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
add_simple_extra_compiler_flag("-fexceptions" "-fexceptions")
