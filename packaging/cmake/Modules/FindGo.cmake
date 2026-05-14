# SPDX-License-Identifier: GPL-3.0-or-later
#
# Custom CMake module to find the Go toolchain
#
# This is a relatively orthodox CMake Find Module. It can be used by
# simply including it and then invoking `find_package(Go)`.
#
# Version handling is done by CMake itself via the
# find_package_handle_standard_args() function, so `find_package(Go 1.21)`
# will also work correctly.

if(GO_FOUND)
    return()
endif()

# The complexity below is needed to account for the complex rules we use for finding the Go install.
#
# If GOROOT is set, we honor that first. Otherwise, on Unix-like systems we probe common install locations and the
# PATH copy, then select the newest usable Go binary we can find. For MSYS2, we prefer a Windows install over an
# MSYS2 install.
function(_netdata_probe_go_candidate candidate version_var root_var)
  if(NOT EXISTS "${candidate}")
    set(${version_var} "" PARENT_SCOPE)
    set(${root_var} "" PARENT_SCOPE)
    return()
  endif()

  execute_process(
    COMMAND "${candidate}" version
    OUTPUT_VARIABLE _go_version_output
    RESULT_VARIABLE _go_version_result
    OUTPUT_STRIP_TRAILING_WHITESPACE
  )
  if(NOT _go_version_result EQUAL 0)
    set(${version_var} "" PARENT_SCOPE)
    set(${root_var} "" PARENT_SCOPE)
    return()
  endif()

  string(REGEX MATCH "go([0-9]+\\.[0-9]+(\\.[0-9]+)?)" _go_version_match "${_go_version_output}")
  if(NOT CMAKE_MATCH_1)
    set(${version_var} "" PARENT_SCOPE)
    set(${root_var} "" PARENT_SCOPE)
    return()
  endif()

  set(_go_version_string "${CMAKE_MATCH_1}")
  execute_process(
    COMMAND "${candidate}" env GOROOT
    OUTPUT_VARIABLE _go_root_output
    RESULT_VARIABLE _go_root_result
    OUTPUT_STRIP_TRAILING_WHITESPACE
  )

  if(_go_root_result EQUAL 0)
    set(_go_root_string "${_go_root_output}")
  else()
    set(_go_root_string "")
  endif()

  set(${version_var} "${_go_version_string}" PARENT_SCOPE)
  set(${root_var} "${_go_root_string}" PARENT_SCOPE)
endfunction()

if(DEFINED ENV{GOROOT} AND NOT "$ENV{GOROOT}" STREQUAL "")
  set(_go_candidates "$ENV{GOROOT}/bin/go")
elseif(OS_WINDOWS)
  if(CMAKE_SYSTEM_NAME STREQUAL "Windows")
    set(_go_candidates C:/go/bin/go "C:/Program Files/go/bin/go")
  else()
    set(_go_candidates /c/go/bin/go "/c/Program Files/go/bin/go" /mingw64/lib/go/bin/go /ucrt64/lib/go/bin/go /clang64/lib/go/bin/go)
  endif()
else()
  file(GLOB _go_versioned_candidates LIST_DIRECTORIES FALSE
       /usr/lib64/go*/go/bin/go
       /usr/lib/go*/bin/go
       /usr/local/go/bin/go
       /opt/go/bin/go)
  find_program(_go_path_candidate go DOC "Go toolchain")

  set(_go_candidates ${_go_versioned_candidates})
  if(_go_path_candidate)
    list(APPEND _go_candidates "${_go_path_candidate}")
  endif()
endif()

set(GO_EXECUTABLE "")
set(GO_VERSION_STRING "")
set(GO_ROOT "")

foreach(_go_candidate IN LISTS _go_candidates)
  _netdata_probe_go_candidate("${_go_candidate}" _go_candidate_version _go_candidate_root)
  if(_go_candidate_version STREQUAL "")
    continue()
  endif()

  if(GO_EXECUTABLE STREQUAL "" OR _go_candidate_version VERSION_GREATER GO_VERSION_STRING)
    set(GO_EXECUTABLE "${_go_candidate}")
    set(GO_VERSION_STRING "${_go_candidate_version}")
    set(GO_ROOT "${_go_candidate_root}")
  endif()
endforeach()

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(
    Go
    REQUIRED_VARS GO_EXECUTABLE GO_ROOT
    VERSION_VAR GO_VERSION_STRING
)
