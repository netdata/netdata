# SPDX-License-Identifier: GPL-3.0-or-later
# Copies these hooks into the shared git hooks dir; run via `ninja setup-git-hooks` or `cmake -P`.

cmake_minimum_required(VERSION 3.16)

set(_hooks pre-commit)
set(_marker "managed-by: netdata setup-git-hooks")
get_filename_component(_source_dir "${CMAKE_CURRENT_LIST_DIR}/../../.." ABSOLUTE)

find_program(_git git)
if(NOT _git)
    message(FATAL_ERROR "setup-git-hooks: git not found.")
endif()

# --git-path follows core.hooksPath and, in a linked worktree, resolves to the
# hooks directory that all worktrees share.
execute_process(COMMAND "${_git}" rev-parse --git-path hooks
                WORKING_DIRECTORY "${_source_dir}"
                RESULT_VARIABLE _rc
                OUTPUT_VARIABLE _hooks_dir
                ERROR_VARIABLE _err
                OUTPUT_STRIP_TRAILING_WHITESPACE)
if(NOT _rc EQUAL 0)
    message(FATAL_ERROR "setup-git-hooks: ${_source_dir} is not a git checkout: ${_err}")
endif()
get_filename_component(_hooks_dir "${_hooks_dir}" ABSOLUTE BASE_DIR "${_source_dir}")
file(MAKE_DIRECTORY "${_hooks_dir}")

foreach(_hook IN LISTS _hooks)
    set(_src "${CMAKE_CURRENT_LIST_DIR}/${_hook}")
    set(_dst "${_hooks_dir}/${_hook}")
    set(_action "installed")

    # The installer never creates symlinks, so any symlink here belongs to someone else.
    if(IS_SYMLINK "${_dst}")
        message(FATAL_ERROR "setup-git-hooks: ${_dst} is a symlink this target did not create. "
                            "Remove it and run again.")
    endif()
    if(EXISTS "${_dst}")
        file(READ "${_src}" _wanted)
        file(READ "${_dst}" _installed)
        if(_installed STREQUAL _wanted)
            message(STATUS "setup-git-hooks: ${_dst} is up to date")
            continue()
        endif()
        string(FIND "${_installed}" "${_marker}" _pos)
        if(_pos EQUAL -1)
            message(FATAL_ERROR "setup-git-hooks: ${_dst} exists and this target did not install it. "
                                "Remove it or merge it by hand, then run again.")
        endif()
        set(_action "updated")
        # file(COPY) skips a destination whose timestamp matches or is newer.
        file(REMOVE "${_dst}")
    endif()

    file(COPY "${_src}" DESTINATION "${_hooks_dir}"
         FILE_PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE
                          GROUP_READ GROUP_EXECUTE
                          WORLD_READ WORLD_EXECUTE)
    message(STATUS "setup-git-hooks: ${_action} ${_dst}")
endforeach()
