# Extra tools for working with FetchContent on older CMake
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

# FetchContent_MakeAvailable_NoInstall
#
# Add a sub-project with FetchContent, but with the EXCLUDE_FROM_ALL
# argument for the add_subdirectory part.
#
# CMake 3.28 and newer provide a way to do this with an extra argument
# on FetchContent_Declare, but older versions need you to implement
# the logic yourself. Once we no longer support CMake versions older
# than 3.28, we can get rid of this macro.
#
# Unlike FetchContent_MakeAvailble, this only accepts a single project
# to make available.
macro(FetchContent_MakeAvailable_NoInstall name)
    include(FetchContent)

    FetchContent_GetProperties(${name})

    if(NOT ${name}_POPULATED)
        FetchContent_Populate(${name})
        add_subdirectory(${${name}_SOURCE_DIR} ${${name}_BINARY_DIR} EXCLUDE_FROM_ALL)
    endif()
endmacro()
