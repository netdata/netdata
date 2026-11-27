# SPDX-License-Identifier: GPL-3.0-or-later
# Handling for eBPF CO-RE files

include_guard()

include(ExternalProject)

set(ebpf-co-re_SOURCE_DIR "${CMAKE_BINARY_DIR}/ebpf-co-re")

# Fetch our eBPF CO-RE files. The tarball is consumed at build time through
# include paths - by netdata_add_ebpf_co_re_to_target below and by the CGO
# flags in NetdataPluginEBPF.cmake - and no part of it is installed.
function(netdata_fetch_ebpf_co_re)
    ExternalProject_Add(
        ebpf-co-re
        URL https://github.com/netdata/ebpf-co-re/releases/download/v1.8.0.4/netdata-ebpf-co-re-glibc-v1.8.0.4.tar.xz
        URL_HASH SHA256=884c6bc79e62b6b2f1ae109d0bad1c11aea637624a58af3fc1ca6cedda2a8e6b
        SOURCE_DIR "${ebpf-co-re_SOURCE_DIR}"
        CONFIGURE_COMMAND ""
        BUILD_COMMAND ""
        INSTALL_COMMAND ""
        EXCLUDE_FROM_ALL 1
    )
endfunction()

# Expose the fetched headers to a target and order the fetch ahead of it.
function(netdata_add_ebpf_co_re_to_target _target)
        add_dependencies(${_target} ebpf-co-re)
        target_include_directories(${_target} BEFORE PRIVATE "${ebpf-co-re_SOURCE_DIR}")
endfunction()
