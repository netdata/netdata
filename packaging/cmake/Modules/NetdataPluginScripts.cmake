# SPDX-License-Identifier: GPL-3.0-or-later
# scripts.d.plugin: the Go Nagios-compatibility plugin and its config tree.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

if(ENABLE_PLUGIN_SCRIPTS)
    if (OS_WINDOWS)
        set(SCRIPTS_PLUGIN_BIN scripts.d.plugin.exe)
    else()
        set(SCRIPTS_PLUGIN_BIN scripts.d.plugin)
    endif()

    add_go_target(scripts-plugin ${SCRIPTS_PLUGIN_BIN} src/go cmd/scriptsdplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${SCRIPTS_PLUGIN_BIN}
            COMPONENT plugin-scripts
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/go/plugin/scripts.d/config/scripts.d.conf
            COMPONENT plugin-scripts
            DESTINATION ${LIBCONFIG_DEST})

    install(DIRECTORY src/go/plugin/scripts.d/config/scripts.d
            COMPONENT plugin-scripts
            DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")

    netdata_add_deb_copyright(plugin-scripts netdata-plugin-scripts)
endif()
