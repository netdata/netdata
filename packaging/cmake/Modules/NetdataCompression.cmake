# Handling for compression libraries
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(NetdataUtil)

# Look for a usable copy of LZ4
macro(netdata_detect_lz4)
    pkg_check_modules(LZ4 liblz4>=1.7.0)
    if(LZ4_FOUND)
        set(ENABLE_LZ4 True)
        set(NETDATA_LINK_LZ4 True)
    endif()
endmacro()

# Add lz4 to a target with the given scope
function(netdata_add_lz4_to_target _target _scope)
    if(NETDATA_LINK_LZ4)
        netdata_add_lib_to_target(${_target} ${_scope} LZ4)
    endif()
endfunction()

# Look for a usable copy of zstd
macro(netdata_detect_zstd)
    pkg_check_modules(ZSTD libzstd)

    if(ZSTD_FOUND)
        set(ENABLE_ZSTD True)
    endif()
endmacro()

# Add zstd to a target with the given scope
function(netdata_add_zstd_to_target _target _scope)
    if(ENABLE_ZSTD)
        netdata_add_lib_to_target(${_target} ${_scope} ZSTD)
    endif()
endfunction()

# Look for a usable copy of brotli
macro(netdata_detect_brotli)
    pkg_check_modules(BROTLI libbrotlidec libbrotlienc libbrotlicommon)

    if(BROTLI_FOUND)
        set(ENABLE_BROTLI True)
    endif()
endmacro()

# Add brotli to a target with the given scope
function(netdata_add_brotli_to_target _target _scope)
    if(ENABLE_BROTLI)
        netdata_add_lib_to_target(${_target} ${_scope} BROTLI)
    endif()
endfunction()
