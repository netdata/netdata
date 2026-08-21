# SPDX-License-Identifier: GPL-3.0-or-later
# macos-logs: unified-log collection on macOS.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list moved here from NetdataSourceLists.cmake per D27, and stays
# above the guard so it is still defined unconditionally.
#
# Unreachable on this workstation: no local configuration takes the OS_MACOS
# branch, so the configure matrix cannot cover this block. The statement-
# stream comparison with guard stacks is what covers it, plus the macOS CI
# jobs.
#
# FINDINGS.md F2: the plugin-macos-logs CPack component is orphaned. Pre-
# existing, unchanged by this move.

set(MACOS_LOGS_PLUGIN_FILES
        src/collectors/macos-logs.plugin/macos-logs.c
        src/collectors/macos-logs.plugin/macos-logs.h
        src/collectors/macos-logs.plugin/macos-logs-oslog.m
)

if(OS_MACOS AND OSLOG AND FOUNDATION)
        add_executable(macos-logs.plugin ${MACOS_LOGS_PLUGIN_FILES})
        target_compile_options(macos-logs.plugin PRIVATE "$<$<COMPILE_LANGUAGE:OBJC>:-fobjc-arc>")
        target_link_libraries(macos-logs.plugin libnetdata ${FOUNDATION} ${OSLOG})

        install(TARGETS macos-logs.plugin
                COMPONENT plugin-macos-logs
                DESTINATION ${PLUGINS_DEST})
endif()
