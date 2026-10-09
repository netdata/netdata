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
        set(_hash be23d718c79ba15f520062bf048f887cbc95f90bc2449d9c841b4a4a2276088e)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash 613cf0b7a2eec12993b7da61d6177ce32dbc4a4a9b6ef660d764c52c9068391e)
    elseif(_libc STREQUAL "musl")
        set(_hash b217f932d4059f69b401e892d1216aa37179015d09c3fe6371bcec5b6896bc65)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.7.0.4/netdata-kernel-collector-${_libc}-v1.7.0.4.tar.xz
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
