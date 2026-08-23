# SPDX-License-Identifier: GPL-3.0-or-later
# macos-logs: unified-log collection on macOS.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The source list lives here rather than in the shared
# NetdataSourceLists.cmake, so a plugin's whole definition is in one place. It
# sits above the guard, so it is defined unconditionally.
#
# This block only configures on macOS, and only when OSLog and Foundation are
# found, so a Linux build never evaluates it. Verify any change to it against
# the macOS CI jobs; a local configure will not exercise it.
#
# The plugin-macos-logs CPack component tagged below is registered in
# Packaging.cmake, alongside every other component, behind the same guard this
# file uses. Keep the two in step: a component registered without its install
# rule makes CPack emit an empty package, and an install rule whose component is
# unregistered ships the binary in no package at all.

include_guard()

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
