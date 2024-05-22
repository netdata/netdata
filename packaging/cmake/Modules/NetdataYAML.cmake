# Functions and macros for handling of libYAML
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

# Handle bundling of libyaml.
#
# This pulls it in as a sub-project using FetchContent functionality.
#
# This needs to be a function and not a macro for variable scoping
# reasons. All the things we care about from the sub-project are exposed
# as targets, which are globally scoped and not function scoped.
function(netdata_bundle_libyaml)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        if(ENABLE_BUNDLED_LIBYAML)
                set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)
        endif()

        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        FetchContent_Declare(yaml
                GIT_REPOSITORY https://github.com/yaml/libyaml
                GIT_TAG 2c891fc7a770e8ba2fec34fc6b545c672beb37e6 # v0.2.5
                CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
        )

        FetchContent_MakeAvailable_NoInstall(yaml)
endfunction()

# Handle setup of libyaml for the build.
#
# This will attempt to find libyaml using pkg_check_modules. If it finds
# a usable copy, that will be used. If not, it will bundle a vendored copy
# as a sub-project.
#
# Irrespective of how libyaml is to be included, library names,
# include directories, and compile definitions will be specified in the
# NETDATA_YAML_* variables for later use.
macro(netdata_detect_libyaml)
        set(HAVE_LIBYAML True)

        pkg_check_modules(YAML yaml-0.1)

        if(ENABLE_BUNDLED_LIBYAML OR NOT YAML_FOUND)
                netdata_bundle_libyaml()
                set(NETDATA_YAML_LDFLAGS yaml)
                get_target_property(NETDATA_YAML_INCLUDE_DIRS yaml INTERFACE_INCLUDE_DIRECTORIES)
                get_target_property(NETDATA_YAML_CFLAGS_OTHER yaml INTERFACE_COMPILE_DEFINITIONS)
        else()
                set(NETDATA_YAML_LDFLAGS ${YAML_LDFLAGS})
                set(NETDATA_YAML_CFLAGS_OTHER ${YAML_CFLAGS_OTHER})
                set(NETDATA_YAML_INCLUDE_DIRS ${YAML_INCLUDE_DIRS})
        endif()
endmacro()

# Add libyaml as a public link dependency of the specified target.
#
# The specified target must already exist, and the netdata_detect_libyaml
# macro must have already been run at least once for this to work correctly.
function(netdata_add_libyaml_to_target _target)
        target_include_directories(${_target} PUBLIC ${NETDATA_YAML_INCLUDE_DIRS})
        target_compile_options(${_target} PUBLIC ${NETDATA_YAML_CFLAGS_OTHER})
        target_link_libraries(${_target} PUBLIC ${NETDATA_YAML_LDFLAGS})
endfunction()
