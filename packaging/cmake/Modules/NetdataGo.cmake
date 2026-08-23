# SPDX-License-Identifier: GPL-3.0-or-later
# The Go toolchain: minimum-version detection and the Go-availability facts every Go-built component reads.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

set(ENABLE_PLUGIN_EBPF_GO OFF)
if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_EBPF OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW OR ENABLE_CGROUP_NAME)
    include(NetdataGoTools)

    find_min_go_version("${CMAKE_SOURCE_DIR}/src/go")

    find_package(Go "${MIN_GO_VERSION}" QUIET)
    if(NOT GO_FOUND)
        if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW)
            find_package(Go "${MIN_GO_VERSION}" REQUIRED)
        endif()
    endif()

    if(GO_FOUND)
        set(ENABLE_PLUGIN_EBPF_GO ON)
    endif()
endif()
