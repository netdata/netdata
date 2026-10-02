# SPDX-License-Identifier: GPL-3.0-or-later

# Finds an MSYS2 executable from the configured root or standard installs.
function(netdata_find_msys2_program output)
    cmake_parse_arguments(PARSE_ARGV 1 ARG "" "" "NAMES;HINTS")
    if(NOT ARG_NAMES)
        message(FATAL_ERROR "netdata_find_msys2_program requires NAMES")
    endif()

    unset(_netdata_msys2_program CACHE)
    unset(_netdata_msys2_program)
    find_program(_netdata_msys2_program
                 NAMES ${ARG_NAMES}
                 HINTS ${ARG_HINTS})

    if(_netdata_msys2_program)
        get_filename_component(_netdata_msys2_bin "${_netdata_msys2_program}" DIRECTORY)
        get_filename_component(_netdata_msys2_root "${_netdata_msys2_bin}/../.." ABSOLUTE)
        if(NOT EXISTS "${_netdata_msys2_bin}/msys-2.0.dll"
           OR NOT IS_DIRECTORY "${_netdata_msys2_root}/ucrt64/bin")
            unset(_netdata_msys2_program CACHE)
            unset(_netdata_msys2_program)
        endif()
    endif()

    set(${output} "${_netdata_msys2_program}" PARENT_SCOPE)
endfunction()
