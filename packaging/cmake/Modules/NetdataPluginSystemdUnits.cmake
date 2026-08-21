# SPDX-License-Identifier: GPL-3.0-or-later
# systemd-units: systemd unit state collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list moved here from NetdataSourceLists.cmake per D27, and stays
# above the guard so it is still defined unconditionally.

set(SYSTEMD_UNITS_PLUGIN_FILES
        src/collectors/systemd-units.plugin/plugin_systemd_units.c
)

if(ENABLE_PLUGIN_SYSTEMD_UNITS)
        if(NOT SYSTEMD_FOUND)
                message(FATAL_ERROR "Systemd units plugin requires systemd, but systemd was not found.")
        endif()

        if(SYSTEMD_VERSION LESS 221)
            message(FATAL_ERROR "Systemd units plugin requires systemd 221 or newer, but only systemd ${SYSTEMD_VERSION} was found.")
        endif()

        include(FetchContent)

        add_executable(systemd-units.plugin ${SYSTEMD_UNITS_PLUGIN_FILES})
        target_link_libraries(systemd-units.plugin libnetdata)

        install(TARGETS systemd-units.plugin
                COMPONENT plugin-systemd-units
                DESTINATION ${PLUGINS_DEST})

        netdata_add_deb_copyright(plugin-systemd-units netdata-plugin-systemd-units)
endif()
