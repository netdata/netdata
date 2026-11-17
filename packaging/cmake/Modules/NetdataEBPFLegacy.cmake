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
        set(_hash 7e9ebe82873956bbdb0f2070f056a6afab622a07803d75a6ffa59ae3e94906e9)
        set(_libc "static")
    elseif(_libc STREQUAL "glibc")
        set(_hash 9db210325a6514e1ed6ada1162e9e2b384e8d2fa91cc231fa503a3a4e41f7651)
    elseif(_libc STREQUAL "musl")
        set(_hash a92b362b7cf176a823f15fe4a8ea1a3dd6afea69b28b48a7d9ff9a8b498eeaeb)
    else()
        message(FATAL_ERROR "Could not determine libc implementation, unable to install eBPF legacy code.")
    endif()

    ExternalProject_Add(
        ebpf-code-legacy
        URL https://github.com/netdata/kernel-collector/releases/download/v1.8.0.4/netdata-kernel-collector-${_libc}-v1.8.0.4.tar.xz
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
