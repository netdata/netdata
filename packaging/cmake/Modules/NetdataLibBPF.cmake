# SPDX-License-Identifier: GPL-3.0-or-later
# Handling for libbpf (used by the eBPF plugin)

include(ExternalProject)
include(NetdataUtil)

set(libbpf_SOURCE_DIR "${CMAKE_BINARY_DIR}/libbpf")

# Check if the kernel is old enough that we need to use a legacy copy of eBPF.
function(_need_legacy_libbpf _var)
    if(FORCE_LEGACY_LIBBPF)
        set(${_var} TRUE PARENT_SCOPE)
        return()
    endif()

    netdata_detect_host_kernel_version()

    if(HOST_KERNEL_VERSION VERSION_LESS "4.14.0")
        set(${_var} TRUE PARENT_SCOPE)
    else()
        set(${_var} FALSE PARENT_SCOPE)
    endif()
endfunction()

# Prepare a vendored copy of libbpf
function(netdata_bundle_libbpf)
    _need_legacy_libbpf(USE_LEGACY_LIBBPF)

    if(USE_LEGACY_LIBBPF)
        set(_libbpf_tag 673424c56127bb556e64095f41fd60c26f9083ec) # v0.0.9_netdata-1
    else()
        set(_libbpf_tag 02bdeb7a2c2e7cb2c9cecb125527a9c5a6bbf139) # v1.6.0p_netdata
    endif()

    if(DEFINED BUILD_SHARED_LIBS)
        if(NOT BUILD_SHARED_LIBS)
            set(need_static TRUE)
        endif()
    endif()

    if(NOT need_static)
        netdata_identify_libc(_libc)

        string(REGEX MATCH "glibc|musl" _libc_supported "${_libc}")

        if(NOT _libc_supported)
            message(FATAL_ERROR "This system’s libc (detected: ${_libc}) is not not supported by the eBPF plugin.")
        endif()
    endif()

    find_program(MAKE_COMMAND make)

    if(MAKE_COMMAND STREQUAL MAKE_COMMAND-NOTFOUND)
        message(FATAL_ERROR "GNU Make is required when building the eBPF plugin, but could not be found.")
    endif()

    pkg_check_modules(ELF REQUIRED libelf)
    pkg_check_modules(ZLIB REQUIRED zlib)

    set(_libbpf_lib_dir lib)

    if(CMAKE_SYSTEM_PROCESSOR MATCHES "(x86_64)|(amd64)")
        set(_libbpf_lib_dir lib64)
    endif()

    set(_libbpf_library "${libbpf_SOURCE_DIR}/usr/${_libbpf_lib_dir}/libbpf.a")

    ExternalProject_Add(
        libbpf
        GIT_REPOSITORY https://github.com/netdata/libbpf.git
        GIT_TAG ${_libbpf_tag}
        SOURCE_DIR "${libbpf_SOURCE_DIR}"
        CONFIGURE_COMMAND ""
        BUILD_COMMAND ${MAKE_COMMAND} -C src CC=${CMAKE_C_COMPILER} BUILD_STATIC_ONLY=1 OBJDIR=build/ DESTDIR=../ install
        BUILD_IN_SOURCE 1
        BUILD_BYPRODUCTS "${_libbpf_library}"
        INSTALL_COMMAND ""
        EXCLUDE_FROM_ALL 1
    )

    add_library(libbpf_library STATIC IMPORTED GLOBAL)
    set_property(
        TARGET libbpf_library
        PROPERTY IMPORTED_LOCATION "${_libbpf_library}"
    )
    set_property(
        TARGET libbpf_library
        PROPERTY INTERFACE_LINK_LIBRARIES "${ELF_LIBRARIES};${ZLIB_LIBRARIES}"
    )
    set(NETDATA_LIBBPF_INCLUDE_DIRECTORIES "${libbpf_SOURCE_DIR}/usr/include;${libbpf_SOURCE_DIR}/include;${ELF_INCLUDE_DIRECTORIES};${ZLIB_INCLUDE_DIRECTORIES}" PARENT_SCOPE)
    set(NETDATA_LIBBPF_COMPILE_OPTIONS "${ELF_CFLAGS_OTHER};${ZLIB_CFLAGS_OTHER}" PARENT_SCOPE)
endfunction()

# Add libbpf as a link dependency for the given target.
function(netdata_add_libbpf_to_target _target)
    target_link_libraries(${_target} libbpf_library)
    target_include_directories(${_target} BEFORE PUBLIC "${NETDATA_LIBBPF_INCLUDE_DIRECTORIES}")
    target_compile_options(${_target} PUBLIC "${NETDATA_LIBBPF_COMPILE_OPTIONS}")
    add_dependencies(${_target} libbpf)
endfunction()
