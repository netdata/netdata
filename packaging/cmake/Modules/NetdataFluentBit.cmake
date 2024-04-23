# Macros and functions for handling of Fluent-Bit
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

# Handle bundling of fluent-bit
function(netdata_bundle_flb)
    include(FetchContent)
    include(NetdataFetchContentExtra)
    include(NetdataUtil)

    set(_need_musl_patches 0)

    netdata_identify_libc(_libc_name)

    if(_libc_name MATCHES "musl")
        set(_need_musl_patches 1)
    endif()

    message(STATUS "Preparing vendored copy of fluent-bit")

    set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

    include("${CMAKE_SOURCE_DIR}/src/logsmanagement/fluent_bit_build/config.cmake")

    FetchContent_Declare(
        fluent-bit
        GIT_REPOSITORY https://github.com/fluent/fluent-bit.git
        GIT_TAG b19e9ce674de872640c00a697fa545b66df0628a # last used submodule commit
        GIT_PROGRESS On
        SOURCE_DIR ${_ND_FLB_SRC_DIR}
        PATCH_COMMAND ${CMAKE_SOURCE_DIR}/src/logsmanagement/fluent_bit_build/apply-patches.sh ${_ND_FLB_SRC_DIR} ${_need_musl_patches}
        CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
    )

    FetchContent_MakeAvailable_NoInstall(fluent-bit)

    message(STATUS "Finished preparing vendored copy of fluent-bit")
endfunction()

# Ensure that fluentbit gets built if the specified target is built
function(netdata_require_flb_for_target _target)
    add_dependencies(${_target} fluent-bit-shared)
endfunction()

# Install the fluentbit library in the correct path for our plugin to find it
function(netdata_install_flb _component)
    install(FILES ${fluent-bit_BINARY_DIR}/lib/libfluent-bit.so
            DESTINATION usr/lib/netdata
            COMPONENT ${_component})
endfunction()
