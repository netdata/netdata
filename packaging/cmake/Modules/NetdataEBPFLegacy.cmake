# Handling for eBPF legacy programs
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(ExternalProject)
include(NetdataUtil)

set(ebpf-legacy_SOURCE_DIR "${CMAKE_BINARY_DIR}/ebpf-legacy")
set(ebpf-legacy_BUILD_DIR "${CMAKE_BINARY_DIR}/ebpf-legacy-build")

# Fetch the legacy eBPF code.
function(netdata_fetch_legacy_ebpf_code)
    netdata_identify_libc(_libc)

    if(DEFINED BUILD_SHARED_LIBS)
        if(NOT BUILD_SHARED_LIBS)
            set(need_static TRUE)
        endif()
    endif()

    if(need_static)
        set(_hash 73cfe6ceb0098447c2a016d8f9674ad8a80ba6dd61cfdd8646710a0dc5c52d9f)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash e9b89ff215c56f04249a2169bee15cdae6e61ef913468add258719e8afe6eac2)
    elseif(_libc STREQUAL "musl")
        set(_hash 1574e3bbdcac7dae534d783fa6fc6452268240d7477aefd87fadc4e7f705d48e)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.4.3/netdata-kernel-collector-${_libc}-v1.4.3.tar.xz
        URL_HASH SHA256=${_hash}
        SOURCE_DIR "${ebpf-legacy_SOURCE_DIR}"
        CONFIGURE_COMMAND ""
        BUILD_COMMAND sh -c "mkdir -p ${ebpf-legacy_BUILD_DIR}/ebpf.d && mv ${ebpf-legacy_SOURCE_DIR}/*netdata_ebpf_*.o ${ebpf-legacy_BUILD_DIR}/ebpf.d"
        INSTALL_COMMAND ""
    )
endfunction()

function(netdata_install_legacy_ebpf_code)
    install(DIRECTORY ${ebpf-legacy_BUILD_DIR}/ebpf.d
            DESTINATION usr/libexec/netdata/plugins.d
            COMPONENT ebpf-code-legacy)
endfunction()
