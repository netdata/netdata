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
        URL https://github.com/netdata/ebpf-co-re/releases/download/v1.8.0.2/netdata-ebpf-co-re-glibc-v1.8.0.2.tar.xz
        URL_HASH SHA256=1f2e5a4cc9643eab9f39796ab2b124476a88976b48ff1c5539fdfd0e2b2548aa
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
