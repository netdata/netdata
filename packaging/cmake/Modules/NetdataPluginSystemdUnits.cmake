# SPDX-License-Identifier: GPL-3.0-or-later
# systemd-units: systemd unit state collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The source list lives here rather than in the shared
# NetdataSourceLists.cmake, so a plugin's whole definition is in one place. It
# sits above the guard, so it is defined unconditionally.

set(SYSTEMD_UNITS_PLUGIN_FILES
        src/collectors/systemd-units.plugin/plugin_systemd_units.c
)

if(ENABLE_PLUGIN_SYSTEMD_UNITS)
        add_executable(systemd-units.plugin ${SYSTEMD_UNITS_PLUGIN_FILES})
        target_link_libraries(systemd-units.plugin libnetdata)

        install(TARGETS systemd-units.plugin
                COMPONENT plugin-systemd-units
                DESTINATION ${PLUGINS_DEST})

        netdata_add_deb_copyright(plugin-systemd-units netdata-plugin-systemd-units)
endif()
