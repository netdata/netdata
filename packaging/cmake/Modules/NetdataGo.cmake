# SPDX-License-Identifier: GPL-3.0-or-later
# The Go toolchain: minimum-version detection and the Go-availability facts every Go-built component reads.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

set(ENABLE_PLUGIN_EBPF_GO OFF)
# ENABLE_CGROUP_NAME needs no term of its own: it is dependent on ENABLE_PLUGIN_GO.
if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_EBPF OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW)
    include(NetdataGoTools)

    find_min_go_version("${CMAKE_SOURCE_DIR}/src/go")

    find_package(Go "${MIN_GO_VERSION}" QUIET)
    if(NOT GO_FOUND)
        if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW)
            find_package(Go "${MIN_GO_VERSION}" REQUIRED)
        endif()
    endif()

    if(GO_FOUND)
        # The BPF side is always LP64, and the loader shares struct layouts with it,
        # so this plugin is only built for 64-bit userlands.
        if(CMAKE_SIZEOF_VOID_P EQUAL 8)
            set(ENABLE_PLUGIN_EBPF_GO ON)
        else()
            message(STATUS "eBPF Go plugin disabled: requires a 64-bit userland (pointer size is ${CMAKE_SIZEOF_VOID_P} bytes)")
        endif()
    endif()
endif()
