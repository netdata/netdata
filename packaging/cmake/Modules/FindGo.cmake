# Custom CMake module to find the Go toolchain
#
# Copyright (c) 2024 Netdata Inc
#
# SPDX-License-Identifier: GPL-3.0-or-later
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

find_program(GO_EXECUTABLE go PATHS /usr/local/go/bin DOC "Go toolchain")

if (GO_EXECUTABLE)
  execute_process(
       COMMAND ${GO_EXECUTABLE} version
       OUTPUT_VARIABLE GO_VERSION_STRING
       RESULT_VARIABLE RESULT
  )
  if (RESULT EQUAL 0)
    string(REGEX MATCH "go([0-9]+\\.[0-9]+\(\\.[0-9]+\)?)" GO_VERSION_STRING "${GO_VERSION_STRING}")
    string(REGEX MATCH "([0-9]+\\.[0-9]+\(\\.[0-9]+\)?)" GO_VERSION_STRING "${GO_VERSION_STRING}")
  endif()
endif()

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(
    Go
    REQUIRED_VARS GO_EXECUTABLE
    VERSION_VAR GO_VERSION_STRING
    FAIL_MESSAGE "Go toolchain not found"
)
