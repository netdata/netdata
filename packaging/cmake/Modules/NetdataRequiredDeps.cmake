# Handling for mandatory dependencies.
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(NetdataUtil)

# Locate a usable copy of zlib
macro(netdata_detect_zlib)
    if(MACOS)
        find_package(ZLIB REQUIRED)
        set(ZLIB_LIBRARIES "ZLIB::ZLIB")
    else()
        pkg_check_modules(ZLIB REQUIRED zlib)
    endif()
endmacro()

# Add zlib to a target with the given scope
function(netdata_add_zlib_to_target _target _scope)
    netdata_add_lib_to_target(${_target} ${_scope} ZLIB)
endfunction()

# Locate a usable copy of libuuid
#
# macOS includes the required functionality in the system libraries, so skip this there.
macro(netdata_detect_libuuid)
    if(NOT MACOS)
        pkg_check_modules(UUID REQUIRED uuid)
    endif()
endmacro()

# Add libuuid to a target with the given scope
function(netdata_add_libuuid_to_target _target _scope)
    netdata_add_lib_to_target(${_target} ${_scope} UUID)
endfunction()

# Locate a usable copy of libuv
macro(netdata_detect_libuv)
    pkg_check_modules(LIBUV REQUIRED libuv)
endmacro()

# Add libuv to a target with the given scope
function(netdata_add_libuv_to_target _target _scope)
    netdata_add_lib_to_target(${_target} ${_scope} LIBUV)
endfunction()
