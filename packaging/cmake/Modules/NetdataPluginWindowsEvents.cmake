# SPDX-License-Identifier: GPL-3.0-or-later
# windows-events: Windows Event Log collection.
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
# This block only configures on Windows, so a Linux or macOS build never
# evaluates it. Verify any change to it against the Build Windows CI job; a
# local configure will not exercise it.

include_guard()

set(WINDOWS_EVENTS_PLUGIN_FILES
        src/collectors/windows-events.plugin/windows-events.c
        src/collectors/windows-events.plugin/windows-events.h
        src/collectors/windows-events.plugin/windows-events-query.h
        src/collectors/windows-events.plugin/windows-events-query.c
        src/collectors/windows-events.plugin/windows-events-sources.c
        src/collectors/windows-events.plugin/windows-events-sources.h
        src/collectors/windows-events.plugin/windows-events-unicode.c
        src/collectors/windows-events.plugin/windows-events-unicode.h
        src/collectors/windows-events.plugin/windows-events-xml.c
        src/collectors/windows-events.plugin/windows-events-xml.h
        src/collectors/windows-events.plugin/windows-events-providers.c
        src/collectors/windows-events.plugin/windows-events-providers.h
        src/collectors/windows-events.plugin/windows-events-fields-cache.c
        src/collectors/windows-events.plugin/windows-events-fields-cache.h
        src/collectors/windows-events.plugin/windows-events-query-builder.c
        src/collectors/windows-events.plugin/windows-events-query-builder.h
        src/collectors/windows-events.plugin/windows-events-query-evt-variant.c
)

if(OS_WINDOWS)
        add_executable(windows-events.plugin ${WINDOWS_EVENTS_PLUGIN_FILES})
        target_link_libraries(windows-events.plugin libnetdata wevtapi)

        install(TARGETS windows-events.plugin
                COMPONENT plugin-windows-events
                DESTINATION ${PLUGINS_DEST})
endif()
