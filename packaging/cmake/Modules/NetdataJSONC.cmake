# Functions and macros for handling of JSON-C
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

# Handle bundling of json-c.
#
# This pulls it in as a sub-project using FetchContent functionality.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_jsonc)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        if(ENABLE_BUNDLED_JSONC)
                set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)
        endif()

        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        set(DISABLE_BSYMBOLIC True)
        set(DISABLE_WERROR True)
        set(DISABLE_EXTRA_LIBS True)
        set(BUILD_SHARED_LIBS False)
        set(BUILD_STATIC_LIBS True)
        set(BUILD_APPS False)

        FetchContent_Declare(json-c
                GIT_REPOSITORY https://github.com/json-c/json-c
                GIT_TAG b4c371fa0cbc4dcbaccc359ce9e957a22988fb34 # json-c-0.17-20230812
        )

        FetchContent_MakeAvailable_NoInstall(json-c)
endfunction()

# Handle setup of json-c for the build.
#
# This will attempt to find json-c using pkg_check_modules. If it finds
# a usable copy, that will be used. If not, it will bundle a vendored copy
# as a sub-project.
#
# Irrespective of how json-c is to be included, library names,
# include directories, and compile definitions will be specified in the
# NETDATA_JSONC_* variables for later use.
macro(netdata_detect_jsonc)
        pkg_check_modules(JSONC json-c)

        if(ENABLE_BUNDLED_JSONC OR NOT JSONC_FOUND)
                netdata_bundle_jsonc()
                set(NETDATA_JSONC_LDFLAGS json-c)
                get_target_property(NETDATA_JSONC_INCLUDE_DIRS json-c INTERFACE_INCLUDE_DIRECTORIES)
                get_target_property(NETDATA_JSONC_CFLAGS_OTHER json-c INTERFACE_COMPILE_DEFINITIONS)

                if(NETDATA_JSONC_CFLAGS_OTHER STREQUAL NETDATA_JSONC_CFLAGS_OTHER-NOTFOUND)
                        set(NETDATA_JSONC_CFLAGS_OTHER "")
                endif()

                if(NETDATA_JSONC_INCLUDE_DIRS STREQUAL NETDATA_JSONC_INCLUDE_DIRS-NOTFOUND)
                        set(NETDATA_JSONC_INCLUDE_DIRS "")
                endif()
        else()
                set(NETDATA_JSONC_LDFLAGS ${JSONC_LDFLAGS})
                set(NETDATA_JSONC_CFLAGS_OTHER ${JSONC_CFLAGS_OTHER})
                set(NETDATA_JSONC_INCLUDE_DIRS ${JSONC_INCLUDE_DIRS})
        endif()
endmacro()

# Add json-c as a public link dependency of the specified target.
#
# The specified target must already exist, and the netdata_detect_json-c
# macro must have already been run at least once for this to work correctly.
function(netdata_add_jsonc_to_target _target)
        target_include_directories(${_target} BEFORE PUBLIC ${NETDATA_JSONC_INCLUDE_DIRS})
        target_compile_definitions(${_target} PUBLIC ${NETDATA_JSONC_CFLAGS_OTHER})
        target_link_libraries(${_target} PUBLIC ${NETDATA_JSONC_LDFLAGS})
endfunction()
