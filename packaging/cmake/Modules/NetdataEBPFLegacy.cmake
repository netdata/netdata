# SPDX-License-Identifier: GPL-3.0-or-later
# Handling for eBPF legacy programs

include_guard()

include(ExternalProject)
include(NetdataUtil)

set(ebpf-legacy_SOURCE_DIR "${CMAKE_BINARY_DIR}/ebpf-legacy")
set(ebpf-legacy_BUILD_DIR "${CMAKE_BINARY_DIR}/ebpf-legacy-build")

# Fetch the legacy eBPF code.
function(netdata_fetch_legacy_ebpf_code)
    netdata_identify_libc(_libc)

    # The root file forces BUILD_SHARED_LIBS off, so every current build takes
    # the static arm and its one libc-independent tarball; the glibc/musl arms
    # stay for the day that changes.
    if(DEFINED BUILD_SHARED_LIBS)
        if(NOT BUILD_SHARED_LIBS)
            set(need_static TRUE)
        endif()
    endif()

    if(need_static)
        set(_hash e25fe47729196089f450c481bfd38cfd8b3a3997f6b1f3a72b6b14cd6b134ce4)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash 7991d17f19fb2779906a4a1cbe1983a881ae989cbaf491c6e05b6a035d4e3635)
    elseif(_libc STREQUAL "musl")
        set(_hash 1244b369ed9d8f1cfacf6c6bdb73710e77bfbd4231afc4133698c3153260e619)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.8.0.3/netdata-kernel-collector-${_libc}-v1.8.0.3.tar.xz
        URL_HASH SHA256=${_hash}
        SOURCE_DIR "${ebpf-legacy_SOURCE_DIR}"
        CONFIGURE_COMMAND ""
        BUILD_COMMAND sh -c "mkdir -p ${ebpf-legacy_BUILD_DIR}/ebpf.d && mv ${ebpf-legacy_SOURCE_DIR}/*netdata_ebpf_*.o ${ebpf-legacy_BUILD_DIR}/ebpf.d"
        INSTALL_COMMAND ""
    )
endfunction()

function(netdata_install_legacy_ebpf_code)
    install(DIRECTORY ${ebpf-legacy_BUILD_DIR}/ebpf.d
            DESTINATION ${PLUGINS_DEST}
            COMPONENT ebpf-code-legacy)

    netdata_add_deb_copyright(ebpf-code-legacy netdata-ebpf-code-legacy)
endfunction()
