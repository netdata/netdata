# SPDX-License-Identifier: GPL-3.0-or-later
# Extra tools for working with FetchContent on older CMake

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

    if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
        FetchContent_MakeAvailable(${name})
    else()
        FetchContent_GetProperties(${name})

        if(NOT ${name}_POPULATED)
            FetchContent_Populate(${name})
            add_subdirectory(${${name}_SOURCE_DIR} ${${name}_BINARY_DIR} EXCLUDE_FROM_ALL)
        endif()
    endif()
endmacro()

# NETDATA_PROPAGATE_TOOLCHAIN_ARGS
#
# Defines a set of CMake flags to be passed to CMAKE_ARGS for
# FetchContent_Declare and ExternalProject_Add to ensure that toolchain
# configuration propagates correctly to sub-projects.
#
# This needs to be explicitly included for any sub-project that needs
# to be built for the target system.
#
# This also needs to _NOT_ have any generator expressions, as they are not
# supported for the required usage of this variable in CMake 3.30 or newer.
set(NETDATA_PROPAGATE_TOOLCHAIN_ARGS
    "-DCMAKE_C_COMPILER=${CMAKE_C_COMPILER}
     -DCMAKE_CXX_COMPILER=${CMAKE_CXX_COMPILER}")

if(DEFINED CMAKE_C_COMPILER_TARGET)
  set(NETDATA_PROPAGATE_TOOLCHAIN_ARGS "${NETDATA_PROPAGATE_TOOLCHAIN_ARGS} -DCMAKE_C_COMPILER_TARGET=${CMAKE_C_COMPILER_TARGET}")
endif()

if(DEFINED CMAKE_CXX_COMPILER_TARGET)
  set(NETDATA_PROPAGATE_TOOLCHAIN_ARGS "${NETDATA_PROPAGATE_TOOLCHAIN_ARGS} -DCMAKE_CXX_COMPILER_TARGET=${CMAKE_CXX_COMPILER_TARGET}")
endif()
