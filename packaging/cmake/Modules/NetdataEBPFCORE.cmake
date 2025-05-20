# SPDX-License-Identifier: GPL-3.0-or-later
# Handling for eBPF CO-RE files

include(ExternalProject)

set(ebpf-co-re_SOURCE_DIR "${CMAKE_BINARY_DIR}/ebpf-co-re")

# Fetch and install our eBPF CO-RE files
function(netdata_fetch_ebpf_co_re)
    ExternalProject_Add(
        ebpf-co-re
        URL https://github.com/netdata/ebpf-co-re/releases/download/v1.5.1/netdata-ebpf-co-re-glibc-v1.5.1.tar.xz
        URL_HASH SHA256=a0c64a8d4f61c106c01125b07eeebda92a9ed45af806366c0ebea8197eae83f8
        SOURCE_DIR "${ebpf-co-re_SOURCE_DIR}"
        CONFIGURE_COMMAND ""
        BUILD_COMMAND ""
        INSTALL_COMMAND ""
        EXCLUDE_FROM_ALL 1
    )
endfunction()

function(netdata_add_ebpf_co_re_to_target _target)
        add_dependencies(${_target} ebpf-co-re)
        target_include_directories(${_target} BEFORE PRIVATE "${ebpf-co-re_SOURCE_DIR}")
endfunction()
