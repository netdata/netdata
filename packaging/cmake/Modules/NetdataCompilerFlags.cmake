# SPDX-License-Identifier: GPL-3.0-or-later
# Functions to simplify handling of extra compiler flags.

include(CheckCCompilerFlag)
include(CheckCXXCompilerFlag)

# Conditionally add an extra compiler flag to C and C++ flags.
#
# If the language flags already match the `match` argument, skip this flag.
# Otherwise, check for support for `flag` and if support is found, add it to
# the compiler flags for the run. Also sets `result` to MATCHED/ADDED/UNSUPPORTED
# depending on whether the flag was added or not.
function(add_extra_compiler_flag match flag result)
  set(CMAKE_REQUIRED_FLAGS "-Werror")

  string(MAKE_C_IDENTIFIER "${flag}" flag_name)

  if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
    check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
  else()
    set(matched_c TRUE)
  endif()

  if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
    check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})
  else()
    set(matched_cxx TRUE)
  endif()

  if(HAVE_C_${flag_name} AND HAVE_CXX_${flag_name})
    add_compile_options("${flag}")
    add_link_options("${flag}")
    set(${result} ADDED PARENT_SCOPE)
  elseif(matched_c OR matched_cxx)
    set(${result} MATCHED PARENT_SCOPE)
  else()
    set(${result} UNSUPPORTED PARENT_SCOPE)
  endif()
endfunction()

# Same as add_simple_extra_compiler_flag, but check for a second flag if the
# first one is unsupported.
function(add_double_extra_compiler_flag match flag1 flag2 result)
  add_extra_compiler_flag("${match}" "${flag1}" flag1_success)

  if(${flag1_success} STREQUAL UNSUPPORTED)
    add_extra_compiler_flag("${match}" "${flag2}" flag2_success)
    set(${result} "${flag2_success}" PARENT_SCOPE)
  else()
    set(${result} "${flag1_success}" PARENT_SCOPE)
  endif()
endfunction()

# Add a required extra compiler flag to C and C++ flags.
#
# Similar logic to add_extra_compiler_flag, but ignores existing
# instances and throws an error if the flag is not supported.
function(add_required_compiler_flag flag)
  set(CMAKE_REQUIRED_FLAGS "-Werror")

  string(MAKE_C_IDENTIFIER "${flag}" flag_name)

  check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
  check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})

  if(HAVE_C_${flag_name} AND HAVE_CXX_${flag_name})
    add_compile_options("${flag}")
    add_link_options("${flag}")
  else()
    message(FATAL_ERROR "${flag} support is required to build Netdata")
  endif()
endfunction()

message(CHECK_START "Checking for known bad compiler flags")
string(REGEX MATCH "(-Ofast|-ffast-math)" BAD_FLAGS "${CMAKE_C_FLAGS}" "${CMAKE_CXX_FLAGS}")
if(BAD_FLAGS)
  message(CHECK_FAIL "${BAD_FLAGS}")
  message(FATAL_ERROR "Found known bad compiler flag '${BAD_FLAGS}'. This flag allows the compiler to violate the guarantees of the C/C++ standards in ways that are known to break Netdata (and many other things as well). Refusing to build with this compiler flag. Any issues opened about builds that circumvent this check and build with this flag anyway will be closed as invalid.")
else()
  message(CHECK_PASS "none found")
endif()

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

if(STATIC_BUILD)
  add_required_compiler_flag("-static")

  set(CMAKE_EXE_LINKER_FLAGS "${CMAKE_EXE_LINKER_FLAGS} -static")
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
  add_double_extra_compiler_flag("stack-protector" "-fstack-protector-strong" "-fstack-protector" HAVE_STACK_PROTECTOR)
  add_double_extra_compiler_flag("_FORTIFY_SOURCE" "-D_FORTIFY_SOURCE=3" "-D_FORTIFY_SOURCE=2" HAVE_FORTIFY_SOURCE)
  add_extra_compiler_flag("stack-clash-protection" "-fstack-clash-protection" HAVE_STACK_CLASH_PROTECTION)
  add_extra_compiler_flag("-fcf-protection" "-fcf-protection=full" HAVE_CFI)
  add_extra_compiler_flag("branch-protection" "-mbranch-protection=standard" HAVE_BRANCH_PROTECTION)
endif()

foreach(FLAG function-sections data-sections)
  add_extra_compiler_flag("${FLAG}" "-f${FLAG}" HAVE_${FLAG})
endforeach()

add_extra_compiler_flag("-Wbuiltin-macro-redefined" "-Wno-builtin-macro-redefined" HAVE_MACRO)
