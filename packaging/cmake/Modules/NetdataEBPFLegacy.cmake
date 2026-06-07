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
        set(_hash 8c245281693c3bbfdea362e8e4faad47c31465d9504cb680e92d7015d58fd20b)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash 83a4f226e0094bbc0584bfa3a75c034bb8afb46b4d4afb0e212dd08f95102fae)
    elseif(_libc STREQUAL "musl")
        set(_hash 37637b099e73e375a17882cb7de25d21a74c9538630810bed43076726a2e8b90)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.7.0.2/netdata-kernel-collector-${_libc}-v1.7.0.2.tar.xz
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
