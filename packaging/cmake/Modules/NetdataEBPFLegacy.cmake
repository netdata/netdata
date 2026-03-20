# SPDX-License-Identifier: GPL-3.0-or-later
# Handling for eBPF legacy programs

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
        set(_hash f06a22d1e25eb3a18a43660ccf3160c7f1c80baa0f0a4ed6c0c6f4be3d11034b)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash 537f73c340ad2ef2d31a9d412d1eeaa7274bbb064328a77a0b7d8c069a163077)
    elseif(_libc STREQUAL "musl")
        set(_hash d1da846746df0f2fe67133d97df51f0550048457df75daaf4bc9e0c5d9e7e1a5)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.7.0/netdata-kernel-collector-${_libc}-v1.7.0.tar.xz
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
