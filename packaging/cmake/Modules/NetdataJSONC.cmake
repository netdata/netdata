# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of JSON-C

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

        message(STATUS "Preparing vendored copy of JSON-C")

        if(ENABLE_BUNDLED_JSONC)
                set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)
        endif()

        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        # JSON-C supports older versions of CMake than we do, so set
        # the correct values for the few policies we actually need.
        set(CMAKE_POLICY_DEFAULT_CMP0077 NEW)

        # JSON-C's build system does string comparisons against option
        # values instead of treating them as booleans, so we need to use
        # proper strings for option values instead of just setting them
        # to true or false.
        set(DISABLE_BSYMBOLIC ON)
        set(DISABLE_WERROR ON)
        set(DISABLE_EXTRA_LIBS ON)
        set(BUILD_SHARED_LIBS OFF)
        set(BUILD_STATIC_LIBS ON)
        set(BUILD_APPS OFF)

        set(repo https://github.com/json-c/json-c)
        set(tag b4c371fa0cbc4dcbaccc359ce9e957a22988fb34) # json-c-0.17-20230812

        if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
                FetchContent_Declare(json-c
                        GIT_REPOSITORY ${repo}
                        GIT_TAG ${tag}
                        CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                        EXCLUDE_FROM_ALL
                )
        else()
                FetchContent_Declare(json-c
                        GIT_REPOSITORY ${repo}
                        GIT_TAG ${tag}
                        CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                )
        endif()

        FetchContent_MakeAvailable_NoInstall(json-c)

        message(STATUS "Finished preparing vendored copy of JSON-C")
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
        if(NOT ENABLE_BUNDLED_JSONC)
                pkg_check_modules(JSONC json-c>=0.14)
        endif()

        if(NOT JSONC_FOUND)
                set(ENABLE_BUNDLED_JSONC True)
                netdata_bundle_jsonc()
                set(NETDATA_JSONC_LDFLAGS json-c)
                set(NETDATA_JSONC_INCLUDE_DIRS ${PROJECT_BINARY_DIR}/include)
                get_target_property(NETDATA_JSONC_CFLAGS_OTHER json-c INTERFACE_COMPILE_DEFINITIONS)

                if(NETDATA_JSONC_CFLAGS_OTHER STREQUAL NETDATA_JSONC_CFLAGS_OTHER-NOTFOUND)
                        set(NETDATA_JSONC_CFLAGS_OTHER "")
                endif()

                add_custom_command(
                        OUTPUT ${PROJECT_BINARY_DIR}/include/json-c
                        COMMAND ${CMAKE_COMMAND} -E make_directory ${PROJECT_BINARY_DIR}/include
                        COMMAND ${CMAKE_COMMAND} -E create_symlink ${json-c_BINARY_DIR} ${PROJECT_BINARY_DIR}/include/json-c
                        COMMENT "Create compatibility symlink for vendored JSON-C headers"
                        DEPENDS json-c
                )
                add_custom_target(
                        json-c-compat-link
                        DEPENDS ${PROJECT_BINARY_DIR}/include/json-c
                )
        else()
                set(NETDATA_JSONC_LDFLAGS ${JSONC_LDFLAGS})
                set(NETDATA_JSONC_CFLAGS_OTHER ${JSONC_CFLAGS_OTHER})
                set(NETDATA_JSONC_INCLUDE_DIRS ${JSONC_INCLUDE_DIRS})
                add_custom_target(json-c-compat-link)
        endif()
endmacro()

# Add json-c as a public link dependency of the specified target.
#
# The specified target must already exist, and the netdata_detect_json-c
# macro must have already been run at least once for this to work correctly.
function(netdata_add_jsonc_to_target _target)
        if(ENABLE_BUNDLED_JSONC)
                target_include_directories(${_target} BEFORE PUBLIC ${NETDATA_JSONC_INCLUDE_DIRS})
        else()
                target_include_directories(${_target} PUBLIC ${NETDATA_JSONC_INCLUDE_DIRS})
        endif()
        target_compile_options(${_target} PUBLIC ${NETDATA_JSONC_CFLAGS_OTHER})
        target_link_libraries(${_target} PUBLIC ${NETDATA_JSONC_LDFLAGS})
        add_dependencies(${_target} json-c-compat-link)
endfunction()
