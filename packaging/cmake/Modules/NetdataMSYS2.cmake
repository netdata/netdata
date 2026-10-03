# SPDX-License-Identifier: GPL-3.0-or-later

# Finds an MSYS2 executable from the configured root or standard installs.
function(netdata_find_msys2_program output)
    cmake_parse_arguments(PARSE_ARGV 1 ARG "" "" "NAMES;HINTS")
    if(NOT ARG_NAMES)
        message(FATAL_ERROR "netdata_find_msys2_program requires NAMES")
    endif()

    set(_netdata_msys2_program "")
    set(_netdata_msys2_search_dirs ${ARG_HINTS})
    file(TO_CMAKE_PATH "$ENV{PATH}" _netdata_msys2_path_dirs)
    list(APPEND _netdata_msys2_search_dirs ${_netdata_msys2_path_dirs})

    foreach(_netdata_msys2_hint IN LISTS _netdata_msys2_search_dirs)
        if(NOT _netdata_msys2_hint)
            continue()
        endif()
        unset(_netdata_msys2_candidate CACHE)
        unset(_netdata_msys2_candidate)
        find_program(_netdata_msys2_candidate
                     NAMES ${ARG_NAMES}
                     HINTS "${_netdata_msys2_hint}"
                     NO_DEFAULT_PATH)

        if(NOT _netdata_msys2_candidate)
            continue()
        endif()

        get_filename_component(_netdata_msys2_bin "${_netdata_msys2_candidate}" DIRECTORY)
        get_filename_component(_netdata_msys2_root "${_netdata_msys2_bin}/../.." ABSOLUTE)
        if(EXISTS "${_netdata_msys2_bin}/msys-2.0.dll"
           AND IS_DIRECTORY "${_netdata_msys2_root}/ucrt64/bin")
            set(_netdata_msys2_program "${_netdata_msys2_candidate}")
            break()
        endif()
    endforeach()

    set(${output} "${_netdata_msys2_program}" PARENT_SCOPE)
endfunction()
